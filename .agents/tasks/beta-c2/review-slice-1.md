# Review: C2 slice 1 — migration 17 + ADR-0028 (8.1 adjusted)

Verdict: PASS

Scope: worktree athena-erp-c2, branch feat/c2-events-jobs, commit fd7e372
(clean tree; slice-2 WIP confirmed stashed and untouched). Diff `main...HEAD`
is 5 files: `internal/server/sysmigrations.go`,
`internal/server/sysmigrations_test.go`,
`docs/decisions/adr-0028-events-jobs-and-workflows.md`,
`docs/decisions/decision-log.md`, `docs/README.md`.

## Blockers

None.

## What was verified

1. Migration 17 `events_jobs_workflows`
   (`internal/server/sysmigrations.go:526`): creates `athena_events`,
   `athena_inbox`, `athena_event_subscriptions`, `athena_jobs`,
   `athena_schedule_occurrences`, `athena_workflow_instances`,
   `athena_tasks`, and extends `athena_outbox` with dispatch columns only.
   Every envelope/lease/occurrence/workflow/task column required by R8.2,
   R8.3, R8.4, R8.6, R8.7, R8.8, R8.9 is present, with the unique keys the
   requirements name (event sequence, inbox consumer+event+effect,
   occurrence workspace+schedule+scope+period, task instance+key).
2. Outbox compatibility (ADR-0023 "must keep id, event type and payload"):
   migration 17 only `ADD COLUMN IF NOT EXISTS` / `ADD INDEX IF NOT EXISTS`;
   `TestEventsJobsWorkflowsMigrationExtendsTheOutbox` inserts a migration-14
   row and asserts id/type/payload survive with status `pending`.
3. Deferred on open Z4: no `athena_secret_refs`, `athena_key_versions`,
   `athena_disposal_register`, `athena_holds` tables anywhere in the
   migration (grep over the diff shows only deferral prose and the absence
   test). No secrets ADR, no lifetimes code, no other product changes.
4. Conventions: entry 17 appended at the end, versions 1-17 contiguous,
   no edit to any applied migration; statements are idempotent
   (`IF NOT EXISTS` throughout, matching the file's stated convention).
5. ADR-0028 (Proposed): states the P16-before-P14 reorder ("the roadmap
   order P14 then P16 is superseded for this spec only"), records retention
   periods + key custody as OPEN (Z4), defers 8.8/8.9/8.10 explicitly, and
   invents no retention/custody policy. Follows `adr-template.md` section
   for section, consistent with ADR-0023's style. Decision-log and README
   rows added in order.
6. Checks run (node v26.7.0 via nvm, pnpm present):
   - `gofmt -l cmd internal` → empty
   - `go build ./...` → ok
   - `go vet ./...` → ok
   - `make test` → exit 0 (Go suite + 49 web files / 537 tests passed)
   - `TestEventsJobsWorkflowsMigrationExtendsTheOutbox` → PASS against
     MariaDB in 6.22s (not skipped); `TestSystemMigrationsAreValid`,
     `TestEveryProcedureHasAPolicy`,
     `TestOldVocabularyIsGoneOutsideLegacyFiles` → PASS
   - `make generate` twice → no diff, tree still clean
   - No browser pass (no Desk in this slice, per brief)
7. Database safety: all migration testing ran in throwaway `test_athena_*`
   databases. Dev database `athena_erp` confirmed untouched afterwards
   (max applied version 16, no `athena_events` table).

## Notes (not blockers)

- N1. The brief asks for "up AND down". The platform has no down
  mechanism: `store.Migration` (`internal/store/migrate.go:28`) carries
  only `Up`, and `athena migrate` (`cmd/athena/migrate.go:30`) is
  `status|up` only; all 17 migrations are Up-only and append-only. Up is
  proven both fresh (full 0→17 across the suite) and incremental (14→17
  in the new test). Suggest future briefs say "fresh full up +
  incremental up".
- N2. The new tables carry indexes but no foreign keys (e.g.
  `athena_tasks.instance_id`, `athena_inbox.event_id`). Neither the 8.1
  task text nor design G8/R8 requires FKs, and platform convention is
  mixed, so this is not a blocker under the merge-small rule. If the
  master wants FKs, they need a new migration in a later slice, never
  an edit to 17.

## Follow-ups for later slices (non-blocking)

- F1. Consider adding `athena_key_versions` (named in design.md's G8
  table list alongside the other deferred tables) to the absence-test
  list in `sysmigrations_test.go:255`, which currently checks
  `athena_secret_refs`, `athena_disposal_register`, `athena_holds`.
- F2. `athena_event_subscriptions` PK is (consumer, event_type): one
  action per consumer+type. Confirm that cardinality is intended when
  8.2 (Pack-declared subscriptions) lands.
- F3. The decision-log ADR-0027 row's "(no migration 17)" parenthetical
  now sits next to a real migration 17; it is historically accurate
  (ADR-0027 consumed no migration) — leave it, just don't misread it.
