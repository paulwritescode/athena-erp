package server

import (
	"bytes"
	"context"
	"image/png"
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

func brandFixture(t *testing.T) (*brandHandler, *ledger.Service, *files.Store) {
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
	h := newBrandHandler(&Workspace{Root: root}, db)
	return &h, &ledger.Service{DB: db, Dialect: dialect}, st
}

func seedLogo(t *testing.T, svc *ledger.Service, st *files.Store, side string, logo brand.Logo) {
	t.Helper()
	if _, err := st.Put(context.Background(), "brand/logo-"+side+"."+logo.Ext, logo.Bytes, files.Expect{}, files.Meta{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	doc, err := svc.GetIdentityDoc(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ref := ledger.LogoRef{Ext: logo.Ext, Hash: logo.Hash}
	if side == "light" {
		doc.Light = &ref
	} else {
		doc.Dark = &ref
	}
	if err := svc.SaveIdentityDoc(context.Background(), doc, "test"); err != nil {
		t.Fatal(err)
	}
}

func getBrand(t *testing.T, h *brandHandler, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /brand/{name}", h)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func requireBrandHeaders(t *testing.T, rec *httptest.ResponseRecorder, contentType string) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != contentType {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache = %q", got)
	}
}

func TestBrandLogos(t *testing.T) {
	h, svc, st := brandFixture(t)
	png := testLogoPNG(t, 11)
	logo, err := brand.ValidateLogo("logo.png", png)
	if err != nil {
		t.Fatal(err)
	}
	seedLogo(t, svc, st, "light", logo)
	svg, err := brand.ValidateLogo("logo.svg", []byte(testLogoSVG))
	if err != nil {
		t.Fatal(err)
	}
	seedLogo(t, svc, st, "dark", svg)

	rec := getBrand(t, h, brand.LogoURL(logo.Hash, "png"))
	if rec.Code != http.StatusOK {
		t.Fatalf("logo PNG: %d", rec.Code)
	}
	requireBrandHeaders(t, rec, "image/png")
	if !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatal("logo bytes differ")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "" {
		t.Fatalf("PNG CSP = %q", csp)
	}

	rec = getBrand(t, h, brand.LogoURL(svg.Hash, "svg"))
	if rec.Code != http.StatusOK {
		t.Fatalf("logo SVG: %d", rec.Code)
	}
	requireBrandHeaders(t, rec, "image/svg+xml")
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "sandbox" {
		t.Fatalf("SVG CSP = %q", csp)
	}

	for _, target := range []string{
		brand.LogoURL(strings.Repeat("0", 64), "png"),
		"/brand/logo-xyz.png",
		"/brand/logo-" + logo.Hash + ".jpg",
		"/brand/nope.png",
		"/brand/favicon-mark-16.png",
		"/brand/favicon-" + logo.Hash + "-64.png",
	} {
		if rec := getBrand(t, h, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
}

func TestBrandFavicons(t *testing.T) {
	h, svc, st := brandFixture(t)

	for _, size := range []int{brand.FaviconSmall, brand.FaviconLarge} {
		rec := getBrand(t, h, brand.FaviconURL(brand.MarkHash, size))
		if rec.Code != http.StatusOK {
			t.Fatalf("mark %d: %d", size, rec.Code)
		}
		requireBrandHeaders(t, rec, "image/png")
		want, err := brand.MarkFavicon(size)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(rec.Body.Bytes(), want) {
			t.Fatalf("mark %d bytes differ", size)
		}
	}

	logoPNG := testLogoPNG(t, 12)
	logo, err := brand.ValidateLogo("logo.png", logoPNG)
	if err != nil {
		t.Fatal(err)
	}
	seedLogo(t, svc, st, "light", logo)
	rec := getBrand(t, h, brand.FaviconURL(logo.Hash, brand.FaviconSmall))
	if rec.Code != http.StatusOK {
		t.Fatalf("favicon: %d", rec.Code)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil || cfg.Width != brand.FaviconSmall || cfg.Height != brand.FaviconSmall {
		t.Fatalf("favicon dims: %+v %v", cfg, err)
	}

	svg, err := brand.ValidateLogo("logo.svg", []byte(testLogoSVG))
	if err != nil {
		t.Fatal(err)
	}
	seedLogo(t, svc, st, "light", svg)
	if rec := getBrand(t, h, brand.FaviconURL(svg.Hash, brand.FaviconSmall)); rec.Code != http.StatusNotFound {
		t.Fatalf("SVG favicon: %d", rec.Code)
	}
}

func TestBrandHandlerWithoutDatabase(t *testing.T) {
	h := newBrandHandler(nil, nil)
	rec := getBrand(t, &h, brand.FaviconURL(brand.MarkHash, brand.FaviconSmall))
	if rec.Code != http.StatusOK {
		t.Fatalf("mark favicon: %d", rec.Code)
	}
	if rec := getBrand(t, &h, brand.LogoURL(strings.Repeat("0", 64), "png")); rec.Code != http.StatusNotFound {
		t.Fatalf("logo without db: %d", rec.Code)
	}
}
