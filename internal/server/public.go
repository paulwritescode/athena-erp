package server

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"athena-erp/internal/files"
)

//go:embed public/*.html
var publicTemplates embed.FS

//go:embed public/og-fallback.png
var ogFallbackPNG []byte

// publicLayout is the shared shell every public page renders inside: the SEO
// head block, the Workspace header and the footer. Each page clones it and
// adds its own "content" block, because one parsed set cannot hold several
// blocks of the same name.
var publicLayout = template.Must(template.ParseFS(publicTemplates, "public/layout.html"))

// publicPageTemplates holds one layout clone per public page.
var publicPageTemplates = map[string]*template.Template{
	"home":         mustPublicPage("home.html"),
	"notfound":     mustPublicPage("notfound.html"),
	"shop":         mustPublicPage("shop.html"),
	"category":     mustPublicPage("category.html"),
	"product":      mustPublicPage("product.html"),
	"courses":      mustPublicPage("courses.html"),
	"verify":       mustPublicPage("verify.html"),
	"verifyresult": mustPublicPage("verifyresult.html"),
}

func mustPublicPage(file string) *template.Template {
	t, err := publicLayout.Clone()
	if err != nil {
		panic(err)
	}
	return template.Must(t.ParseFS(publicTemplates, "public/"+file))
}

// publicHandler serves the crawler-facing public pages from ADR-0030: the
// Workspace home, the per-Workspace sitemap and robots files, and (slices 3
// and 4) the commerce and catalogue pages. Public handlers ignore sessions
// and cookies entirely; publication is by content, never by login.
type publicHandler struct {
	db    *sql.DB
	name  string
	files *files.Store
}

func newPublicHandler(workspace *Workspace, db *sql.DB) publicHandler {
	h := publicHandler{db: db, name: "Athena"}
	if workspace != nil && workspace.Name != "" {
		h.name = workspace.Name
	}
	if workspace == nil || workspace.Root == "" || db == nil {
		return h
	}
	if store, err := files.Open(workspace.Root, db, dialect); err == nil {
		h.files = store
	}
	return h
}

// mountPublic registers the public routes. Explicit patterns win over the
// "/" SPA catch-all in the Go 1.22 mux, so registration order is free.
func mountPublic(mux *http.ServeMux, workspace *Workspace, db *sql.DB) {
	h := newPublicHandler(workspace, db)
	mux.HandleFunc("GET /home", h.home)
	mux.HandleFunc("GET /shop", h.shop)
	mux.HandleFunc("GET /shop/c/{slug}", h.category)
	mux.HandleFunc("GET /shop/p/{slug}", h.product)
	mux.HandleFunc("GET /courses", h.courses)
	mux.HandleFunc("GET /verify", h.verify)
	mux.HandleFunc("GET /verify/{token}", h.verifyResult)
	mux.HandleFunc("GET /p/img/{kind}/{slug}/{hash}", h.publicImage)
	mux.HandleFunc("GET /p/og-fallback.png", h.ogFallback)
	mux.HandleFunc("GET /sitemap.xml", h.sitemap)
	mux.HandleFunc("GET /robots.txt", h.robots)
}

// deskNoIndex marks Desk responses as not for search engines. The SPA entry,
// sign-in and every Desk path serve through the wrapped handler; hashed
// assets under /assets/ keep no header (they are not pages).
func deskNoIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		}
		next.ServeHTTP(w, r)
	})
}

// publicPage is the data every public template receives. Title, Description
// and Canonical form the per-page SEO contract; Image is the absolute
// og:image URL (the page image, else the logo, else the 1200x630 fallback);
// JSONLD is encoder output only, never raw content (see publicJSONLD).
type publicPage struct {
	Title       string
	Description string
	Canonical   string
	Image       string
	ImageWidth  string
	ImageHeight string
	Favicon32   string
	Favicon180  string
	JSONLD      template.JS
	Name        string
	LogoURL     string
	Base        string
	Crumbs      []publicCrumb
	ShopURL     string
	CoursesURL  string
}

// publicJSONLD encodes v for the application/ld+json block. encoding/json
// escapes <, > and & by construction, so no value can break out of the
// script element; the result is template.JS because html/template emits any
// other type in a script context as a quoted string, which would invalidate
// the structured data. This is the only trusted-script value on public
// pages, and it is encoder output only, never raw content.
func publicJSONLD(v any) template.JS {
	raw, err := json.Marshal(v)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(string(raw))
}

// base starts a page for path with the resolved public origin.
func (h publicHandler) base(r *http.Request, path string) publicPage {
	base := h.baseURL(r)
	return publicPage{Base: base, Canonical: base + path}
}

// baseURL returns the public origin: the published public.base_url setting
// when it holds a valid http(s) origin, else the request's own scheme and
// host. While the server is loopback-only the fallback is a placeholder.
func (h publicHandler) baseURL(r *http.Request) string {
	if h.db != nil {
		if base := publicBaseSetting(r.Context(), h.db); base != "" {
			return base
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// publicBaseSetting reads the published public.base_url scalar from the
// Settings chain. Anything missing or invalid yields "", and the caller
// falls back to the request host. The value is a JSON string (a bare URL is
// accepted too); only the scheme and host are kept.
func publicBaseSetting(ctx context.Context, db *sql.DB) string {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value_json FROM athena_setting_values WHERE owner_pack='athena' AND setting_type='public.base_url' AND scope_chain='' AND status='published' AND effective_from<=UTC_TIMESTAMP(3) ORDER BY effective_from DESC,revision DESC LIMIT 1`).Scan(&raw)
	if err != nil {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		value = strings.TrimSpace(string(raw))
	}
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// renderPublic writes a page with the HTML content type. It renders into a
// buffer first so a render failure answers 500 instead of half a page. Data
// is publicPage or a struct embedding it.
func renderPublic(w http.ResponseWriter, page string, data any) {
	t, ok := publicPageTemplates[page]
	if !ok {
		publicError(w)
		return
	}
	var body strings.Builder
	if err := t.ExecuteTemplate(&body, "layout", data); err != nil {
		log.Printf("public: render %s failed", page)
		publicError(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body.String()))
}

// publicError answers a generic 500. The log carries the route only: no
// Record values, credentials or session material ever reach public logs.
func publicError(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}

// notFound answers a bare 404 that hints at nothing. Unknown slugs and
// tokens (slices 3 and 4) share it.
func (h publicHandler) notFound(w http.ResponseWriter, r *http.Request) {
	page := h.base(r, "/home")
	page.Canonical = ""
	page.Name = h.name
	page.Title = "Nothing here"
	page.Description = "That page does not exist."
	page.JSONLD = publicJSONLD(map[string]any{
		"@context": "https://schema.org",
		"@type":    "WebPage",
		"name":     page.Title,
	})
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	t := publicPageTemplates["notfound"]
	var body strings.Builder
	if err := t.ExecuteTemplate(&body, "layout", page); err != nil {
		log.Printf("public: render notfound failed")
		publicError(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(body.String()))
}

// home serves the Workspace home page: the Workspace name, its logo, and the
// Organization structured data. Without a database it renders the fallback
// name; a database failure answers 500.
func (h publicHandler) home(w http.ResponseWriter, r *http.Request) {
	doc, err := h.publicIdentity(r.Context())
	if err != nil {
		log.Printf("public: home identity read failed")
		publicError(w)
		return
	}
	page := h.base(r, "/home")
	applyBrand(&page, h.name, doc)
	page.Title = page.Name
	// Meta description source: composed from the display name. The Workspace
	// holds no tagline field, so nothing else can source it.
	page.Description = "Welcome to " + page.Name + "."
	page.ShopURL = h.shopURLFor(r.Context())
	page.CoursesURL = h.coursesURLFor(r.Context())
	org := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Organization",
		"name":     page.Name,
		"url":      page.Canonical,
	}
	// The logo is the Workspace mark only; the og:image fallback chain in
	// page.Image must never stand in for it.
	if page.LogoURL != "" {
		org["logo"] = page.Base + page.LogoURL
	}
	page.JSONLD = publicJSONLD(org)
	renderPublic(w, "home", page)
}

// publicSitemapURL is one sitemap entry. LastMod stays empty until slices 3
// and 4 add published rows with modified timestamps.
type publicSitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type publicSitemapDoc struct {
	XMLName xml.Name           `xml:"urlset"`
	Xmlns   string             `xml:"xmlns,attr"`
	URLs    []publicSitemapURL `xml:"url"`
}

// sitemap serves the per-Workspace sitemap. It lists only live URLs: /home
// always, published commerce URLs with their modified dates, and /courses
// when the catalogue has content. A projection failure degrades to the home
// entry.
func (h publicHandler) sitemap(w http.ResponseWriter, r *http.Request) {
	base := h.baseURL(r)
	urls := []publicSitemapURL{{Loc: base + "/home"}}
	if h.db != nil {
		urls = append(urls, h.sitemapCommerce(r, base)...)
		urls = append(urls, h.sitemapLearn(r, base)...)
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(publicSitemapDoc{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls})
}

// ogFallback serves the neutral 1200x630 image behind every og:image tag
// whose page has no image of its own. The bytes are fixed at build time, so
// answers cache for a day; crawlers may index it (it backs og:image).
func (h publicHandler) ogFallback(w http.ResponseWriter, _ *http.Request) {
	header := w.Header()
	header.Set("Content-Type", "image/png")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(ogFallbackPNG)
}

// robots serves the per-Workspace crawlers file: the Desk, token routes,
// telemetry and RPC paths stay out, and the sitemap is advertised.
func (h publicHandler) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "User-agent: *\nDisallow: /build/\nDisallow: /view/\nDisallow: /verify/\nDisallow: /otlp/\nDisallow: /athena.v1.\n\nSitemap: %s/sitemap.xml\n", h.baseURL(r))
}
