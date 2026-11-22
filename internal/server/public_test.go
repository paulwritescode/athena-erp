package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"athena-erp/internal/brand"
	"athena-erp/internal/files"
	"athena-erp/internal/ledger"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

func publicFixture(t *testing.T, workspace *Workspace) (*sql.DB, *ledger.Service, *files.Store) {
	t.Helper()
	db := testdb.New(t)
	if _, err := store.Up(context.Background(), db, dialect, SystemMigrations(), store.UpOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	root := t.TempDir()
	st, err := files.Open(root, db, dialect)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return db, &ledger.Service{DB: db, Dialect: dialect}, st
}

func getPublic(t *testing.T, workspace *Workspace, db *sql.DB, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mountPublic(mux, workspace, db)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// parsePublicJSONLD extracts the ld+json block and unmarshals it, so tests prove
// the structured data parses rather than eyeball its text.
func parsePublicJSONLD(t *testing.T, body string) map[string]any {
	t.Helper()
	const open = `<script type="application/ld+json">`
	start := strings.Index(body, open)
	if start < 0 {
		t.Fatal("no JSON-LD block")
	}
	rest := body[start+len(open):]
	end := strings.Index(rest, "</script>")
	if end < 0 {
		t.Fatal("unterminated JSON-LD block")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(rest[:end]), &doc); err != nil {
		t.Fatalf("JSON-LD does not parse: %v", err)
	}
	return doc
}

func seedPublicBaseURL(t *testing.T, db *sql.DB, value string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO athena_setting_values(owner_pack,setting_type,scope_chain,effective_from,revision,value_json,status,published_by) VALUES('athena','public.base_url','',UTC_TIMESTAMP(3),1,?,'published','test')`, value); err != nil {
		t.Fatal(err)
	}
}

func TestPublicHome(t *testing.T) {
	ws := &Workspace{Name: "Acme Workspace"}
	db, svc, st := publicFixture(t, ws)
	logo, err := brand.ValidateLogo("logo.png", testLogoPNG(t, 21))
	if err != nil {
		t.Fatal(err)
	}
	seedLogo(t, svc, st, "light", logo)
	if err := svc.SaveIdentityDoc(context.Background(), ledger.IdentityDoc{DisplayName: "Acme", Light: &ledger.LogoRef{Ext: "png", Hash: logo.Hash}}, "test"); err != nil {
		t.Fatal(err)
	}

	rec := getPublic(t, ws, db, "/home")
	if rec.Code != http.StatusOK {
		t.Fatalf("home: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("home must be indexable, robots tag = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Acme</title>",
		`<meta name="description" content="Welcome to Acme.">`,
		`<link rel="canonical" href="http://example.com/home">`,
		`<meta property="og:title" content="Acme">`,
		`<meta property="og:url" content="http://example.com/home">`,
		`<meta property="og:image" content="http://example.com` + brand.LogoURL(logo.Hash, "png") + `">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<img src="` + brand.LogoURL(logo.Hash, "png") + `"`,
		`<link rel="icon" type="image/png" sizes="32x32" href="` + brand.FaviconURL(logo.Hash, brand.FaviconSmall) + `">`,
		"Powered by Athena",
		`<a href="/">Sign in</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("home misses %q", want)
		}
	}
	if strings.Contains(body, "og:image:width") {
		t.Fatal("logo og:image must not claim 1200x630 dimensions")
	}
	doc := parsePublicJSONLD(t, body)
	if doc["@type"] != "Organization" || doc["name"] != "Acme" || doc["url"] != "http://example.com/home" {
		t.Fatalf("organization JSON-LD = %v", doc)
	}
	if doc["logo"] != "http://example.com"+brand.LogoURL(logo.Hash, "png") {
		t.Fatalf("organization logo = %v", doc["logo"])
	}
}

func TestPublicHomeWithoutIdentity(t *testing.T) {
	ws := &Workspace{Name: "Acme Workspace"}
	db, _, _ := publicFixture(t, ws)
	rec := getPublic(t, ws, db, "/home")
	if rec.Code != http.StatusOK {
		t.Fatalf("home: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>Acme Workspace</title>") {
		t.Fatal("home misses the Workspace name fallback")
	}
	if strings.Contains(body, "og:image") || strings.Contains(body, "<img") {
		t.Fatal("home without a logo must carry no image tags")
	}
	if !strings.Contains(body, brand.FaviconURL(brand.MarkHash, brand.FaviconSmall)) {
		t.Fatal("home misses the mark favicon fallback")
	}
	doc := parsePublicJSONLD(t, body)
	if doc["name"] != "Acme Workspace" {
		t.Fatalf("organization name = %v", doc["name"])
	}
	if _, ok := doc["logo"]; ok {
		t.Fatalf("organization without a logo must omit logo: %v", doc)
	}
}

func TestPublicHomeWithoutDatabase(t *testing.T) {
	rec := getPublic(t, nil, nil, "/home")
	if rec.Code != http.StatusOK {
		t.Fatalf("home without db: %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<title>Athena</title>") {
		t.Fatal("home without db misses the Athena fallback")
	}
}

func TestPublicHomeEscapesName(t *testing.T) {
	ws := &Workspace{Name: "Acme"}
	db, svc, _ := publicFixture(t, ws)
	const tricky = `Acme <b>&"quoted"`
	if err := svc.SaveIdentityDoc(context.Background(), ledger.IdentityDoc{DisplayName: tricky}, "test"); err != nil {
		t.Fatal(err)
	}
	body := getPublic(t, ws, db, "/home").Body.String()
	if strings.Contains(body, tricky) {
		t.Fatal("display name reached the page unescaped")
	}
	if !strings.Contains(body, "Acme &lt;b&gt;&amp;&#34;quoted&#34;") {
		t.Fatal("display name is not HTML-escaped in the title")
	}
	doc := parsePublicJSONLD(t, body)
	if doc["name"] != tricky {
		t.Fatalf("JSON-LD name = %v", doc["name"])
	}
}

func TestPublicBaseURL(t *testing.T) {
	ws := &Workspace{Name: "Acme"}
	db, _, _ := publicFixture(t, ws)

	seedPublicBaseURL(t, db, `"https://shop.example.org/sub"`)
	rec := getPublic(t, ws, db, "/home")
	if body := rec.Body.String(); !strings.Contains(body, `<link rel="canonical" href="https://shop.example.org/home">`) {
		t.Fatal("public.base_url setting is not honored")
	}
	rec = getPublic(t, ws, db, "/robots.txt")
	if !strings.Contains(rec.Body.String(), "Sitemap: https://shop.example.org/sitemap.xml") {
		t.Fatal("robots sitemap line misses the setting origin")
	}

	if _, err := db.Exec(`UPDATE athena_setting_values SET value_json=? WHERE setting_type='public.base_url'`, `"not a url"`); err != nil {
		t.Fatal(err)
	}
	rec = getPublic(t, ws, db, "/home")
	if body := rec.Body.String(); !strings.Contains(body, `<link rel="canonical" href="http://example.com/home">`) {
		t.Fatal("invalid public.base_url must fall back to the request host")
	}
}

func TestPublicSitemap(t *testing.T) {
	ws := &Workspace{Name: "Acme"}
	db, _, _ := publicFixture(t, ws)
	rec := getPublic(t, ws, db, "/sitemap.xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("sitemap: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	var doc struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("sitemap does not parse: %v", err)
	}
	if len(doc.URLs) != 1 || doc.URLs[0].Loc != "http://example.com/home" {
		t.Fatalf("sitemap urls = %+v", doc.URLs)
	}
}

func TestPublicRobots(t *testing.T) {
	ws := &Workspace{Name: "Acme"}
	db, _, _ := publicFixture(t, ws)
	rec := getPublic(t, ws, db, "/robots.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("robots: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"User-agent: *",
		"Disallow: /build/",
		"Disallow: /view/",
		"Disallow: /verify/",
		"Disallow: /otlp/",
		"Disallow: /athena.v1.",
		"Sitemap: http://example.com/sitemap.xml",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("robots misses %q", want)
		}
	}
}

func TestDeskNoIndex(t *testing.T) {
	stub := deskNoIndex(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	for _, path := range []string{"/", "/build/overview", "/view/foo", "/manifest.webmanifest"} {
		rec := httptest.NewRecorder()
		stub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
			t.Fatalf("%s robots tag = %q", path, got)
		}
	}
	rec := httptest.NewRecorder()
	stub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-123.js", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("asset robots tag = %q", got)
	}
}

func TestPublicNotFound(t *testing.T) {
	h := newPublicHandler(&Workspace{Name: "Acme"}, nil)
	rec := httptest.NewRecorder()
	h.notFound(rec, httptest.NewRequest(http.MethodGet, "/shop/p/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("notfound: %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("robots tag = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Nothing here") || strings.Contains(body, "nope") {
		t.Fatal("404 hints at the missing path")
	}
	if strings.Contains(body, "canonical") {
		t.Fatal("404 must carry no canonical")
	}
}
