# Split: C2 slices 4–6 — workflows, delivery, API surface (submaster brief)

Parent task branch `feat/c2-events-jobs` is at origin/main `919bab4` (slices 1–3 merged, branch emptied).
Worktrees live beside the main repo `/Users/iampaulking/projects/duchess/athena-erp` (siblings `athena-erp-*`).
Master pre-approved this split; sm-c2 may run at most 2 subworkers (sw-c2-1, sw-c2-2), one per sub-slice.

Read first (both sub-slices): `.kiro/specs/athena-batch-2/tasks.md` G8.6–G8.7, G8.11–G8.14 (NOTE printed
migration/ADR numbers there are stale — numbers below win); batch-2 "Conventions"; batch-2 design.md G8;
requirements R8; `internal/events/`; `internal/jobs/`; `internal/auth` invite/reset flow; ADR-0028 (Proposed);
tasks.md C2. Z4 RULE (binding): retention/custody UNDECIDED, so 8.8/8.9 (secrets) and 8.10 (lifetimes/disposal)
are OUT — no tables, no CLI, no ADR text. List the deferred items in every report.

No new migration in this split: migration 17 holds events/outbox/inbox + jobs/workflows tables, migration 19
holds the occurrence_key widening. If a subworker believes a migration is needed, STOP and escalate to w-c2
(master reserves numbers in `.agents/stylus/migrations.md`; workers never pick one).

## Sub-slice A "core" — brief slices 4+5 (G8.6+G8.7 workflows, G8.11 delivery)

- Branch `feat/c2-core` cut from `feat/c2-events-jobs`; worktree `../athena-erp-c2-core`.
- Scope: `internal/events/workflow.go` (instances, waits, gated task creation, cycle budget) + gated-task
  property test (Property 14); invite + reset delivery through the outbox where a destination exists, manual
  code path otherwise (`internal/auth/service.go`).
- Files it may touch (ONLY these): `internal/events/workflow.go` (new), `internal/events/workflow_test.go`
  (new), `internal/events/dispatch.go` (only if workflow hooks need it, additive), `internal/auth/service.go`
  (edit), `internal/auth/outbox_delivery_test.go` (new). No other files.
- Acceptance: Property 14 passes; workflows behave per G8.6–G8.7; invite+reset deliver via outbox with manual
  fallback per G8.11; `gofmt -l cmd internal` empty; `go vet ./...`; `affected` green on its branch.

## Sub-slice B "surface" — brief slice 6 (G8.12+G8.13+G8.14)

- Branch `feat/c2-surface` cut from `feat/c2-events-jobs`; worktree `../athena-erp-c2-surface`.
- Scope: `events.proto` (+ settings additions — EXTEND `SettingsPage.vue`/registry, REPLACE the Jobs stub page,
  never duplicate); `make generate`; handlers + gate.go policies (every procedure needs one);
  `cmd/athena/{jobs,events}.go` (no secrets CLI — deferred with 8.8); Jobs page + docs; checkpoint incl.
  browser pass of Jobs and Settings in both themes + crash-between-commit-and-dispatch test.
- Files it may touch (ONLY these): `proto/athena/v1/events.proto` (new, own file); generated
  `gen/athena/v1/events*` and `web/src/gen/athena/v1/events*` (via `make generate` only);
  `internal/server/events.go` + `internal/server/events_test.go` (new); `internal/server/server.go` (mount only);
  `internal/server/gate.go` (append policy entries at the END of `procedurePolicies`);
  `cmd/athena/jobs.go` + `cmd/athena/jobs_test.go` (new); `cmd/athena/events.go` + `cmd/athena/events_test.go`
  (new); `cmd/athena/main.go` (wire commands + help text only); `web/src/pages/JobsPage.vue` (replace stub);
  `web/src/pages/SettingsPage.vue` (extend); `web/src/pages/registry.ts` (Jobs entry props only, if needed);
  `web/src/api.ts` (append client fns at end); web tests for touched pages; docs
  (`docs/architecture/overview.md`, `README.md`, `AGENTS.md` only if commands changed,
  `docs/decisions/decision-log.md` C2 row, ADR-0028 file status notes only).
- MUST NOT touch `web/src/App.vue` (Jobs+Settings are already in `registry.ts` and the `Page` type, so no
  App.vue change should be needed; if one becomes necessary, STOP and escalate to w-c2 — one lane at a time).
- Acceptance: `make generate` twice with no diff; every new procedure has a gate policy
  (`TestEveryProcedureHasAPolicy` passes); jobs/events CLI commands work; Jobs page replaces the stub and
  Settings is extended (design system holds, Sonner toasts, both themes browser-passed);
  crash-between-commit-and-dispatch test passes; docs match; web typecheck/test/build green; `affected` green.

## Submaster runbook

1. Cut both sub-branches from `feat/c2-events-jobs` with their worktrees; start sw-c2-1 (sub-slice A) then
   sw-c2-2 (sub-slice B): `spawn subworker sw-c2-<n> --task c2 --lane L2 --cwd ../athena-erp-c2-<core|surface>
   --cli <default> --place down --brief .agents/tasks/beta-c2/split.md "<sub-slice A|B> only; touch only its
   listed files."`
2. Review each subworker diff against this brief (scope + file list); send back anything out of scope.
3. Merge into `feat/c2-events-jobs` in order A then B (rebase each sub-branch on the task branch first;
   gate.go/main.go/registry/api appends merge cleanly — if a rebase conflicts outside an append tail, escalate).
4. Run `affected` after each merge; at the end run the full slice-6 checks (gofmt, build, vet, generate
   stable, web typecheck/test/build, `affected`); restore `internal/webui/dist/.gitkeep` before committing.
5. Standing rules for all: batch-2 conventions (property tests, spans without values, no Record
   values/credentials in spans, telemetry off unless the endpoint is set); throwaway Workspaces and
   `test_athena_*` databases only, never the dev database; fake HOME for any server; heavy checks only
   through `affected`/`suite` (never bypass the lock); ADRs stay Proposed; report deferred Z4 items.
6. Report `PASS` (or `BLOCKED`/`QUESTION`) to w-c2 with the merged head commit when done. Subworkers report
   only to sm-c2; sm-c2 reports only to w-c2.
