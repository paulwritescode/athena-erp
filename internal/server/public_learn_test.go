package server

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/files"
)

// seedLearn installs course and transcript types with one published course
// (image, secret field), one unpublished course, one published transcript
// with a completion date, one published transcript without, and one
// unpublished transcript. It returns the stored course image bytes.
func seedLearn(t *testing.T, f *shopFixture) []byte {
	t.Helper()
	ctx := context.Background()
	saveRecordType(t, f.types, "course",
		textField("name", "Name", "Text"), textField("slug", "Slug", "Text"),
		textField("description", "Description", "Long text"), textField("image", "Image", "Image"),
		textField("is_published", "Published", "Checkbox"), textField("internal_note", "Internal note", "Text"))
	saveRecordType(t, f.types, "transcript",
		textField("verification_token", "Token", "Text"), textField("holder_name", "Holder", "Text"),
		textField("course_name", "Course", "Text"), textField("completed_on", "Completed on", "Date"),
		textField("status", "Status", "Text"),
		textField("is_published", "Published", "Checkbox"), textField("internal_note", "Internal note", "Text"))

	if _, err := f.store.PutImage(ctx, "images/bread.png", testLogoPNG(t, 41), files.Expect{}, files.Meta{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	_, stored, err := f.store.Get(ctx, "images/bread.png", 0)
	if err != nil {
		t.Fatal(err)
	}
	hash := files.Hash(stored)
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "course", Values: map[string]*athenav1.FieldValue{
		"name": text("Bread Basics"), "slug": text("bread-basics"), "description": text("Flour, water, salt, patience."),
		"image": imageValue("images/bread.png", hash, "A fresh loaf"), "is_published": shopBool(true),
		"internal_note": text("SECRET-COURSE")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "course", Values: map[string]*athenav1.FieldValue{
		"name": text("Hidden"), "slug": text("hidden"), "is_published": shopBool(false)}}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "transcript", Values: map[string]*athenav1.FieldValue{
		"verification_token": text("VFY-2026-0001"), "holder_name": text("Ada Baker"), "course_name": text("Bread Basics"),
		"completed_on": text("2026-06-15"), "status": text("Completed"), "is_published": shopBool(true),
		"internal_note": text("SECRET-TRANSCRIPT")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "transcript", Values: map[string]*athenav1.FieldValue{
		"verification_token": text("VFY-2026-0002"), "holder_name": text("Bo Miller"), "course_name": text("Bread Basics"),
		"status": text("Completed"), "is_published": shopBool(true)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "transcript", Values: map[string]*athenav1.FieldValue{
		"verification_token": text("VFY-2026-0003"), "holder_name": text("Cy Fox"), "course_name": text("Bread Basics"),
		"status": text("Completed"), "is_published": shopBool(false)}}); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestPublicCourses(t *testing.T) {
	f := newShopFixture(t)
	stored := seedLearn(t, f)
	imgURL := "http://example.com" + "/p/img/course/bread-basics/" + files.Hash(stored)
	rec := getPublic(t, f.ws, f.db, "/courses")
	if rec.Code != http.StatusOK {
		t.Fatalf("courses: %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("courses must be indexable, robots tag = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Courses — Acme</title>",
		`<meta name="description" content="Browse courses at Acme.">`,
		`<link rel="canonical" href="http://example.com/courses">`,
		`<meta property="og:image" content="http://example.com/p/og-fallback.png">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta name="twitter:image" content="http://example.com/p/og-fallback.png">`,
		"Bread Basics",
		"Flour, water, salt, patience.",
		`<img src="` + imgURL + `" alt="A fresh loaf"`,
		`aria-label="Breadcrumb"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("courses misses %q", want)
		}
	}
	for _, hidden := range []string{"Hidden", "SECRET-COURSE"} {
		if strings.Contains(body, hidden) {
			t.Fatalf("courses leaks %q", hidden)
		}
	}
	graph := graphNodes(t, body)
	if graphNode(graph, "Organization") == nil {
		t.Fatal("courses JSON-LD misses Organization")
	}
	list := graphNode(graph, "ItemList")
	if list == nil {
		t.Fatal("courses JSON-LD misses ItemList")
	}
	items, _ := list["itemListElement"].([]any)
	if len(items) != 1 {
		t.Fatalf("course items = %d", len(items))
	}
	entry, _ := items[0].(map[string]any)
	course, _ := entry["item"].(map[string]any)
	if entry["position"] != 1.0 || course["@type"] != "Course" || course["name"] != "Bread Basics" || course["image"] != imgURL {
		t.Fatalf("course item = %v", entry)
	}
}

func TestPublicCoursesEmpty(t *testing.T) {
	f := newShopFixture(t)
	if rec := getPublic(t, f.ws, f.db, "/courses"); rec.Code != http.StatusNotFound {
		t.Fatalf("courses without types: %d", rec.Code)
	}
	saveRecordType(t, f.types, "course",
		textField("name", "Name", "Text"), textField("slug", "Slug", "Text"), textField("is_published", "Published", "Checkbox"))
	if _, err := f.records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "course", Values: map[string]*athenav1.FieldValue{
		"name": text("Draft"), "slug": text("draft"), "is_published": shopBool(false)}}); err != nil {
		t.Fatal(err)
	}
	if rec := getPublic(t, f.ws, f.db, "/courses"); rec.Code != http.StatusNotFound {
		t.Fatalf("courses without published rows: %d", rec.Code)
	}
}

func TestPublicCourseImage(t *testing.T) {
	f := newShopFixture(t)
	stored := seedLearn(t, f)
	hash := files.Hash(stored)
	rec := getPublic(t, f.ws, f.db, "/p/img/course/bread-basics/"+hash)
	if rec.Code != http.StatusOK {
		t.Fatalf("image: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), stored) {
		t.Fatal("image bytes differ")
	}
	for _, target := range []string{
		"/p/img/course/bread-basics/" + strings.Repeat("0", 64),
		"/p/img/course/hidden/" + hash,
		"/p/img/course/nope/" + hash,
		"/p/img/transcript/bread-basics/" + hash,
	} {
		if rec := getPublic(t, f.ws, f.db, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
}

func TestPublicVerifyForm(t *testing.T) {
	f := newShopFixture(t)
	seedLearn(t, f)
	rec := getPublic(t, f.ws, f.db, "/verify")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("verify form must be indexable, robots tag = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Verify a transcript — Acme</title>",
		`<meta name="description" content="Verify a Acme transcript.">`,
		`<link rel="canonical" href="http://example.com/verify">`,
		`<form method="get" action="/verify">`,
		`<input id="token" name="token"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("verify misses %q", want)
		}
	}
	graph := graphNodes(t, body)
	if len(graph) != 1 || graphNode(graph, "Organization") == nil {
		t.Fatalf("verify JSON-LD must be Organization only: %v", graph)
	}
}

func TestPublicVerifySubmit(t *testing.T) {
	f := newShopFixture(t)
	seedLearn(t, f)
	rec := getPublic(t, f.ws, f.db, "/verify?token=VFY-2026-0001")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("submit: %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/verify/VFY-2026-0001" {
		t.Fatalf("location = %q", got)
	}
	if rec := getPublic(t, f.ws, f.db, "/verify?token="); rec.Code != http.StatusOK {
		t.Fatalf("empty token: %d", rec.Code)
	}
}

func TestPublicVerifyResult(t *testing.T) {
	f := newShopFixture(t)
	seedLearn(t, f)
	rec := getPublic(t, f.ws, f.db, "/verify/VFY-2026-0001")
	if rec.Code != http.StatusOK {
		t.Fatalf("result: %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("robots tag = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Transcript verified — Acme</title>",
		"Transcript verified",
		"Ada Baker",
		"Bread Basics",
		"2026-06-15",
		"Completed",
		`<a href="/verify">Verify another transcript</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("result misses %q", want)
		}
	}
	for _, hidden := range []string{"VFY-2026-0001", "SECRET-TRANSCRIPT", "canonical", "og:image"} {
		if strings.Contains(body, hidden) {
			t.Fatalf("result leaks %q", hidden)
		}
	}
	for _, target := range []string{"/verify/nope", "/verify/VFY-2026-0003"} {
		if rec := getPublic(t, f.ws, f.db, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
}

func TestPublicVerifyResultWithoutDate(t *testing.T) {
	f := newShopFixture(t)
	seedLearn(t, f)
	body := getPublic(t, f.ws, f.db, "/verify/VFY-2026-0002").Body.String()
	if !strings.Contains(body, "Bo Miller") {
		t.Fatal("result misses the holder")
	}
	if strings.Contains(body, "Completed on") {
		t.Fatal("result shows a date row without a date")
	}
}

func TestPublicVerifyResultEscapesValues(t *testing.T) {
	f := newShopFixture(t)
	ctx := context.Background()
	saveRecordType(t, f.types, "transcript",
		textField("verification_token", "Token", "Text"), textField("holder_name", "Holder", "Text"),
		textField("course_name", "Course", "Text"), textField("status", "Status", "Text"),
		textField("is_published", "Published", "Checkbox"))
	const tricky = `Bo <script>&"x"`
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "transcript", Values: map[string]*athenav1.FieldValue{
		"verification_token": text("VFY-X"), "holder_name": text(tricky), "course_name": text("Bread Basics"),
		"status": text("Completed"), "is_published": shopBool(true)}}); err != nil {
		t.Fatal(err)
	}
	body := getPublic(t, f.ws, f.db, "/verify/VFY-X").Body.String()
	if strings.Contains(body, tricky) {
		t.Fatal("holder name reached the page unescaped")
	}
	if !strings.Contains(body, "Bo &lt;script&gt;&amp;&#34;x&#34;") {
		t.Fatal("holder name is not HTML-escaped")
	}
}

func TestPublicOgFallback(t *testing.T) {
	f := newShopFixture(t)
	rec := getPublic(t, f.ws, f.db, "/p/og-fallback.png")
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=86400" {
		t.Fatalf("cache = %q", got)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("fallback must stay indexable, robots tag = %q", got)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 1200 || cfg.Height != 630 {
		t.Fatalf("fallback size = %dx%d", cfg.Width, cfg.Height)
	}
}

func TestPublicSitemapLearn(t *testing.T) {
	f := newShopFixture(t)
	if body := getPublic(t, f.ws, f.db, "/sitemap.xml").Body.String(); strings.Contains(body, "/courses") {
		t.Fatal("sitemap lists the catalogue with no courses")
	}
	seedLearn(t, f)
	body := getPublic(t, f.ws, f.db, "/sitemap.xml").Body.String()
	if !strings.Contains(body, "<loc>http://example.com/courses</loc>") {
		t.Fatal("sitemap misses the catalogue")
	}
	if strings.Contains(body, "/verify") {
		t.Fatal("sitemap lists verification URLs")
	}
}

func TestPublicHomeCoursesLink(t *testing.T) {
	f := newShopFixture(t)
	if body := getPublic(t, f.ws, f.db, "/home").Body.String(); strings.Contains(body, "Browse courses") {
		t.Fatal("home links courses with nothing published")
	}
	seedLearn(t, f)
	if body := getPublic(t, f.ws, f.db, "/home").Body.String(); !strings.Contains(body, `<a href="/courses">Browse courses</a>`) {
		t.Fatal("home misses the courses link")
	}
}
