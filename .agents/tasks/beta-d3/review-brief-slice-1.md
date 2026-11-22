# Review brief: D3 slice 1, ADR-0030 + decision-log/README rows (docs only)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-d3, branch
feat/d3-public (commit e801e78). Brief:
../athena-erp/.agents/tasks/beta-d3/brief.md (slice 1 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md.

Review the slice-1 diff (git diff 2bde6c7...HEAD — must be 3 docs
files only: docs/decisions/adr-0030-public-pages-and-seo.md,
docs/decisions/decision-log.md, docs/README.md):

1. ADR-0030 is marked **Proposed** (never accepted): SSR route
   design (Go html/template per request), the public-projection
   rule (exactly which fields each page may expose), the per-page
   SEO contract, the noindex list, the vitals budget.
2. Completeness against tasks.md D3: all six page kinds covered
   (storefront, category, product, course catalogue, transcript
   verification, Workspace home); per-page title/description/
   canonical/OG/Twitter/1200x630/JSON-LD; per-Workspace
   sitemap.xml + robots.txt; noindex for carts, checkout,
   tracking, sign-in and the whole Desk; vitals targets (LCP ≤
   2.5 s, INP ≤ 200 ms, CLS ≤ 0.1); the Desk SPA stays for
   signed-in work (no SSR migration).
3. No overreach: carts/checkout/tracking must NOT be designed as
   pages (they do not exist — noindex rule only); no new tables
   or columns snuck in (no migration is reserved); no section Z
   decision taken; nothing marked accepted; consistent with
   design-system §11 (pages by audience).
4. decision-log.md + docs/README.md rows accurate and linked.
5. Confirm the diff holds no product code (no .go, .vue, .proto,
   no web/ or cmd/ changes).

No suite (docs-only slice; do not run make test). No browser pass.
Write findings to
../athena-erp/.agents/tasks/beta-d3/review-slice-1.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: say master "D3 SLICE 1 REVIEW: PASS or
NEEDS_CHANGES <one line>". Then wait for instructions. Do not edit
product code, commit, merge, push or start agents.
