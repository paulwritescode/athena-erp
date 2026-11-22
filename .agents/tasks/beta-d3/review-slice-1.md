# Review: D3 slice 1 — ADR-0030 + decision-log/README rows (docs only)

- Worktree: `/Users/iampaulking/projects/duchess/athena-erp-d3`, branch `feat/d3-public`, commit `e801e78`
- Diff reviewed: `git diff 2bde6c7...HEAD`
- Reviewer workflow: `.kiro/specs/athena-beta/design.md` §5 step 4 (read-only review against the task, requirements, design system); brief `review-brief-slice-1.md`
- No suite run (docs-only slice per brief); no browser pass

Verdict: PASS

## Scope check (brief item 5)

`git diff --stat 2bde6c7...HEAD` shows exactly 3 files, all docs:

- `docs/decisions/adr-0030-public-pages-and-seo.md` (new, 104 lines)
- `docs/decisions/decision-log.md` (+1 row)
- `docs/README.md` (+1 row)

`git diff 2bde6c7...HEAD -- '*.go' '*.vue' '*.proto' 'web/*' 'cmd/*' 'internal/*'` is empty: no product code. Worktree `git status` clean apart from the committed slice.

## Findings

No blockers. No NEEDS_CHANGES items.

### 1. ADR-0030 status and required sections — OK

- `docs/decisions/adr-0030-public-pages-and-seo.md:3` — `Status: Proposed`; `:26` restates "Proposed." The only "Accepted" string in the whole diff is a pre-existing D-014 context line in `decision-log.md`, not an added line. Nothing marked accepted.
- SSR route design (`:20`, `:26`, `:28-42`): Go `html/template` per request, new route group ahead of the SPA catch-all. Present.
- Public-projection rule (`:44-61`): named Go function per page, fixed Record Type + field allowlist, direct store queries, never RecordService/Grants/sessions; `is_published` + liveness + exact slug/token match. Present.
- Per-page SEO contract (`:63-76`): every indexable page carries unique title, unique meta description, canonical, OG, Twitter `summary_large_image`, 1200x630 image, JSON-LD; per-page title + JSON-LD table; canonical via `public.base_url` setting with request-host fallback; sitemap/robots rules. Present.
- Noindex list (`:78-80`): `X-Robots-Tag` header approach; covers `/`, `/build/*`, `/view/*`, `/verify/{token}`, 404, RPC/telemetry; carts/checkout/tracking forward noindex rule. Present.
- Vitals budget (`:82-84`): LCP ≤ 2.5 s, INP ≤ 200 ms, CLS ≤ 0.1 + construction budget (no client JS, inline CSS, system fonts, explicit image dimensions). Present.

### 2. Completeness against tasks.md D3 — OK

- Six page kinds all in the route table (`:32-37`): storefront `/shop`, category `/shop/c/{slug}`, product `/shop/p/{slug}`, course catalogue `/courses`, transcript verification `/verify` + `/verify/{token}`, Workspace home `/home`.
- Per-page SEO elements: title + JSON-LD per page in the table (`:67-74`); description/canonical/OG/Twitter/1200x630 covered by stated page-wide rules (`:65`, `:76`). See non-blocking note (a) below on description templates.
- Per-Workspace `sitemap.xml` + `robots.txt` (`:39-40`, `:76`). OK.
- Noindex for carts, checkout, tracking, sign-in, whole Desk (`:80`): sign-in via SPA root `/`, Desk via `/build/*` + `/view/*`; carts/checkout/tracking get the must-noindex-from-first-version rule since they do not exist. Matches tasks.md D3 and brief slice 3.
- Vitals targets exact (`:84`). OK.
- Desk SPA stays (`:10`, `:21` rejects SPA/SSR, `:42` `/` stays Desk with B11 rationale, `:96` `web/src` untouched). OK.

### 3. No overreach — OK

- Carts/checkout/tracking: no routes designed, only the noindex forward rule (`:80`). OK.
- No new tables/columns: stated in Context (`:12`), canonical via existing Settings chain (`:76`), Consequences (`:96`). No migration touched or requested. OK.
- No section Z decision: Z10 explicitly avoided (system font stack `:15`, `:84`, revisit `:104`); O-008 left open with provisional canonicals (`:16`, `:76`, `:97`, `:101`).
- Design-system §11 consistent: visitor row indexes public pages "except carts, checkout, tracking and verification results" — ADR noindexes `/verify/{token}` results while keeping the verify form findable (`:80`), and forwards noindex to carts/checkout/tracking. Workspace logo use (`:54`, `:76`) matches §10 (Workspace logo on public pages).
- Vocabulary (ADR-0008): Record Type / Record / Pack / Workspace used correctly; no old terms ("project" appears only in "project owner review", generic English).

### 4. decision-log.md + docs/README.md rows — OK

- `decision-log.md` added row: `ADR-0030`, accurate summary (projections, `is_published`, liveness, SEO contract, noindex, vitals), `Proposed 2026-11-22`, links `adr-0030-public-pages-and-seo.md`. Format matches neighboring rows.
- `docs/README.md` added row: links the ADR file, accurate "proposed: ..." summary, placed with the other ADR rows.
- Both links resolve to the new file (verified on disk).

## Non-blocking notes (follow-ups for later slices, not merge blockers)

(a) `adr-0030-public-pages-and-seo.md:65` requires a unique meta description per indexable page, but unlike titles and JSON-LD (table at `:67-74`) there is no per-page description source/template. Slices 2–4 should define where each page's description comes from (e.g. truncated `description` field vs composed string); suggest recording it in slice 5 docs if the ADR is not amended.
(b) There is no ADR-0029 file in `docs/decisions/` (README jumps 0028 → 0030). Numbering was assigned by master per the brief, so this is not a slice-1 defect — noted only so the gap is not read as a missing file in this diff.
(c) Home route is `/home` with `/` staying the Desk (`:42`), a deliberate Proposed choice with B11 rationale, flagged for project-owner review at `:26`. No action; recorded here because it is the one route-path decision most likely to draw owner comment.
