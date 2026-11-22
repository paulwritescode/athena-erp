# Review: D3 slice 2 — public plumbing + Workspace home (server)

- Worktree: `/Users/iampaulking/projects/duchess/athena-erp-d3-r` (detached at `10ce8cb`; see process note d)
- Branch/commit reviewed: `feat/d3-public`, `10ce8cb` ("feat(d3): slice 2 public route group + Workspace home + sitemap/robots + Desk noindex")
- Diff reviewed: `git diff 9cf4005...10ce8cb` (7 files, all read in full)
- Brief: `.agents/tasks/beta-d3/review-brief-slice-2.md` (slice 2 only); contract ADR-0030 as amended in this diff
- Reviewer workflow: `.kiro/specs/athena-beta/design.md` §5 step 4 (read-only review against the task); AGENTS.md

Verdict: PASS

No blockers. No NEEDS_CHANGES items.

## Scope check (brief item 6)

`git diff --stat 9cf4005...10ce8cb` shows exactly the 7 briefed files; `--name-only` confirms no others:

- `docs/decisions/adr-0030-public-pages-and-seo.md` (+15 net: meta-description table, noindex-list rewrite, og:image slice-4 note)
- `internal/server/public.go` (new, 291 lines)
- `internal/server/public/layout.html`, `home.html`, `notfound.html` (new)
- `internal/server/public_test.go` (new, 288 lines, 9 tests)
- `internal/server/server.go` (+4/−1: healthz header, `mountPublic`, `deskNoIndex` wrap)

No new tables (no migration file, `sysmigrations.go` untouched; both queries read the existing `athena_setting_values`), no `.proto` changes, no `gate.go`/`api.ts`/`App.vue`/`web/**`/`cmd/**` changes. `tasks.md` untouched, so Z2 (backdating/book policy) and Z4 (retention/key custody) are untouched. ADR stays Proposed.

## Findings by brief item

### 1. Route group + /home projection — OK

- `server.go:153-154`: `mountPublic` is registered ahead of the `/` SPA catch-all; `public.go:61-62` notes explicit patterns win in the Go 1.22 mux anyway. Routes: `GET /home`, `GET /sitemap.xml`, `GET /robots.txt` (`public.go:65-67`). No collision with `/`, `/build/*`, `/view/*`, `/assets/*`, `/brand/*`, RPC paths.
- `home` (`public.go:212-259`) reads only `ledger.Service.GetIdentityDoc` (returns display name + light/dark logo refs, zero doc when unset — `internal/ledger/identity.go:32-46`) and the fixed `public.base_url` setting query (`public.go:141-156`, existing Settings chain, http(s) origin only, falls back to request host). Uses exactly the ADR-allowed fields: display name, logo URLs, mark favicon fallback.
- Grep over `public.go` + `public/*.html`: no `RecordService`, `Grant`, session, cookie, or `template.HTML` — only comments stating sessions/cookies are ignored. `Content-Type: text/html; charset=utf-8` on pages; the whole mux (public included) keeps the `securityHeaders` wrapper (`server.go:158`, observed live as CSP/HSTS-family headers).

### 2. Noindex coverage — OK

- Desk: `deskNoIndex` (`public.go:73-80`) wraps the SPA handler; everything except hashed `/assets/*` gets `X-Robots-Tag: noindex, nofollow`. Covers `/` (sign-in when signed out — no separate sign-in route exists), `/build/*`, `/view/*`.
- Health: `/healthz` sets the header (`server.go:147`); it is the only health endpoint.
- Public 404 (`public.go:196`) and 500 (`public.go:179`) set the header.
- `robots.txt` (`public.go:288-291`) disallows `/build/`, `/view/`, `/verify/`, `/otlp/`, `/athena.v1.` and advertises the sitemap — exactly the ADR list. `/brand/*` stays indexable (backs `og:image`).
- Carts/checkout/tracking: no such routes, handlers, or templates exist anywhere in the diff (grep clean); the ADR forward rule ("must send noindex from their first version") covers their landing. `/verify/*` routes do not exist yet (slice 4); the form/result rules are documented, not code.

### 3. Templates: SEO + Organization JSON-LD + escaping — OK

- `layout.html:6-24`: unique `<title>`, meta description, canonical (omitted when empty, so the 404 carries none), OG tags, `og:url`, conditional `og:image` (no `width`/`height` claims until slice 4 sets them), Twitter `summary_large_image`, favicons, one `application/ld+json` block. Inline CSS only, system font stack (Z10 avoided), explicit image height.
- `/home` JSON-LD is `Organization` (name, url, logo when a logo exists); 404 JSON-LD is a bare `WebPage` ("Nothing here", no path echo, no canonical).
- Escaping by construction: all stored values interpolate through `html/template`; the only trusted value is `template.JS` from `publicJSONLD` (`public.go:107-113`), which is `encoding/json` output (escapes `<`, `>`, `&`) — never raw content. Proven by `TestPublicHomeEscapesName` (name with `<b>&"quoted"` is HTML-escaped in markup and round-trips through the parsed JSON-LD).

### 4. ADR amendment (slice-1 note a) — OK

- `adr-0030-public-pages-and-seo.md:78-87` adds the per-page meta-description source table (composed strings for index pages, truncated `description` + fallback for detail pages). The only indexable page so far, `/home`, uses the composed `Welcome to {name}.` source, which exists (`public.go:233`, observed live). Detail-page sources bind to Pack fields in slices 3-4. The noindex-list rewrite (`:89-91`: `/assets/*` exception, health check, POST-only RPC/telemetry via robots, indexable `/brand/*`) matches the code, except wording note (a) below.

### 5. Tests — OK, rerun green

9 tests in `public_test.go`, all passing via the locked `affected` run (see gates): home tags/headers/JSON-LD (`TestPublicHome`), name fallback without identity, `Athena` fallback without DB, escaping round-trip, `public.base_url` honored + invalid-value fallback, sitemap XML parse (exactly `/home`), robots lines, `deskNoIndex` paths incl. `/assets/` exception, 404 status/headers/no-hint/no-canonical. Coverage judges adequate for the slice: status codes, tags, headers, per-Workspace sitemap/robots origin handling, and noindex headers are all asserted; MariaDB-backed tests ran (not skipped).

### 6. Confirmations — OK

See scope check: no migration, no proto, no hand-written `web/` or `cmd/` changes, Z2/Z4 untouched.

### 7. Render check (throwaway Workspace, clean-tree binary) — OK

Binary built from the review worktree; `go version -m` stamps `vcs.revision=10ce8cb`, `vcs.modified=false`. Throwaway home + Workspace `test_athena_d3_s2_review` (slug-derived `test_athena_*` DB, dropped afterwards) with display name "Slice Two Review Shop", served on `127.0.0.1:13210` with a fake HOME (user's dev server untouched). Raw bytes observed:

- `/home` → 200, `text/html`, no `X-Robots-Tag`: `<title>Slice Two Review Shop</title>`, composed description, canonical `http://127.0.0.1:13210/home`, OG + Twitter tags, no `og:image` (no logo set, per ADR), mark favicons, `Organization` JSON-LD with name+url and no logo key.
- `/sitemap.xml` → 200, `application/xml`: single `<loc>` for `/home`.
- `/robots.txt` → 200, `text/plain`: the 5 ADR disallows + `Sitemap:` line with the request origin.
- `/shop/p/nope` → 404 `text/plain` + `X-Robots-Tag: noindex, nofollow` (falls through to the SPA catch-all; the `notfound` template itself is unit-tested and gets its routes in slices 3-4 — note b).
- Noindex matrix: `/`, `/build/overview`, `/view/x` → 404 + noindex (Desk dist unbuilt in the review tree, so the SPA handler answers Go-default 404 rather than the shell; the header comes from `deskNoIndex` either way); `/healthz` → 200 `{"status":"ok","database":"Connected"}` + noindex; `/home` → 200 with no robots tag.

### 8. Gates (all in the review worktree at 10ce8cb) — OK

| Gate | Observed |
| --- | --- |
| `gofmt -l cmd internal` | empty |
| `go build ./...` | ok |
| `go vet ./...` | ok |
| `affected` (absolute main-repo path, locked) | `ok athena-erp/cmd/athena 150.552s`, `ok athena-erp/internal/server 225.809s` |
| `make generate` twice | both ok, `git status` clean afterwards |
| Tree before gates | clean at `10ce8cb` |

(`make generate` needs node on PATH; agent shells must prepend `~/.nvm/versions/node/v26.7.0/bin` — same fallback `suite` uses.)

## Non-blocking follow-ups (merge-small rule: none stop the merge)

(a) `adr-0030-public-pages-and-seo.md:91` says "RPC and telemetry paths are POST-only". Connect v2 (used here, `go.mod:5`) accepts unary GET (`connecthttp/protocol_connect.go:76,141` in `connect/v2@v2.0.2` module source). No indexing risk — RPC URLs are unlinked, absent from the sitemap, and robots-disallowed — but the sentence is imprecise; suggest rewording to "unlinked; kept out via robots.txt disallow" in a later slice.
(b) The `notfound` template (`public.go:185-207`) has no route yet; unknown public paths 404 through the SPA catch-all (still noindexed). Expected — slices 3-4 wire slug/token lookups to it. Covered meanwhile by `TestPublicNotFound`.
(c) `home` uses the Workspace logo verbatim as `og:image`, including SVG (`public.go:239-241`); several crawlers only render raster `og:image`s. Slice 4 owns the 1200x630 story; consider a raster note then.
(d) Process: the task worktree `../athena-erp-d3` became dirty mid-review (the worker started slice 3 there: new `public_shop.go`, shop/category/product templates, edits to `public.go`/`layout.html`/`home.html`). Per master's order and the current AGENT_RULES Heavy-commands section I moved all gates to detached worktree `../athena-erp-d3-r` at `10ce8cb`, discarded the first render attempt (its binary reported `vcs.modified=true`), and rested the verdict on the locked `affected` run there. An early unfiltered `go test` outside the lock was also stopped per master; it is not used as evidence.

## Skipped (per brief / rules)

Full `make test` (master's merge-time gate runs it after the rebase), a full browser pass (brief asks only for the throwaway render check above), and web typecheck/test/build (no `web/` changes; `affected --print` planned Go only: `./cmd/athena ./internal/server`, web tests: no).
