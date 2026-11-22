# Review brief: D3 slice 2, public plumbing + Workspace home (server)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-d3, branch
feat/d3-public (commit 10ce8cb). Brief:
../athena-erp/.agents/tasks/beta-d3/brief.md (slice 2 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md. Contract: ADR-0030 (as amended in this diff).

Review the slice-2 diff (git diff 9cf4005...10ce8cb — 7 files):

1. `internal/server/public.go` (new) + `server.go` (+4 wiring):
   public route group placement (ahead of the SPA catch-all?),
   GET /home, per-Workspace sitemap.xml + robots.txt,
   X-Robots-Tag noindex. Verify the projection rule: /home
   exposes exactly the ADR-allowed fields, nothing else
   reachable; no RecordService/Grants/sessions from public
   paths.
2. Noindex coverage: Desk routes, sign-in, health check, public
   404s — verify each against the ADR list (carts/checkout/
   tracking must NOT exist as pages — forward rule only).
3. Templates (`public/*.html`): SEO tags + Organization JSON-LD
   on /home per the contract; escaping by construction
   (html/template, no unescaped interpolation of stored values).
4. ADR-0030 amendment (+15): meta-description sources defined
   per the slice-1 review note (a) — verify the sources exist
   for every indexable page so far.
5. Tests (`public_test.go`, 9 tests): rerun; judge coverage
   (status codes, tags, headers, per-Workspace sitemap/robots,
   noindex headers).
6. Confirm: no new tables (no migration), no .proto changes, no
   hand-written web/ or cmd/ changes, Z2/Z4 untouched.
7. Render check (not a full browser pass): serve a throwaway
   Workspace from this tree's binary on a free loopback port
   and read the raw bytes of /home, /sitemap.xml, /robots.txt
   and a 404 — tags, JSON-LD and headers as claimed.
8. Run: gofmt -l cmd internal (empty), go build ./..., go vet
   ./..., `affected` by the ABSOLUTE main-repo path
   /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected
   (same lock), make generate twice with no diff. Tree must be
   clean at 10ce8cb before any gate.

Merge-small rule: only blockers stop the merge (projection
overreach, missing noindex, unescaped output, red tests);
the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-d3/review-slice-2.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master
"D3 SLICE 2 REVIEW: PASS or NEEDS_CHANGES <one line>". Then
wait for instructions. Do not edit product code, commit, merge,
push or start agents.
