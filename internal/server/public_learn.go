package server

import (
	"context"
	"log"
	"net/http"
	"net/url"
)

// Catalogue pages from ADR-0030: the course catalogue and transcript
// verification, reading only through the fixed projections below. The
// catalogue has no detail pages; each course appears on the one index.

var (
	learnCourseRequired  = []publicSlot{{Key: "name", Kind: "text"}, {Key: "slug", Kind: "text"}, {Key: "is_published", Kind: "checkbox"}}
	learnCourseOptional  = []publicSlot{{Key: "description", Kind: "text"}, {Key: "image", Kind: "image"}}
	learnTranscriptSlots = []publicSlot{{Key: "verification_token", Kind: "text"}, {Key: "holder_name", Kind: "text"}, {Key: "course_name", Kind: "text"}, {Key: "status", Kind: "text"}, {Key: "is_published", Kind: "checkbox"}}
	learnTranscriptDates = []publicSlot{{Key: "completed_on", Kind: "date"}}
)

// publicCourseView is one course on the catalogue page.
type publicCourseView struct {
	Name        string
	Description string
	ImageURL    string
	ImageAlt    string
	HasImage    bool
}

type coursePageData struct {
	publicPage
	Courses []publicCourseView
}

// publicTranscriptView is one verified transcript. The token that found it is
// never rendered back.
type publicTranscriptView struct {
	HolderName  string
	CourseName  string
	CompletedOn string
	Status      string
	HasDate     bool
}

type verifyResultData struct {
	publicPage
	Transcript publicTranscriptView
}

// coursesURLFor reports whether the catalogue has anything to show. A
// database failure degrades to no link; the page itself reports the failure.
func (h publicHandler) coursesURLFor(ctx context.Context) string {
	if h.db == nil {
		return ""
	}
	rows, _, err := publicRead(ctx, h.db, "course", learnCourseRequired, nil, publicFilter{Limit: 1})
	if err != nil || len(rows) == 0 {
		if err != nil {
			log.Printf("public: courses existence read failed")
		}
		return ""
	}
	return "/courses"
}

// courses serves the catalogue: every published course on one index page.
// Without a single published course it answers 404, keeping a thin page out
// of the index.
func (h publicHandler) courses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	doc, err := h.publicIdentity(ctx)
	if err != nil {
		log.Printf("public: courses identity read failed")
		publicError(w)
		return
	}
	rows, _, err := publicRead(ctx, h.db, "course", learnCourseRequired, learnCourseOptional, publicFilter{})
	if err != nil {
		log.Printf("public: courses read failed")
		publicError(w)
		return
	}
	if len(rows) == 0 {
		h.notFound(w, r)
		return
	}
	page := h.base(r, "/courses")
	applyBrand(&page, h.name, doc)
	page.Title = "Courses — " + page.Name
	page.Description = "Browse courses at " + page.Name + "."
	page.Crumbs = []publicCrumb{{Name: "Home", URL: page.Base + "/home"}}
	data := coursePageData{}
	items := make([]any, 0, len(rows))
	for i, row := range rows {
		view := publicCourseView{
			Name:        row.Text["name"],
			Description: publicSnippet(row.Text["description"], 160),
		}
		node := map[string]any{"@type": "Course", "name": row.Text["name"]}
		if row.Text["description"] != "" {
			node["description"] = publicSnippet(row.Text["description"], 500)
		}
		if ref, ok := row.Images["image"]; ok {
			view.ImageURL = page.Base + "/p/img/course/" + url.PathEscape(row.Text["slug"]) + "/" + ref.Hash
			view.ImageAlt = ref.AltText
			view.HasImage = true
			node["image"] = view.ImageURL
		}
		data.Courses = append(data.Courses, view)
		items = append(items, map[string]any{"@type": "ListItem", "position": i + 1, "item": node})
	}
	data.publicPage = page
	data.JSONLD = publicJSONLD(publicGraph(
		publicOrgNode(page.Name, page.Base+"/home", pageLogo(page, doc)),
		map[string]any{"@type": "ItemList", "itemListElement": items},
	))
	renderPublic(w, "courses", data)
}

// verify serves the transcript verification form. A submitted token arrives
// as ?token= and redirects to the canonical noindex result URL, so the form
// itself never renders a result.
func (h publicHandler) verify(w http.ResponseWriter, r *http.Request) {
	if token := r.URL.Query().Get("token"); token != "" {
		http.Redirect(w, r, "/verify/"+url.PathEscape(token), http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	doc, err := h.publicIdentity(ctx)
	if err != nil {
		log.Printf("public: verify identity read failed")
		publicError(w)
		return
	}
	page := h.base(r, "/verify")
	applyBrand(&page, h.name, doc)
	page.Title = "Verify a transcript — " + page.Name
	page.Description = "Verify a " + page.Name + " transcript."
	page.Crumbs = []publicCrumb{{Name: "Home", URL: page.Base + "/home"}}
	page.JSONLD = publicJSONLD(publicGraph(
		publicOrgNode(page.Name, page.Base+"/home", pageLogo(page, doc)),
	))
	renderPublic(w, "verify", page)
}

// verifyResult serves one verification result for a published transcript
// whose token matches exactly. Results are personal pages: noindex, no
// canonical, and unknown or unpublished tokens are bare 404s.
func (h publicHandler) verifyResult(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	rows, _, err := publicRead(r.Context(), h.db, "transcript", learnTranscriptSlots, learnTranscriptDates, publicFilter{Token: token, Limit: 1})
	if err != nil {
		log.Printf("public: verify result read failed")
		publicError(w)
		return
	}
	if len(rows) == 0 {
		h.notFound(w, r)
		return
	}
	row := rows[0]
	page := h.base(r, "/verify/"+url.PathEscape(token))
	page.Canonical = ""
	page.Name = h.name
	page.Title = "Transcript verified — " + page.Name
	page.Description = "Verification result."
	view := publicTranscriptView{
		HolderName: row.Text["holder_name"],
		CourseName: row.Text["course_name"],
		Status:     row.Text["status"],
	}
	if date := row.Text["completed_on"]; date != "" {
		view.CompletedOn = date
		view.HasDate = true
	}
	data := verifyResultData{Transcript: view}
	data.publicPage = page
	data.JSONLD = publicJSONLD(map[string]any{
		"@context": "https://schema.org",
		"@type":    "WebPage",
		"name":     page.Title,
	})
	// The header lands before the first body write, so renderPublic sends it.
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	renderPublic(w, "verifyresult", data)
}

// sitemapLearn lists the catalogue when it has content. Verification URLs
// stay out: the form is found from the home page and results are noindex.
func (h publicHandler) sitemapLearn(r *http.Request, base string) []publicSitemapURL {
	if h.db == nil {
		return nil
	}
	rows, _, err := publicRead(r.Context(), h.db, "course", learnCourseRequired, nil, publicFilter{Limit: 1})
	if err != nil {
		log.Printf("public: sitemap courses read failed")
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	return []publicSitemapURL{{Loc: base + "/courses"}}
}
