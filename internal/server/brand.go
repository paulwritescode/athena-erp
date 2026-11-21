package server

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"

	"athena-erp/internal/brand"
	"athena-erp/internal/files"
	"athena-erp/internal/ledger"
)

// brandHandler serves the public brand routes: content-hashed logos and
// favicons for the sign-in page, the header and the browser tab. Logo bytes
// need the database (the identity) and the Workspace directory; the Athena
// mark favicons need neither. Unknown hashes answer 404 without saying what
// exists.
type brandHandler struct {
	db    *sql.DB
	files *files.Store
}

func newBrandHandler(workspace *Workspace, db *sql.DB) brandHandler {
	h := brandHandler{db: db}
	if workspace == nil || workspace.Root == "" || db == nil {
		return h
	}
	if store, err := files.Open(workspace.Root, db, dialect); err == nil {
		h.files = store
	}
	return h
}

var (
	logoRoute    = regexp.MustCompile(`^logo-([0-9a-f]{64})\.(png|svg)$`)
	faviconRoute = regexp.MustCompile(`^favicon-([0-9a-f]{64}|mark)-(32|180)\.png$`)
)

func (h brandHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if m := logoRoute.FindStringSubmatch(name); m != nil {
		h.logo(w, r, m[1], m[2])
		return
	}
	if m := faviconRoute.FindStringSubmatch(name); m != nil {
		size, _ := strconv.Atoi(m[2])
		h.favicon(w, r, m[1], size)
		return
	}
	http.NotFound(w, r)
}

// logo serves the stored logo whose hash and extension match.
func (h brandHandler) logo(w http.ResponseWriter, r *http.Request, hash, ext string) {
	if h.db == nil || h.files == nil {
		http.NotFound(w, r)
		return
	}
	svc := ledger.Service{DB: h.db, Dialect: dialect}
	doc, err := svc.GetIdentityDoc(r.Context())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	side := ""
	for _, candidate := range []struct {
		name string
		ref  *ledger.LogoRef
	}{{"light", doc.Light}, {"dark", doc.Dark}} {
		if candidate.ref != nil && candidate.ref.Hash == hash && candidate.ref.Ext == ext {
			side = candidate.name
			break
		}
	}
	if side == "" {
		http.NotFound(w, r)
		return
	}
	_, content, err := h.files.Get(r.Context(), "brand/logo-"+side+"."+ext, 0)
	if err != nil || files.Hash(content) != hash {
		http.NotFound(w, r)
		return
	}
	if ext == "svg" {
		serveBrand(w, r, content, "image/svg+xml", true)
		return
	}
	serveBrand(w, r, content, "image/png", false)
}

// favicon serves the mark favicon, or the light PNG logo resized. An SVG
// light logo keeps the mark: the server cannot rasterize SVG.
func (h brandHandler) favicon(w http.ResponseWriter, r *http.Request, hash string, size int) {
	if hash == brand.MarkHash {
		content, err := brand.MarkFavicon(size)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		serveBrand(w, r, content, "image/png", false)
		return
	}
	if h.db == nil || h.files == nil {
		http.NotFound(w, r)
		return
	}
	svc := ledger.Service{DB: h.db, Dialect: dialect}
	doc, err := svc.GetIdentityDoc(r.Context())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if doc.Light == nil || doc.Light.Ext != "png" || doc.Light.Hash != hash {
		http.NotFound(w, r)
		return
	}
	_, content, err := h.files.Get(r.Context(), "brand/logo-light.png", 0)
	if err != nil || files.Hash(content) != hash {
		http.NotFound(w, r)
		return
	}
	resized, err := brand.Favicon(content, size)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	serveBrand(w, r, resized, "image/png", false)
}

// serveBrand writes brand bytes with a year of immutable cache. SVG answers
// carry a fully sandboxed content security policy, so a logo can never run
// script even if sanitizing missed something.
func serveBrand(w http.ResponseWriter, r *http.Request, content []byte, contentType string, sandbox bool) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	if sandbox {
		h.Set("Content-Security-Policy", "sandbox")
	}
	_, _ = w.Write(content)
}
