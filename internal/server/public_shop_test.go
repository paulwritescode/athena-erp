package server

import (
	"bytes"
	"context"
	"database/sql"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"testing"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/files"
	"athena-erp/internal/store"
	"athena-erp/internal/testdb"
)

type shopFixture struct {
	db      *sql.DB
	types   recordTypeService
	records recordService
	store   *files.Store
	ws      *Workspace
}

func newShopFixture(t *testing.T) *shopFixture {
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
	return &shopFixture{db: db, types: recordTypeService{db: db}, records: recordService{db: db}, store: st, ws: &Workspace{Name: "Acme", Root: root}}
}

func shopBool(v bool) *athenav1.FieldValue {
	return &athenav1.FieldValue{Kind: &athenav1.FieldValue_BoolValue{BoolValue: v}}
}

func textField(key, label, fieldType string) *athenav1.RecordTypeField {
	return &athenav1.RecordTypeField{Key: key, Label: label, Type: fieldType}
}

// seedCommerce installs product_category and product types with one published
// category, one published product (image, price, category, secret field), one
// unpublished product of each kind and one soft-deleted product. It returns
// the category ID and the stored image bytes.
func seedCommerce(t *testing.T, f *shopFixture) (string, []byte) {
	t.Helper()
	ctx := context.Background()
	saveRecordType(t, f.types, "product_category",
		textField("name", "Name", "Text"), textField("slug", "Slug", "Text"),
		textField("description", "Description", "Long text"), textField("is_published", "Published", "Checkbox"))
	saveRecordType(t, f.types, "product",
		textField("name", "Name", "Text"), textField("slug", "Slug", "Text"),
		textField("description", "Description", "Long text"), moneyField("price", "Price"),
		textField("image", "Image", "Image"),
		&athenav1.RecordTypeField{Key: "category", Label: "Category", Type: "Link", TargetRecordTypeKey: "product_category"},
		textField("is_published", "Published", "Checkbox"), textField("internal_note", "Internal note", "Text"))

	cat, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "product_category", Values: map[string]*athenav1.FieldValue{
		"name": text("Tools"), "slug": text("tools"), "description": text("Hand tools for the workshop."), "is_published": shopBool(true)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "product_category", Values: map[string]*athenav1.FieldValue{
		"name": text("Hidden"), "slug": text("hidden"), "is_published": shopBool(false)}}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.store.PutImage(ctx, "images/widget.png", testLogoPNG(t, 31), files.Expect{}, files.Meta{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	_, stored, err := f.store.Get(ctx, "images/widget.png", 0)
	if err != nil {
		t.Fatal(err)
	}
	hash := files.Hash(stored)
	long := strings.Repeat("A fine widget for every workshop. ", 10)
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "product", Values: map[string]*athenav1.FieldValue{
		"name": text("Widget"), "slug": text("widget"), "description": text(long), "price": text("12.30"),
		"image":    imageValue("images/widget.png", hash, "A shiny widget"),
		"category": text(cat.GetRecord().GetId()), "is_published": shopBool(true),
		"internal_note": text("SECRET-DO-NOT-PUBLISH")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "product", Values: map[string]*athenav1.FieldValue{
		"name": text("Hidden"), "slug": text("hidden"), "is_published": shopBool(false)}}); err != nil {
		t.Fatal(err)
	}
	gone, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "product", Values: map[string]*athenav1.FieldValue{
		"name": text("Gone"), "slug": text("gone"), "is_published": shopBool(true)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.records.DeleteRecord(ctx, &athenav1.DeleteRecordRequest{RecordTypeKey: "product", Id: gone.GetRecord().GetId(), ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	return cat.GetRecord().GetId(), stored
}

func graphNodes(t *testing.T, body string) []any {
	t.Helper()
	doc := parsePublicJSONLD(t, body)
	graph, ok := doc["@graph"].([]any)
	if !ok {
		t.Fatalf("JSON-LD has no @graph: %v", doc)
	}
	return graph
}

func graphNode(graph []any, nodeType string) map[string]any {
	for _, n := range graph {
		if m, ok := n.(map[string]any); ok && m["@type"] == nodeType {
			return m
		}
	}
	return nil
}

func TestPublicShop(t *testing.T) {
	f := newShopFixture(t)
	seedCommerce(t, f)
	rec := getPublic(t, f.ws, f.db, "/shop")
	if rec.Code != http.StatusOK {
		t.Fatalf("shop: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Shop Acme</title>",
		`<meta name="description" content="Browse everything in the Acme shop.">`,
		`<link rel="canonical" href="http://example.com/shop">`,
		`<a href="http://example.com/shop/p/widget">Widget</a>`,
		`12.30 USD`,
		`<a href="http://example.com/shop/c/tools">Tools</a>`,
		`aria-label="Breadcrumb"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("shop misses %q", want)
		}
	}
	for _, hidden := range []string{"Hidden", "Gone", "SECRET-DO-NOT-PUBLISH"} {
		if strings.Contains(body, hidden) {
			t.Fatalf("shop leaks %q", hidden)
		}
	}
	graph := graphNodes(t, body)
	if graphNode(graph, "Organization") == nil || graphNode(graph, "BreadcrumbList") == nil {
		t.Fatal("shop JSON-LD misses Organization or BreadcrumbList")
	}
}

func TestPublicCategory(t *testing.T) {
	f := newShopFixture(t)
	seedCommerce(t, f)
	rec := getPublic(t, f.ws, f.db, "/shop/c/tools")
	if rec.Code != http.StatusOK {
		t.Fatalf("category: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<title>Tools — Acme</title>",
		`<meta name="description" content="Hand tools for the workshop.">`,
		`<link rel="canonical" href="http://example.com/shop/c/tools">`,
		`<a href="http://example.com/shop/p/widget">Widget</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("category misses %q", want)
		}
	}
	for _, target := range []string{"/shop/c/nope", "/shop/c/hidden"} {
		if rec := getPublic(t, f.ws, f.db, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
}

func TestPublicProduct(t *testing.T) {
	f := newShopFixture(t)
	_, stored := seedCommerce(t, f)
	imgURL := "http://example.com" + "/p/img/product/widget/" + files.Hash(stored)
	rec := getPublic(t, f.ws, f.db, "/shop/p/widget")
	if rec.Code != http.StatusOK {
		t.Fatalf("product: %d", rec.Code)
	}
	body := rec.Body.String()
	long := strings.Repeat("A fine widget for every workshop. ", 10)
	for _, want := range []string{
		"<title>Widget — Acme</title>",
		`<meta name="description" content="` + publicSnippet(long, 160) + `">`,
		`<link rel="canonical" href="http://example.com/shop/p/widget">`,
		`<meta property="og:image" content="` + imgURL + `">`,
		`<meta name="twitter:image" content="` + imgURL + `">`,
		`<img src="` + imgURL + `" alt="A shiny widget"`,
		`12.30 USD`,
		`<a href="http://example.com/shop/c/tools">Tools</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("product misses %q", want)
		}
	}
	if strings.Contains(body, "SECRET-DO-NOT-PUBLISH") {
		t.Fatal("product leaks the secret field")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(stored))
	if err != nil {
		t.Fatal(err)
	}
	if want := `width="` + strconv.Itoa(cfg.Width) + `" height="` + strconv.Itoa(cfg.Height) + `"`; !strings.Contains(body, want) {
		t.Fatalf("product image misses dimensions %q", want)
	}
	graph := graphNodes(t, body)
	product := graphNode(graph, "Product")
	if product == nil {
		t.Fatal("product JSON-LD misses the Product node")
	}
	offers, _ := product["offers"].(map[string]any)
	if offers["price"] != "12.30" || offers["priceCurrency"] != "USD" {
		t.Fatalf("offers = %v", offers)
	}
	if product["image"] != imgURL {
		t.Fatalf("product image = %v", product["image"])
	}
	crumbs, _ := graphNode(graph, "BreadcrumbList")["itemListElement"].([]any)
	if len(crumbs) != 4 {
		t.Fatalf("breadcrumb items = %d", len(crumbs))
	}
	for _, target := range []string{"/shop/p/nope", "/shop/p/hidden", "/shop/p/gone"} {
		if rec := getPublic(t, f.ws, f.db, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
}

func TestPublicProductImage(t *testing.T) {
	f := newShopFixture(t)
	_, stored := seedCommerce(t, f)
	hash := files.Hash(stored)
	rec := getPublic(t, f.ws, f.db, "/p/img/product/widget/"+hash)
	if rec.Code != http.StatusOK {
		t.Fatalf("image: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), stored) {
		t.Fatal("image bytes differ")
	}
	for _, target := range []string{
		"/p/img/product/widget/" + strings.Repeat("0", 64),
		"/p/img/product/widget/notahash",
		"/p/img/product/hidden/" + hash,
		"/p/img/product/nope/" + hash,
		"/p/img/course/widget/" + hash,
	} {
		if rec := getPublic(t, f.ws, f.db, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
}

func TestPublicShopEmpty(t *testing.T) {
	f := newShopFixture(t)
	for _, target := range []string{"/shop", "/shop/c/tools", "/shop/p/widget"} {
		if rec := getPublic(t, f.ws, f.db, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s without types: %d", target, rec.Code)
		}
	}
	saveRecordType(t, f.types, "product",
		textField("name", "Name", "Text"), textField("slug", "Slug", "Text"), textField("is_published", "Published", "Checkbox"))
	if _, err := f.records.CreateRecord(context.Background(), &athenav1.CreateRecordRequest{RecordTypeKey: "product", Values: map[string]*athenav1.FieldValue{
		"name": text("Draft"), "slug": text("draft"), "is_published": shopBool(false)}}); err != nil {
		t.Fatal(err)
	}
	if rec := getPublic(t, f.ws, f.db, "/shop"); rec.Code != http.StatusNotFound {
		t.Fatalf("shop without published rows: %d", rec.Code)
	}
}

func TestPublicSitemapCommerce(t *testing.T) {
	f := newShopFixture(t)
	seedCommerce(t, f)
	rec := getPublic(t, f.ws, f.db, "/sitemap.xml")
	body := rec.Body.String()
	for _, want := range []string{
		"<loc>http://example.com/shop</loc>",
		"<loc>http://example.com/shop/c/tools</loc>",
		"<loc>http://example.com/shop/p/widget</loc>",
		"<lastmod>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("sitemap misses %q", want)
		}
	}
	for _, hidden := range []string{"/shop/c/hidden", "/shop/p/hidden", "/shop/p/gone"} {
		if strings.Contains(body, hidden) {
			t.Fatalf("sitemap leaks %q", hidden)
		}
	}
}

func TestPublicHomeShopLink(t *testing.T) {
	f := newShopFixture(t)
	if body := getPublic(t, f.ws, f.db, "/home").Body.String(); strings.Contains(body, "Visit the shop") {
		t.Fatal("home links the shop with nothing published")
	}
	seedCommerce(t, f)
	if body := getPublic(t, f.ws, f.db, "/home").Body.String(); !strings.Contains(body, `<a href="/shop">Visit the shop</a>`) {
		t.Fatal("home misses the shop link")
	}
}

func TestPublicDocumentKind(t *testing.T) {
	f := newShopFixture(t)
	ctx := context.Background()
	if _, err := f.types.SaveRecordType(ctx, &athenav1.SaveRecordTypeRequest{RecordType: &athenav1.RecordType{Key: "gadget", Label: "Gadget", Kind: "document", Class: "non_posting", Submittable: true,
		Fields: []*athenav1.RecordTypeField{textField("slug", "Slug", "Text"), textField("is_published", "Published", "Checkbox")}}}); err != nil {
		t.Fatal(err)
	}
	created, err := f.records.CreateRecord(ctx, &athenav1.CreateRecordRequest{RecordTypeKey: "gadget", Values: map[string]*athenav1.FieldValue{
		"slug": text("gadget"), "is_published": shopBool(true)}})
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := publicRead(ctx, f.db, "gadget",
		[]publicSlot{{Key: "name", Kind: "text"}, {Key: "slug", Kind: "text"}, {Key: "is_published", Kind: "checkbox"}}, nil, publicFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("draft document rows = %d, %v", len(rows), err)
	}
	if _, err := f.db.ExecContext(ctx, "UPDATE "+dialect.QuoteIdent(recordTypeTableName("gadget"))+" SET `docstate` = 2 WHERE `id` = ?", created.GetRecord().GetId()); err != nil {
		t.Fatal(err)
	}
	rows, _, err = publicRead(ctx, f.db, "gadget",
		[]publicSlot{{Key: "name", Kind: "text"}, {Key: "slug", Kind: "text"}, {Key: "is_published", Kind: "checkbox"}}, nil, publicFilter{})
	if err != nil || len(rows) != 0 {
		t.Fatalf("cancelled document rows = %d, %v", len(rows), err)
	}
}

func TestPublicSnippet(t *testing.T) {
	if got := publicSnippet("short", 160); got != "short" {
		t.Fatalf("short = %q", got)
	}
	if got := publicSnippet("a  b\n c", 160); got != "a b c" {
		t.Fatalf("whitespace = %q", got)
	}
	long := strings.Repeat("word ", 50)
	got := publicSnippet(long, 160)
	if len([]rune(got)) > 160 || strings.HasSuffix(got, " ") || !strings.HasPrefix(long, got) {
		t.Fatalf("cut = %q", got)
	}
}
