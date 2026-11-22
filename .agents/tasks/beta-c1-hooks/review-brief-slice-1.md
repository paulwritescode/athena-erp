# Review brief: C1 slice 1 (6.8), lifecycle hooks + Posting rule type

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1, branch
feat/c1-hooks (commit a3691da). Brief:
../athena-erp/.agents/tasks/beta-c1-hooks/brief.md (slice 1 only).
Follow .kiro/specs/athena-beta/design.md section 5 reviewer workflow
and AGENTS.md. Design reference: batch-2 design G6; requirements
R6.4/R6.5/R6.11/R6.14/R6.15/R6.16.

Review the slice-1 diff (git diff f28e953...a3691da — 11 files,
~1800 insertions):

1. Lifecycle hooks: submit posts via posting Rules and cancel
   mirrors in-transition (R6.4/R6.5 single transaction, R6.11).
   NOTE the worker's deviation: `internal/lifecycle/` untouched
   (claims direct transition calls + the registry suffice);
   `internal/server/lifecycle.go` (+136) edited instead. Judge
   whether R6.4/R6.5 are met equivalently or this is a gap.
2. Posting rule type (`internal/rules/rules.go` + posting_test.go):
   plan API, fixtures in source, entry post returning plan dict.
3. Runner + SaveRule (`rules_engine.go`, `server/rules.go`):
   fixture replay, plan diff, two-step named approval (changed
   plan forces disabled save, re-enable by a named user is the
   approval, recorded in versions + audit). No new migration —
   verify the versions/audit recording actually persists.
4. Preview-hash check (R6.15): `records.proto` +`posting_preview_hash`
   (+5, minimal addition approved) + regen; verify generate-clean
   and no new procedure (no gate entry needed — confirm none is).
5. Small additions: posting `HashPlans` export (C1-owned — fine),
   `find()` parent_id filter (verify the need + tests).
6. Z2: worker claims sentinels byte-identical — VERIFY by diffing
   the sentinel paths (no window/policy/day-count/book defaulting
   anywhere in the diff).
7. Confirm no Desk in this slice (only `web/src/gen` regen, no
   hand-written web/ change).

Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
`affected` by the ABSOLUTE main-repo path
/Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected
(it falls back to full make test on the proto change; same lock),
make generate twice with no diff. Tree must be clean at a3691da
before any gate. No browser pass.

Merge-small rule: only blockers stop the merge (unmet R6.x,
broken approval persistence, Z2 invention, red suite); the rest
are follow-ups (worker notes: stale rules.proto posting comment,
Desk Rules page lacks the posting type — confirm non-blocking).
Write findings to
../athena-erp/.agents/tasks/beta-c1-hooks/review-slice-1.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: say master "C1 6.8 REVIEW: PASS
or NEEDS_CHANGES <one line>". Then wait for instructions. Do not
edit product code, commit, merge, push or start agents.
