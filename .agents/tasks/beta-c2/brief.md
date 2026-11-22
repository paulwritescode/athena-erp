# Brief: C2 jobs, outbox, events, workflows (G8 minus Z4-blocked parts) (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c2, branch feat/c2-events-jobs (from latest main).
Read: tasks.md C2; .kiro/specs/athena-batch-2/tasks.md G8.1–G8.14 (this task; NOTE the printed migration/ADR numbers there are stale — use the numbers below); batch-2 "Conventions"; .kiro/specs/athena-batch-2/design.md G8; requirements R8 (batch-2 requirements.md); internal/posting (C1, partially landed — read what exists); internal/lifecycle; internal/auth invite/reset flow; proto/athena/v1/ledger.proto; web page registry + existing Jobs stub + existing SettingsPage.

Numbers assigned by the master: migration **17** `events_jobs_workflows`; ADR-**0028** "Events, jobs and workflows" (Proposed). No other migration/ADR in this task.

Z4 RULE (binding, user decision open): retention periods and key custody are UNDECIDED. Therefore 8.8/8.9 (secrets) and 8.10 (lifetimes/disposal) are OUT of this task entirely — do not build them, do not include their tables in migration 17, do not write their ADR. The migration holds events/outbox/inbox + jobs/workflows tables only. List the deferred items in every report.

Slices, each merging on its own (report after each; one worker, sequential):
1. 8.1 (adjusted): migration 17 + ADR-0028 (Proposed) + decision-log/README rows. State the P16-before-P14 reorder; record retention/custody as open (Z4).
2. 8.2+8.3: `internal/events/` (envelope, producer write in the caller's transaction, inbox with effect keys, Pack-declared subscriptions) + emit-on-commit hooks in internal/posting and internal/lifecycle + inbox-deduplication property test (Property 13). NOTE: C1 still extends internal/posting on another branch — keep your posting edits minimal and additive; rebase carefully at merge time.
3. 8.4+8.5: `internal/jobs/` (leases with expiry + fencing, backoff, owner assignment, restart recovery, occurrence keys with timezone + catch-up) + server.go worker startup + occurrence-uniqueness property test (Property 15).
4. 8.6+8.7: workflows (`internal/events/workflow.go`: instances, waits, gated task creation, cycle budget) + gated-task property test (Property 14).
5. 8.11: invite + reset delivery through the outbox where a destination exists, manual code path otherwise (internal/auth/service.go).
6. 8.12+8.13+8.14: `events.proto` (+ settings additions — EXTEND the existing Settings page/registry, do not recreate it; REPLACE the Jobs stub page, do not duplicate it); `make generate`; handlers + gate.go policies (every procedure needs one); `cmd/athena/{jobs,events}.go` (no secrets CLI — deferred with 8.8); Jobs page + docs (overview, README, AGENTS.md, decision log); checkpoint incl. browser pass of Jobs and Settings in both themes + crash-between-commit-and-dispatch test.

Rules: batch-2 conventions (policies in gate.go, generate twice no diff, property tests, spans without values, no Record values/credentials in spans). Telemetry stays off unless the endpoint is set. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server (user server holds the real lock). Commit on your own branch feat/c2-events-jobs; you never merge, push or start agents.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test, make generate stable; web typecheck/test/build for slice 6.
Report: hc send master "C2 SLICE n: PASS or BLOCKED <one line>".
