# Review brief: C4 slice 1, migration 18 + ADR-0029 (13.1)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c4, branch
feat/c4-trees (commit d9cee00). Brief:
../athena-erp/.agents/tasks/beta-c4/brief.md (slice 1 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md. Design reference: .kiro/specs/athena-batch-2/design.md
G13; requirements R13.1/R13.4; migration number 18 reserved in
.agents/stylus/migrations.md.

Review the slice-1 diff (git diff 9cf4005...HEAD — 5 files:
sysmigrations.go, sysmigrations_test.go, ADR-0029, decision-log.md,
docs/README.md):

1. Migration 18 `trees_effective_uom`: UoM families/conversions
   tables, tree path/depth columns on the four kernel tree
   masters with parent-chain backfill, effective range on
   determinations. Check: columns/tables match ADR-0029 +
   R13.1/R13.4; backfill correct and bounded; down/rollback
   story per convention; nothing seeded; no other migration
   touched (applied migrations are never edited).
2. ADR-0029 marked **Proposed** (never accepted); decision-log +
   README rows accurate (kept alongside the D3 ADR-0030 rows
   from the rebase — verify both rows present and correct).
3. Confirm the diff holds nothing else (no product logic, no
   proto, no web).
4. Run: gofmt -l cmd internal (empty), go build ./..., go vet
   ./..., the sysmigrations tests plus the FULL make test
   through suite — call it by the ABSOLUTE main-repo path
   /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/suite
   (your worktree's copy is stale: no node fallback, no dedup).
   If node is missing on PATH, export the nvm bin dir first.
   Note: the worker's green run predates commit recording, so it
   does not count for reuse — run the suite once yourself.
   make generate twice with no diff. No browser pass.

Merge-small rule: only blockers stop the merge (wrong
tables/columns, broken backfill, edited applied migration,
seeded data, red suite); the rest are follow-ups. Write
findings to ../athena-erp/.agents/tasks/beta-c4/review-slice-1.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master
"C4 SLICE 1 REVIEW: PASS or NEEDS_CHANGES <one line>". Then wait
for instructions. Do not edit product code, commit, merge, push
or start agents.
