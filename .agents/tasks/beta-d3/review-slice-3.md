# Review: D3 slice 3 — commerce pages (storefront, category, product)

- Worktree: `/Users/iampaulking/projects/duchess/athena-erp-d3-r` (detached at `9cbe313`)
- Commit reviewed: `9cbe313` ("feat(d3): slice 3 commerce pages (storefront, category, product)"), parent `919bab4` (= main)
- Diff reviewed: `git diff main...9cbe313` (10 files, all read in full)
- Brief: `.agents/tasks/beta-d3/review-brief-slice-3.md` (slice 3 only); contract ADR-0030; priors: `review-slice-2.md` (PASS), `review-slice-2-reconfirm.md` (PASS at 7f5d934)
- Reviewer workflow: `.kiro/specs/athena-beta/design.md` §5 step 4 (read-only review); AGENTS.md

Verdict: PASS

No blockers. No NEEDS_CHANGES items.

## Scope check (brief item 8)

`git diff --stat main...9cbe313` shows exactly the 10 briefed files (1154+/44-);
`--name-only` confirms no others:

- `docs/decisions/adr-0030-public-pages-and-seo.md` (+1/−1: submittable-document sentence)
- `internal/server/public.go` (routes, template map, files store, `renderPublic` any-typed, home→shared helpers, sitemap commerce)
- `internal/server/public/shop.html`, `category.html`, `product.html` (new)
- `internal/server/public/layout.html` (+9: h2/crumbs/cards CSS, crumbs nav), `home.html` (+2: shop/courses links)
- `internal/server/public_shop.go` (new, 498 lines), `internal/server/public_project.go` (new, 212 lines)
- `internal/server/public_shop_test.go` (new, 361 lines, 9 tests)

No migration file (`sysmigrations.go` untouched), no `.proto`, no `gate.go`,
no `web/**`, no `cmd/**`, `tasks.md` untouched (Z2/Z4 untouched). ADR stays
Proposed. Slice-2 files change only where the slice justifies (shared
layout/plumbing); `server.go` is untouched, so the slice-2 wiring stands.

## Findings by brief item

### 1. Routes — OK

`GET /shop`, `/shop/c/{slug}`, `/shop/p/{slug}`, `/p/img/{kind}/{slug}/{hash}`
registered in `mountPublic` (`public.go:75-78`); explicit patterns win over the
`/` SPA catch-all in the Go 1.22 mux, and nothing collides with `/`,
`/build/*`, `/view/*`, `/assets/*`, `/brand/*`, `/healthz`, `/otlp/*` or
`/athena.v1.*`. Slug lookup is an exact match through a placeholder with quoted
identifiers (`public_project.go:120-123`); table/column names come from the
saved definition, never the request. Unknown or unpublished slugs share the
slice-2 `notFound` (`public_shop.go:176,239,299`): bare 404, `X-Robots-Tag:
noindex, nofollow`, no canonical, no path echo (`public.go:202-224`, verified
live on `/shop/p/nope`, `/shop/p/hidden`, `/shop/c/nope`, `/shop/c/hidden`).
Listings cap at 200 rows, `ORDER BY id` (oldest wins on slug collision).

### 2. SEO per page — OK

Unique titles (`Shop {name}`, `{cat} — {name}`, `{product} — {name}`), unique
descriptions (composed shop string; truncated `description` + `{name} at
{ws}.` fallback), canonical from the stored slug over the resolved base
origin, OG + Twitter `summary_large_image` throughout — all observed live in
raw bytes. `og:image` is the product image when present, else the Workspace
logo, else omitted. `og:image:width/height` (`layout.html:15-16`) render only
on product pages with successfully decoded dimensions
(`public_shop.go:75-76,345-347`; zeros omit the tags) — true claims about the
served image, not premature 1200x630 claims; home/shop/category carry no
dimension claims. JSON-LD is one `@graph`: Organization + BreadcrumbList on
shop/category, plus Product (name, url, 500-rune description, image, offers
with price + priceCurrency) on product; crumb positions are 1-based with
absolute URLs; the visible breadcrumb trail never includes the current page.

### 3. Projection rule — OK

Allow-lists (`public_shop.go:25-29`) match the ADR table exactly: category
`name/slug/is_published` + `description`; product `name/slug/is_published` +
`description/image/price/category-link`. `publicRead`
(`public_project.go:60-183`): a missing type, a non-master/document kind, or a
missing/mistyped required slot publishes nothing; optional slots are skipped
unless present and correctly typed (text accepts Text/Long text only;
checkbox/money/image/link exact). Published means `is_published` TRUE and live
(masters not soft-deleted; `docstate <> 2` only for submittable documents, per
the ADR touch). Grep over the public files finds RecordService/Grants/
sessions/cookies only in comments stating they are never used, and no
`template.HTML`. Every public log line carries the route only, never values.
IDs resolve links internally and are never rendered: category names/URLs
resolve from published rows only, and a link to an unpublished category simply
yields no category block. Money renders only with a currency, and the offers
node requires both price text and currency. The image route allow-lists the
kind (product only; course 404s until slice 4), pins content by hash (URL hash
must equal the stored ref hash and the served bytes' hash), and answers
immutable cache, sniffed content type + nosniff, and a sandbox CSP for SVG
(`public_shop.go:421-463`). Live: `SECRET-DO-NOT-PUBLISH` absent from every
page; unpublished rows 404 and stay out of listings and the sitemap.

### 4. Escaping — OK (live-verified; test gap is a follow-up)

`html/template` throughout; the only trusted value is `template.JS` from
`publicJSONLD` (encoder output, `<`/`>`/`&` escaped). The test file asserts
tags but has no adversarial commerce-name test (follow-up (a)). Verified live
instead with product `<b>Bold</b> & "quoted"` /
`<script>alert(1)</script> & more`: title, meta, h1 and listings carry
`&lt;b&gt;Bold&lt;/b&gt; &amp; &#34;quoted&#34;` /
`&lt;script&gt;alert(1)&lt;/script&gt; &amp; more`, and the JSON-LD carries the
`\u003c`/`\u0026` forms, which round-trip to the same string when parsed.

### 5. Noindex / robots / sitemap — OK

`robots.txt` is unchanged (the 5 ADR disallows + sitemap line with the request
origin); the new page and image routes stay indexable, consistent with
`/brand/*`. The sitemap lists `/home`, `/shop`, and every published category
and product with `lastmod` dates, degrading to the home entry on failure
(`public_shop.go:468-490`). Carts/checkout/tracking exist nowhere: grep finds
only the forward-rule comment (`public_shop.go:21-22`) plus unrelated C4
"tracking effective dates" strings. Live headers: indexable pages carry no
robots tag; every 404 (unknown and unpublished slugs) carries
`noindex, nofollow`.

### 6. ADR touch — OK

The +1/−1 (submittable-document kernel name; `docstate` filter only where it
exists) says what the code does (`public_project.go:78-80,114-117`) and is
covered by `TestPublicDocumentKind` (draft publishes; `docstate = 2` filters).

### 7. Tests — OK, rerun green

9 new tests, all passing in this tree (see gates): shop tags/leak-check/JSON-LD
nodes, category title/desc/canonical + unknown/hidden 404s, product
tags/og:image/dimensions/offers/4 crumbs + nope/hidden/gone 404s, image
bytes/content-type/cache + 5 negative 404s (wrong hash, malformed hash, hidden
slug, unknown slug, course kind), empty-shop 404s (no types; unpublished
only), sitemap locs + leak-check, home shop-link present/absent, document-kind
draft/cancelled, snippet unit cases. Coverage is adequate for the slice except
the adversarial-escaping test (item 4, live-verified, follow-up).

### 8. Confirmations — OK

See scope check: no migration, no proto, no `gate.go`, no `web/`/`cmd/`,
Z2/Z4 untouched. The home refactor (`publicIdentity` + `applyBrand`) preserves
slice-2 bytes modulo the new shop link (verified live). `CoursesURL` is
declared and template-guarded but set nowhere — dead until slice 4 (note (e)).

### 9. Render check (throwaway Workspace, clean-tree binary) — OK

Binary stamped `vcs.revision=9cbe313`, `vcs.modified=false`. Throwaway home +
Workspace `test_athena_d3_s3_review` (slug-derived `test_athena_*` DB, dropped
afterwards) with display name and a PNG logo via the CLI, fixture
`product_category`/`product` types and rows via the CLI (published category +
product with price/link/secret field, adversarial product, unpublished rows),
served on `127.0.0.1:13211` with a fake HOME (user's dev server untouched).
Raw bytes observed:

- `/home` → 200, no robots tag: title, composed description, canonical, OG +
  Twitter, `og:image` = logo URL with no dimension claims, mark favicons,
  `Organization` JSON-LD with logo, and the new `<a href="/shop">Visit the
  shop</a>` link.
- `/shop` → 200: `Shop …` title, composed description, canonical,
  Organization + BreadcrumbList JSON-LD, category/product cards with price and
  160-rune snippets, breadcrumb nav, adversarial product fully escaped,
  unpublished rows and the secret field absent.
- `/shop/c/tools` → 200: `{cat} — {ws}` title, description from the
  projection field, canonical, Home/Shop crumbs, member products.
- `/shop/p/widget` → 200: `{product} — {ws}` title, 160-rune description,
  canonical, logo-fallback `og:image` (this product has no image), Product
  JSON-LD with `offers: {price 12.30, priceCurrency USD}` and 4 breadcrumbs.
- `/shop/p/tricky` → 200 with the escaped title/meta/h1/JSON-LD from item 4.
- Bad slugs (`/shop/p/nope`, `/shop/p/hidden`, `/shop/c/nope`, `/shop/c/hidden`)
  → 404 `text/html` + noindex, no canonical, no path echo.
- `/sitemap.xml` → home + shop + published categories/products with
  `<lastmod>`; nothing unpublished. `/robots.txt` → the 5 disallows + sitemap.
- `/` → 404 + noindex (Desk dist unbuilt in the review tree, as in round 1;
  the header comes from `deskNoIndex` either way — SPA catch-all intact).

### 10. Gates (review worktree at 9cbe313) — OK

| Gate | Observed |
| --- | --- |
| Tree before gates | clean at `9cbe313` |
| `gofmt -l cmd internal` | empty |
| `go build ./...` | ok |
| `go vet ./...` | ok |
| `affected` (absolute main-repo path, locked) | reused worker green on same commit: `ok athena-erp/cmd/athena 176.761s`, `ok athena-erp/internal/server 319.689s` (log `1123-223248-w-d3.log`, read) |
| Narrow `go test -run TestPublic ./internal/server` (no lock needed) | 17/17 PASS (9 slice-3 + 8 slice-2), MariaDB-backed, none skipped, 266s |
| `make generate` twice | both ok, `git status` clean afterwards |

(`make generate` needs node on PATH; agent shells must prepend
`~/.nvm/versions/node/v26.7.0/bin` — same fallback `suite` uses.)

## Non-blocking follow-ups (merge-small rule: none stop the merge)

(a) No adversarial escaping test for commerce names/descriptions/alt-text
(slice 2 has `TestPublicHomeEscapesName` for the home name; slice 3 has no
equivalent). Escaping is proven live in this review; suggest a test mirroring
it so the next template edit is guarded.
(b) `productPageData.ImageWidth/ImageHeight` (int, `public_shop.go:75-76`)
shadow the embedded `publicPage` string fields of the same name
(`public.go:104-105`). It works (templates resolve the outer fields), but the
shadowing is fragile; consider setting the shared string fields from the
decoded dimensions instead.
(c) A corrupt stored image JSON in one row fails the whole listing with 500
(`public_project.go:166-168`) instead of skipping that row's image; likewise a
read error in `publicImage` answers 500 (`public_shop.go:436-439`) where a
bare 404 would do. Consider degrading to no-image/404.
(d) A published-but-empty category returns 200 ("Nothing here yet") and is
listed in the sitemap; only the shop root 404s on thin content. This matches
the ADR letter (404 until the Pack installs the types); consider whether empty
categories should stay out of the sitemap.
(e) `CoursesURL` is set nowhere in slice 3 — a guarded placeholder for slice 4.
(f) Round-1 notes (a) (RPC "POST-only" wording is imprecise for Connect v2 GET)
and (c) (logo used verbatim as `og:image`, including SVG) still stand; product
SVGs at least omit the dimension tags (decode fails → zeros).

## Skipped (per brief / rules)

Full `make test` (master's merge-time gate runs it after the rebase), a full
browser pass (brief asks only for the render check above), web
typecheck/test/build (no `web/` changes), and live product-image seeding (the
CLI has no store-image upload path; bytes, headers, dimensions and hash-404s
are asserted by `TestPublicProductImage` and `TestPublicProduct`).
