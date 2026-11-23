# Review brief: D3 slice 3 — commerce pages (storefront, category, product)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-d3-r (detached; move it to
the final commit in my ORDER). Brief:
../athena-erp/.agents/tasks/beta-d3/brief.md (slice 3 only). Prior verdicts:
review-slice-2.md (PASS) and review-slice-2-reconfirm.md (PASS at 7f5d934).
Contract: ADR-0030. Follow .kiro/specs/athena-beta/design.md section 5
reviewer workflow and AGENTS.md.

ON MY ORDER (final slice-3 commit): move your tree to it
(git checkout --detach <commit>), verify clean, then review the slice-3
diff (git diff main...<commit> — 10 files: public_shop.go + public_project.go
new, shop/category/product templates, layout/home tweaks, public.go edits,
public_shop_test.go, ADR-0030 touch):

1. Routes: storefront, category, product resolve by slug; unknown slugs use
   the slice-2 `notfound` template (no path echo, noindex, no canonical).
   Route placement keeps the SPA catch-all and RPC paths intact.
2. SEO per page: unique title + description, canonical, OG + Twitter tags;
   `og:image` claims only what slice 4 will set (1200x630) — no false
   width/height claims yet. JSON-LD Organization/Product/BreadcrumbList
   valid and page-appropriate.
3. Projection rule: commerce pages expose exactly the ADR-allowed Pack
   fields (name, description, price?, images?) — verify the allow-list in
   code; no RecordService/Grants/sessions/cookies from public paths; no
   Record values beyond the projection in HTML or logs.
4. Escaping by construction (html/template throughout; JSON-LD via encoded
   JSON only). Verify with the tests + one adversarial read (name with
   markup/quote characters).
5. Noindex: robots.txt/sitemap cover the new routes per the ADR; carts/
   checkout/tracking must NOT exist (forward rule only — grep).
6. ADR-0030 touch (+1/-1): verify it says what the code does.
7. Tests (public_shop_test.go): rerun; judge coverage (status codes, tags,
   JSON-LD, escaping, 404s, per-Workspace origin handling).
8. Confirm: no migration, no .proto, no gate.go, no web//cmd/ changes,
   Z2/Z4 untouched. Slice-2 files change only where the diff justifies
   (shared layout/plumbing).
9. Render check (not a full browser pass): serve a throwaway Workspace from
   this tree's binary on a free loopback port with fixture shop data; read
   raw bytes of storefront + one category + one product + one bad slug.
10. Gates: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
    .agents/bin/affected by the ABSOLUTE main-repo path
    /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected (same
    lock; reuse of the worker's green run on the same commit is fine — read
    that log), make generate twice with no diff. Tree clean before any gate.

Merge-small rule: only blockers stop the merge (projection overreach,
missing noindex, unescaped output, false SEO claims, red tests); the rest
are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-d3/review-slice-3.md with a verdict line
`Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with file:line.
Then report: say master "D3 SLICE 3 REVIEW: PASS or NEEDS_CHANGES <one
line>". Then wait for instructions. Do not edit product code, commit, merge,
push or start agents. Send UPDATE every 10 minutes.
