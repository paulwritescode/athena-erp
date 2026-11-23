package server

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"athena-erp/internal/brand"
	"athena-erp/internal/files"
	"athena-erp/internal/ledger"
)

// Commerce pages from ADR-0030: the storefront, category and product pages,
// reading only through the fixed projections below. Carts, checkout and
// tracking do not exist; when they land they send noindex from the start.

var (
	shopCategoryRequired = []publicSlot{{Key: "name", Kind: "text"}, {Key: "slug", Kind: "text"}, {Key: "is_published", Kind: "checkbox"}}
	shopCategoryOptional = []publicSlot{{Key: "description", Kind: "text"}}
	shopProductRequired  = []publicSlot{{Key: "name", Kind: "text"}, {Key: "slug", Kind: "text"}, {Key: "is_published", Kind: "checkbox"}}
	shopProductOptional  = []publicSlot{{Key: "description", Kind: "text"}, {Key: "image", Kind: "image"}, {Key: "price", Kind: "money"}, {Key: "category", Kind: "link"}}
)

// publicCrumb is one visible breadcrumb; crumbs never include the current page.
type publicCrumb struct {
	Name string
	URL  string
}

// publicCategoryView is one category on the storefront.
type publicCategoryView struct {
	Name        string
	URL         string
	Description string
	Count       int
}

// publicProductView is one product on the storefront, a category page or its
// own page. Price reads "12.30 USD" and stays empty without a price.
type publicProductView struct {
	Name        string
	URL         string
	Description string
	Price       string
	ImageURL    string
	ImageAlt    string
	HasImage    bool
	Category    string
	CategoryURL string
	HasCategory bool
}

type shopPageData struct {
	publicPage
	Categories []publicCategoryView
	Products   []publicProductView
}

type categoryPageData struct {
	publicPage
	Category publicCategoryView
	Products []publicProductView
}

type productPageData struct {
	publicPage
	Product     publicProductView
	ImageWidth  int
	ImageHeight int
}

// publicIdentity reads the Workspace identity for brand data. Without a
// database it yields the zero document.
func (h publicHandler) publicIdentity(ctx context.Context) (ledger.IdentityDoc, error) {
	if h.db == nil {
		return ledger.IdentityDoc{}, nil
	}
	svc := ledger.Service{DB: h.db, Dialect: dialect}
	return svc.GetIdentityDoc(ctx)
}

// applyBrand fills the shared brand slots: the display name (or the
// fallback), the logo with the light side preferred, the favicons, and the
// og:image chain (the logo, else the neutral 1200x630 fallback) when the
// page has no image of its own. Only the fallback claims dimensions.
func applyBrand(page *publicPage, fallback string, doc ledger.IdentityDoc) {
	name := doc.DisplayName
	if name == "" {
		name = fallback
	}
	page.Name = name
	if doc.Light != nil {
		page.LogoURL = brand.LogoURL(doc.Light.Hash, doc.Light.Ext)
	} else if doc.Dark != nil {
		page.LogoURL = brand.LogoURL(doc.Dark.Hash, doc.Dark.Ext)
	}
	if page.Image == "" && page.LogoURL != "" {
		page.Image = page.Base + page.LogoURL
	}
	if page.Image == "" {
		page.Image = page.Base + "/p/og-fallback.png"
		page.ImageWidth = "1200"
		page.ImageHeight = "630"
	}
	favHash := brand.MarkHash
	if doc.Light != nil && doc.Light.Ext == "png" {
		favHash = doc.Light.Hash
	}
	page.Favicon32 = brand.FaviconURL(favHash, brand.FaviconSmall)
	page.Favicon180 = brand.FaviconURL(favHash, brand.FaviconLarge)
}

// publicBreadcrumbs builds the BreadcrumbList node for absolute URLs.
func publicBreadcrumbs(items ...publicCrumb) map[string]any {
	elements := make([]any, len(items))
	for i, c := range items {
		elements[i] = map[string]any{"@type": "ListItem", "position": i + 1, "name": c.Name, "item": c.URL}
	}
	return map[string]any{"@type": "BreadcrumbList", "itemListElement": elements}
}

// publicGraph wraps structured-data nodes in one ld+json @graph.
func publicGraph(nodes ...map[string]any) map[string]any {
	elements := make([]any, len(nodes))
	for i, n := range nodes {
		elements[i] = n
	}
	return map[string]any{"@context": "https://schema.org", "@graph": elements}
}

// publicOrgNode builds the Organization member of an @graph (without its own
// @context). logo is the absolute logo URL, "" when the Workspace has none.
func publicOrgNode(name, url, logo string) map[string]any {
	org := map[string]any{"@type": "Organization", "name": name, "url": url}
	if logo != "" {
		org["logo"] = logo
	}
	return org
}

// shopURLFor reports whether the storefront has anything to show. A database
// failure degrades to no link; the shop page itself reports the failure.
func (h publicHandler) shopURLFor(ctx context.Context) string {
	if h.db == nil {
		return ""
	}
	rows, _, err := publicRead(ctx, h.db, "product", shopProductRequired, nil, publicFilter{Limit: 1})
	if err != nil || len(rows) == 0 {
		if err != nil {
			log.Printf("public: shop existence read failed")
		}
		return ""
	}
	return "/shop"
}

// shop serves the storefront: published categories and products. Without a
// single published product it answers 404, keeping a thin shop out of the
// index.
func (h publicHandler) shop(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	doc, err := h.publicIdentity(ctx)
	if err != nil {
		log.Printf("public: shop identity read failed")
		publicError(w)
		return
	}
	products, productDef, err := publicRead(ctx, h.db, "product", shopProductRequired, shopProductOptional, publicFilter{})
	if err != nil {
		log.Printf("public: shop products read failed")
		publicError(w)
		return
	}
	if len(products) == 0 {
		h.notFound(w, r)
		return
	}
	categories, _, err := publicRead(ctx, h.db, "product_category", shopCategoryRequired, shopCategoryOptional, publicFilter{})
	if err != nil {
		log.Printf("public: shop categories read failed")
		publicError(w)
		return
	}
	page := h.base(r, "/shop")
	applyBrand(&page, h.name, doc)
	page.Title = "Shop " + page.Name
	page.Description = "Browse everything in the " + page.Name + " shop."
	page.Crumbs = []publicCrumb{{Name: "Home", URL: page.Base + "/home"}}
	byID := map[string]publicRow{}
	for _, c := range categories {
		byID[c.ID] = c
	}
	counts := map[string]int{}
	for _, p := range products {
		if id := p.Text["category"]; id != "" {
			if _, ok := byID[id]; ok {
				counts[id]++
			}
		}
	}
	data := shopPageData{}
	for _, p := range products {
		data.Products = append(data.Products, h.productView(page.Base, productDef, p, byID))
	}
	for _, c := range categories {
		data.Categories = append(data.Categories, publicCategoryView{
			Name:        c.Text["name"],
			URL:         page.Base + "/shop/c/" + url.PathEscape(c.Text["slug"]),
			Description: publicSnippet(c.Text["description"], 160),
			Count:       counts[c.ID],
		})
	}
	data.publicPage = page
	data.JSONLD = publicJSONLD(publicGraph(
		publicOrgNode(page.Name, page.Base+"/home", pageLogo(page, doc)),
		publicBreadcrumbs(publicCrumb{Name: "Home", URL: page.Base + "/home"}, publicCrumb{Name: "Shop", URL: page.Canonical}),
	))
	renderPublic(w, "shop", data)
}

// category serves one published category with its published products.
func (h publicHandler) category(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	doc, err := h.publicIdentity(ctx)
	if err != nil {
		log.Printf("public: category identity read failed")
		publicError(w)
		return
	}
	cats, _, err := publicRead(ctx, h.db, "product_category", shopCategoryRequired, shopCategoryOptional, publicFilter{Slug: slug, Limit: 1})
	if err != nil {
		log.Printf("public: category read failed")
		publicError(w)
		return
	}
	if len(cats) == 0 {
		h.notFound(w, r)
		return
	}
	products, productDef, err := publicRead(ctx, h.db, "product", shopProductRequired, shopProductOptional, publicFilter{})
	if err != nil {
		log.Printf("public: category products read failed")
		publicError(w)
		return
	}
	cat := cats[0]
	page := h.base(r, "/shop/c/"+url.PathEscape(cat.Text["slug"]))
	applyBrand(&page, h.name, doc)
	page.Title = cat.Text["name"] + " — " + page.Name
	page.Description = cat.Text["name"] + " at " + page.Name + "."
	if d := publicSnippet(cat.Text["description"], 160); d != "" {
		page.Description = d
	}
	page.Crumbs = []publicCrumb{{Name: "Home", URL: page.Base + "/home"}, {Name: "Shop", URL: page.Base + "/shop"}}
	byID := map[string]publicRow{cat.ID: cat}
	data := categoryPageData{Category: publicCategoryView{
		Name:        cat.Text["name"],
		URL:         page.Canonical,
		Description: cat.Text["description"],
	}}
	for _, p := range products {
		if p.Text["category"] != cat.ID {
			continue
		}
		data.Products = append(data.Products, h.productView(page.Base, productDef, p, byID))
	}
	data.Category.Count = len(data.Products)
	data.publicPage = page
	data.JSONLD = publicJSONLD(publicGraph(
		publicOrgNode(page.Name, page.Base+"/home", pageLogo(page, doc)),
		publicBreadcrumbs(
			publicCrumb{Name: "Home", URL: page.Base + "/home"},
			publicCrumb{Name: "Shop", URL: page.Base + "/shop"},
			publicCrumb{Name: cat.Text["name"], URL: page.Canonical},
		),
	))
	renderPublic(w, "category", data)
}

// product serves one published product.
func (h publicHandler) product(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	doc, err := h.publicIdentity(ctx)
	if err != nil {
		log.Printf("public: product identity read failed")
		publicError(w)
		return
	}
	rows, productDef, err := publicRead(ctx, h.db, "product", shopProductRequired, shopProductOptional, publicFilter{Slug: slug, Limit: 1})
	if err != nil {
		log.Printf("public: product read failed")
		publicError(w)
		return
	}
	if len(rows) == 0 {
		h.notFound(w, r)
		return
	}
	categories, _, err := publicRead(ctx, h.db, "product_category", shopCategoryRequired, nil, publicFilter{})
	if err != nil {
		log.Printf("public: product categories read failed")
		publicError(w)
		return
	}
	row := rows[0]
	byID := map[string]publicRow{}
	for _, c := range categories {
		byID[c.ID] = c
	}
	page := h.base(r, "/shop/p/"+url.PathEscape(row.Text["slug"]))
	applyBrand(&page, h.name, doc)
	view := h.productView(page.Base, productDef, row, byID)
	// The page image wins over the chain applyBrand just set; its
	// dimensions are unknown, so the fallback's claim goes with it.
	if view.HasImage {
		page.Image = view.ImageURL
		page.ImageWidth = ""
		page.ImageHeight = ""
	}
	page.Title = row.Text["name"] + " — " + page.Name
	page.Description = row.Text["name"] + " at " + page.Name + "."
	if d := publicSnippet(row.Text["description"], 160); d != "" {
		page.Description = d
	}
	page.Crumbs = []publicCrumb{{Name: "Home", URL: page.Base + "/home"}, {Name: "Shop", URL: page.Base + "/shop"}}
	crumbs := []publicCrumb{{Name: "Home", URL: page.Base + "/home"}, {Name: "Shop", URL: page.Base + "/shop"}}
	if view.HasCategory {
		page.Crumbs = append(page.Crumbs, publicCrumb{Name: view.Category, URL: view.CategoryURL})
		crumbs = append(crumbs, publicCrumb{Name: view.Category, URL: view.CategoryURL})
	}
	crumbs = append(crumbs, publicCrumb{Name: row.Text["name"], URL: page.Canonical})
	product := map[string]any{"@type": "Product", "name": row.Text["name"], "url": page.Canonical}
	if row.Text["description"] != "" {
		product["description"] = publicSnippet(row.Text["description"], 500)
	}
	if view.HasImage {
		product["image"] = view.ImageURL
	}
	if minor, ok := row.Money["price"]; ok {
		if text, currency := publicMoney(productDef, "price", minor); text != "" && currency != "" {
			product["offers"] = map[string]any{"@type": "Offer", "price": text, "priceCurrency": currency}
		}
	}
	data := productPageData{Product: view}
	if ref, ok := row.Images["image"]; ok {
		data.ImageWidth, data.ImageHeight = h.publicImageSize(ctx, ref)
	}
	data.publicPage = page
	data.JSONLD = publicJSONLD(publicGraph(
		publicOrgNode(page.Name, page.Base+"/home", pageLogo(page, doc)),
		product,
		publicBreadcrumbs(crumbs...),
	))
	renderPublic(w, "product", data)
}

// productView renders one published product row into its card. Category names
// resolve from published categories only; IDs never leave the server.
func (h publicHandler) productView(base string, def *recordTypeDefinition, row publicRow, categories map[string]publicRow) publicProductView {
	view := publicProductView{
		Name:        row.Text["name"],
		URL:         base + "/shop/p/" + url.PathEscape(row.Text["slug"]),
		Description: publicSnippet(row.Text["description"], 160),
	}
	if minor, ok := row.Money["price"]; ok {
		if text, currency := publicMoney(def, "price", minor); text != "" {
			view.Price = text
			if currency != "" {
				view.Price += " " + currency
			}
		}
	}
	if ref, ok := row.Images["image"]; ok {
		view.ImageURL = base + "/p/img/product/" + url.PathEscape(row.Text["slug"]) + "/" + ref.Hash
		view.ImageAlt = ref.AltText
		view.HasImage = true
	}
	if id := row.Text["category"]; id != "" {
		if c, ok := categories[id]; ok {
			view.Category = c.Text["name"]
			view.CategoryURL = base + "/shop/c/" + url.PathEscape(c.Text["slug"])
			view.HasCategory = true
		}
	}
	return view
}

// pageLogo returns the absolute logo URL for structured data, "" without one.
func pageLogo(page publicPage, doc ledger.IdentityDoc) string {
	if doc.Light != nil {
		return page.Base + brand.LogoURL(doc.Light.Hash, doc.Light.Ext)
	}
	if doc.Dark != nil {
		return page.Base + brand.LogoURL(doc.Dark.Hash, doc.Dark.Ext)
	}
	return ""
}

// publicImageSize decodes the stored image dimensions for width/height
// attributes. Anything unreadable yields zeros and the template omits them.
func (h publicHandler) publicImageSize(ctx context.Context, ref imageReference) (int, int) {
	if h.files == nil {
		return 0, 0
	}
	_, content, err := h.files.Get(ctx, ref.Path, 0)
	if err != nil {
		return 0, 0
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

var publicImageHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// publicImageTypes pairs each image kind with the Record Type and slots
// whose published rows may serve images.
var publicImageTypes = map[string]struct {
	key      string
	required []publicSlot
	optional []publicSlot
}{
	"product": {key: "product", required: shopProductRequired, optional: shopProductOptional},
	"course":  {key: "course", required: learnCourseRequired, optional: learnCourseOptional},
}

// publicImage serves the image of one published row. The hash pins the
// content (stale URLs 404), so answers carry immutable cache. Anything
// unpublished, missing or mismatched is a bare 404.
func (h publicHandler) publicImage(w http.ResponseWriter, r *http.Request) {
	kind, slug, hash := r.PathValue("kind"), r.PathValue("slug"), r.PathValue("hash")
	imageType, ok := publicImageTypes[kind]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !publicImageHash.MatchString(hash) || h.db == nil || h.files == nil {
		http.NotFound(w, r)
		return
	}
	rows, _, err := publicRead(r.Context(), h.db, imageType.key, imageType.required, imageType.optional, publicFilter{Slug: slug, Limit: 1})
	if err != nil {
		log.Printf("public: image read failed")
		publicError(w)
		return
	}
	if len(rows) == 0 {
		http.NotFound(w, r)
		return
	}
	ref, ok := rows[0].Images["image"]
	if !ok || ref.Hash != hash {
		http.NotFound(w, r)
		return
	}
	_, content, err := h.files.Get(r.Context(), ref.Path, 0)
	if err != nil || files.Hash(content) != hash {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Content-Type", http.DetectContentType(content))
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "public, max-age=31536000, immutable")
	if strings.HasSuffix(strings.ToLower(ref.Path), ".svg") {
		header.Set("Content-Security-Policy", "sandbox")
	}
	_, _ = w.Write(content)
}

// sitemapCommerce lists the storefront and every published category and
// product with their modified dates. Any failure yields nothing: the
// sitemap degrades to the home entry rather than failing.
func (h publicHandler) sitemapCommerce(r *http.Request, base string) []publicSitemapURL {
	ctx := r.Context()
	products, _, err := publicRead(ctx, h.db, "product", shopProductRequired, nil, publicFilter{})
	if err != nil || len(products) == 0 {
		if err != nil {
			log.Printf("public: sitemap products read failed")
		}
		return nil
	}
	urls := []publicSitemapURL{{Loc: base + "/shop"}}
	categories, _, err := publicRead(ctx, h.db, "product_category", shopCategoryRequired, nil, publicFilter{})
	if err != nil {
		log.Printf("public: sitemap categories read failed")
		return nil
	}
	for _, c := range categories {
		urls = append(urls, publicSitemapURL{Loc: base + "/shop/c/" + url.PathEscape(c.Text["slug"]), LastMod: publicLastMod(c)})
	}
	for _, p := range products {
		urls = append(urls, publicSitemapURL{Loc: base + "/shop/p/" + url.PathEscape(p.Text["slug"]), LastMod: publicLastMod(p)})
	}
	return urls
}

// publicLastMod renders a row's modified date for sitemaps, "" when unknown.
func publicLastMod(row publicRow) string {
	if row.Updated.IsZero() {
		return ""
	}
	return row.Updated.UTC().Format("2006-01-02")
}
