# Review brief: C2 slice 1, migration 17 + ADR-0028 (8.1 adjusted)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c2, branch feat/c2-events-
jobs (commit fd7e372). Brief: ../athena-erp/.agents/tasks/beta-c2/brief.md
(slice 1 only: batch-2 G8.1 as adjusted for open Z4). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design reference: .kiro/specs/athena-batch-2/design.md G8; ADR-0023.

Review the slice-1 diff (git diff main...HEAD — the worker's uncommitted
slice-2 WIP is stashed, not part of this review):

1. Migration 17 `events_jobs_workflows`: events/outbox/inbox + Pack
   subscriptions + jobs/occurrences + workflows/tasks tables, indexes,
   foreign keys. NO secrets tables, NO lifetimes/disposal tables (deferred
   on open Z4 — verify their absence). Migration runs up AND down cleanly
   on a throwaway database; platform-table conventions followed (new
   numbered entry, never an edit to an applied one).
2. ADR-0028 "Events, jobs and workflows" (Proposed): states the
   P16-before-P14 reorder, records retention periods + key custody as
   OPEN (Z4), no invented policy. Decision-log + README rows added.
3. No secrets ADR, no lifetimes code, no other product changes.
4. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test with MariaDB enabled (throwaway test_athena_* databases
   only, never the dev database in .env.local), make generate twice with
   no diff. No browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (bad schema, broken
up/down, invented Z4 policy); the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c2/review-slice-1.md with a verdict line
`Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with file:line.
Then report: hc send master "C2 SLICE 1 REVIEW: PASS or NEEDS_CHANGES <one
line>". Do not edit product code, commit, merge, push or start agents.
Permission policy: stay inside athena-erp and athena-erp-* folders, nothing
destructive.
