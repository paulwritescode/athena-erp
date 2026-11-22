# ADR-0030: Public pages and SEO

- **Status:** Proposed
- **Date:** 2026-11-22
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0008 (vocabulary), ADR-0010 (kinds and system fields), ADR-0015 (Workspace files), ADR-0027 (Image fields), D-007 (platform core, no seeded domain types), O-008 (hosted use open), Z10 (Satori embedding open, avoided here)

## Context

Task D3 needs public pages a Pack publishes (storefront, product pages, course catalogue, transcript verification, Workspace home) that search engines can index, while the Desk SPA stays exactly as it is for signed-in work. Constraints from the brief and the repository:

- No new tables without asking (no migration is reserved for D3); no proto changes; no Desk changes (`web/src` untouched); the public must never call the admin RecordService.
- D-007: the core seeds no domain Record Types or Packs, so commerce and catalogue content exists only after a Pack installs it (D5, D7).
- The Desk owns `/` (SPA entry, sign-in when signed out), `/build/*` and `/view/*` (`web/src/routePaths.ts`); public routes must not collide with them.
- Z10 (Satori web embedding rights) is open, so public pages cannot rely on Satori.
- O-008 (hosting, TLS, deployment) is open and the server is loopback-only, so absolute canonical and image URLs have no public origin yet.

## Options considered

1. **Server-rendered Go pages from fixed public projections (chosen).** New `GET` routes in `internal/server/public*.go` render `html/template` templates per request. Each page reads only through a named Go projection function with a fixed Record Type and field allowlist, over direct store queries. Nothing else is reachable: no RecordService, no Grants evaluation, no session use.
2. **Serve public pages from the Vue SPA (rejected).** Keeps one renderer, but crawlers get an empty shell without pre-rendering, and it would move Desk code toward SSR, which tasks.md D3 forbids ("Keep the Desk SPA for signed-in work; do not move it to SSR").
3. **Static export at publish time (rejected).** Simpler to serve, but needs a build/refresh pipeline and a place to store output that does not exist; per-request rendering reuses the running server and is always current.

## Decision

Proposed. Public pages are server-rendered HTML from Go `html/template` templates (new directory, embedded at build), rendered per request by a public route group registered in `server.go` ahead of the SPA catch-all. The sections below are the contract slices 2 to 4 implement. Still needed: project owner review of the route paths, the projection field lists (which become the Pack contract for D5/D7), and the vitals numbers from slice 4.

### Route design

| Route | Page | Index |
| --- | --- | --- |
| `GET /home` | Workspace home | yes |
| `GET /shop` | Storefront index | yes |
| `GET /shop/c/{slug}` | Category | yes |
| `GET /shop/p/{slug}` | Product | yes |
| `GET /courses` | Course catalogue | yes |
| `GET /verify` | Verification form | yes |
| `GET /verify/{token}` | Verification result | no (`noindex`) |
| `GET /sitemap.xml` | Per-Workspace sitemap | n/a |
| `GET /robots.txt` | Crawlers file | n/a |

`/` stays the Desk unconditionally: re-routing the SPA root by session would change the sign-in entry point the B11 first-run flow depends on. Public paths avoid `/build/*`, `/view/*` and `/`; any future top-level Desk path must avoid the public list above. Unknown slugs, tokens, or missing content answer `404` without saying what exists (the `/brand/` pattern). Responses set `Content-Type: text/html; charset=utf-8` and keep the existing `securityHeaders` wrapper. Publication is by content, not by login: public handlers ignore sessions and cookies entirely.

### Public-projection rule

A projection is a Go function per page that returns only allowlisted values. It resolves table and column names from the saved Record Type definition (as `RecordService` does) and queries the store directly with a fixed column list; it never calls `RecordService`, never evaluates Grants, and never reads fields outside the table below. A row is public only when **all** hold:

- its Record Type key is in the page's allowlist and the type carries a boolean field `is_published` set to `true` (types without that field publish nothing);
- the row is live (masters not soft-deleted, documents not cancelled);
- for slug/token routes, the lookup field matches exactly.

| Page | Record Type keys | Exposed fields (nothing else) |
| --- | --- | --- |
| Home | none (Workspace identity only) | display name, logo URLs from `GetIdentityDoc` |
| Storefront | `product_category`, `product` | `name`, `slug`, `description`, `image`, `price` (product); `name`, `slug`, `description` (category) |
| Category | `product_category`, `product` | as storefront, scoped to the category link |
| Product | `product` | `name`, `slug`, `description`, `image`, `price`, category `name` + `slug` |
| Catalogue | `course` | `name`, `slug`, `description`, `image` |
| Verification | `transcript` | match on `verification_token`; show only `holder_name`, `course_name`, `completed_on`, `status` |

These keys are conventions Packs adopt (D5, D7); the core seeds nothing (D-007). Until a Pack installs them the commerce and catalogue routes answer `404`, which keeps thin or empty pages out of the index. `price` renders as plain text from the stored Money value; `image` renders through a public image route that serves only images referenced by published rows, with content-hash URLs in the `/brand/` style. Product, category and course detail beyond the listed fields (stock, cost, internal notes) is never reachable from these routes.

### Per-page SEO contract

Every indexable page carries: a unique `<title>`, a unique meta description, a canonical URL, Open Graph tags, Twitter card tags (`summary_large_image`), a 1200x630 image, and JSON-LD. Per page:

| Page | Title | JSON-LD |
| --- | --- | --- |
| Home | `{Workspace name}` | `Organization` (name, logo, url) |
| Storefront | `Shop {Workspace name}` | `Organization` + `BreadcrumbList` |
| Category | `{Category name} — {Workspace name}` | `BreadcrumbList` (+ `Organization`) |
| Product | `{Product name} — {Workspace name}` | `Product` + `BreadcrumbList` (+ `Organization`) |
| Catalogue | `Courses — {Workspace name}` | `ItemList` of `Course` (+ `Organization`) |
| Verify form | `Verify a transcript — {Workspace name}` | `Organization` only |

`og:image` uses the page's projection image when present, else the Workspace logo, else one neutral 1200x630 fallback PNG embedded in the binary (slice 4; until then pages without an image omit `og:image` rather than claim dimensions they lack). The catalogue page embeds one `Course` node per listed course. Canonical URLs use a `public.base_url` Workspace setting when set (Settings chain, no new table) and otherwise fall back to the request's own scheme and host; while the server is loopback-only (O-008) canonicals are placeholders, and the deployment decision must set the public origin. `sitemap.xml` lists the home, storefront, catalogue and every published category, product and course URL with `lastmod` from the row's modified timestamp; `robots.txt` points at the sitemap and disallows `/build/`, `/view/`, `/verify/`, `/otlp/` and the RPC paths.

Meta description sources (slice-1 review note (a)): the Workspace holds no tagline field, so index pages use composed strings and detail pages use the projection's `description` field, cut at 160 characters on a word boundary with the fallback below when the field is empty.

| Page | Meta description source |
| --- | --- |
| Home | composed `Welcome to {Workspace name}.` |
| Storefront | composed `Browse everything in the {Workspace name} shop.` |
| Category | truncated category `description`; fallback `{Category name} at {Workspace name}.` |
| Product | truncated product `description`; fallback `{Product name} at {Workspace name}.` |
| Catalogue | composed `Browse courses at {Workspace name}.` |
| Verify form | composed `Verify a {Workspace name} transcript.` |

### Noindex list

These send `X-Robots-Tag: noindex, nofollow` (header, so no Desk file changes): the SPA root `/` (sign-in when signed out), every Desk path under `/build/*` and `/view/*` (hashed `/assets/*` files excepted), the health check, verification results (`/verify/{token}`), and the public 404 page. RPC and telemetry paths are POST-only and stay out via `robots.txt` disallow rather than response headers. `/brand/*` content images stay indexable: they back `og:image`. Carts, checkout and order tracking do not exist; when they land they must send `noindex` from their first version (they are personal or transactional pages: §11). The verify form itself stays indexable so people can find it.

### Vitals budget

Targets from tasks.md D3: LCP ≤ 2.5 s, INP ≤ 200 ms, CLS ≤ 0.1, measured with headless Chrome on every new page (slice 4 records the numbers or reports that the harness cannot measure). The construction budget that meets them: one HTML response per page, no client JavaScript, no render-blocking external requests, a system font stack (Satori is unavailable while Z10 is open), inline CSS only, and explicit `width`/`height` on every image. Public pages follow design-system §1 restraint and §8 everyday words; they are not Desk screens, so the Desk component checks do not apply.

### Safety rules

- Escape everything by construction: `html/template` for all markup; no `template.HTML` with Record content; JSON-LD serialized with HTML escaping.
- No Record values, credentials, tokens or session material in logs (log route, status and request ID only), traces or error pages. Telemetry stays off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set (existing behavior).
- Unknown slugs/tokens are bare `404`s; error pages carry no content hints.

## Consequences

- The Go server gains its first crawler-facing surface; every later public route inherits the projection rule and the SEO contract above.
- D5/D7 Packs that want public pages must provide the Record Type keys and fields in the projection table; anything else stays private by default.
- `server.go` grows route registrations and a small noindex wrapper around the SPA handler; `gate.go`, protos, `api.ts`, `App.vue` and `web/src` are untouched.
- Canonical URLs stay provisional until O-008 (hosting) is decided.

## Revisit when

- O-008 picks a deployment shape: set the public origin and re-check canonicals, robots and the sitemap against it.
- D5/D7 need fields the projection table lacks: extend the allowlist here first, then in code.
- Vitals miss their targets on real content: budget the failing page (fewer items per page, smaller images) before adding machinery.
- Z10 clears Satori for the web: revisit the system font stack.
