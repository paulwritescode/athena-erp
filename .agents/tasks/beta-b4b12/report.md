# Report: B4 and B12 server (Muse Spark, from Codex handover)

**Status: Final.**

Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b4b12-server`, branch `feat/b4b12-server`.
Base: `ee66539` (all task work uncommitted, as instructed; nothing committed, merged or pushed, no agents started).

## Verdict: BLOCKED (environmental, work itself complete)

All B4/B12 work is implemented and every task test plus every other package suite is green, but the
required `make test` check is red in this environment and I cannot turn it green from this task:
the pre-existing `internal/auth` suite overruns the 20-minute Go timeout on this machine, serially
as well as in parallel (see "Full-suite status"). Proven pre-existing and load-caused, not a
regression — but the check as written does not pass here, so BLOCKED is the honest signal. The
master's own rerun on a quieter machine is the natural next step; nothing in this diff needs to
change for it.

## What the handover held, and what I did with it

Codex's partial work was complete in structure and correct in substance. I **kept all of it**:
migration 15, both protos plus generated Go/TS, `internal/notifications`, `internal/appearance`,
`internal/auth/notifications.go`, `internal/ledger/appearance.go`, the server handlers and their tests,
both CLIs and their test, the gate policies, all event writers (auth, rules engine, periods),
the G5-Settings reuse for appearance (so no ADR-0025, per the brief), ADR-0024, the decision-log row,
and the README/AGENTS.md updates.

I **changed only two docs rows** (no Go/proto/CLI behavior touched):

- `docs/README.md`: ADR-0024 row now matches the sibling format
  (`[ADR-0024: …] — proposed: …`) and records that no ADR-0025 is needed.
- `docs/architecture/overview.md`: System-migrations row said "thirteen migrations" (stale since
  migration 14 landed); now "fifteen migrations", mentioning 14 (posting engine) and 15
  (notifications and the Challenge issuer).

## Files changed (full list)

Tracked, modified: `AGENTS.md`, `README.md`, `cmd/athena/ledger.go` (gofmt only),
`cmd/athena/main.go`, `docs/README.md`, `docs/architecture/overview.md`,
`docs/decisions/decision-log.md`, `internal/auth/accesslog.go`, `internal/auth/assignment.go`,
`internal/auth/challenge.go`, `internal/auth/principal.go`, `internal/auth/service.go`,
`internal/ledger/determination_test.go` (gofmt only), `internal/ledger/ledger.go`,
`internal/ledger/periods.go`, `internal/ledger/settings.go`, `internal/server/access_test.go`,
`internal/server/gate.go`, `internal/server/kernel.go`, `internal/server/rules_engine.go`,
`internal/server/server.go`, `internal/server/sysmigrations.go`.

New: `cmd/athena/appearance.go`, `cmd/athena/appearance_test.go`, `cmd/athena/notifications.go`,
`docs/decisions/adr-0024-notifications.md`, `gen/athena/v1/notifications.pb.go`,
`gen/athena/v1/workspace.pb.go`, `gen/athena/v1/athenav1connect/notifications.connect.go`,
`gen/athena/v1/athenav1connect/workspace.connect.go`, `internal/appearance/colors.go`,
`internal/appearance/colors_test.go`, `internal/auth/notifications.go`,
`internal/ledger/appearance.go`, `internal/notifications/inbox.go`,
`internal/server/notifications.go`, `internal/server/notifications_test.go`,
`internal/server/workspace.go`, `internal/server/workspace_test.go`,
`proto/athena/v1/notifications.proto`, `proto/athena/v1/workspace.proto`,
`web/src/gen/athena/v1/notifications_pb.ts`, `web/src/gen/athena/v1/workspace_pb.ts`.

No `web/src` file outside `web/src/gen/` was touched.

## Brief compliance notes

- B12 lifecycle: skipped with cause. A case-insensitive search for "approv" over `internal/` and
  `proto/` finds no approval-assignment workflow (only the not-yet-existing hard-reopen approval,
  role display strings, and separate-duties test names), so there is no "submitted document needs
  your approval" event to notify. ADR-0024 records this.
- B4 storage: reuses the G5 Settings chain (`athena/brand.accent` scalar at Workspace scope), so
  per the brief there is no ADR-0025; ADR-0024 carries the sibling note instead.
- Numbering change from the master (2026-11-21 ~14:40): B7 takes migration 15, so this
  notifications migration becomes 16 at merge; ADR numbers stay as briefed (0024, no 0025). As
  instructed I kept `Version: 15` locally with a `TODO(master, merge-time)` comment in
  `internal/server/sysmigrations.go` (ValidateMigrations needs contiguous numbers until B7 merges),
  left every doc saying 15, and did not renumber anything — the master renumbers at merge.
  Comment-only change; `gofmt -l` still empty and `go build ./internal/server/` still exit 0.
- Only throwaway `test_athena_*` databases were used. The dev database in `.env.local` was never
  written except through tests.

## Command output

- `gofmt -l cmd internal` → empty.
- `go build ./...` → exit 0.
- `go vet ./...` → exit 0.
- `make generate` (full target incl. `web-deps`; node from `~/.nvm/versions/node/v26.7.0`,
  since it is not on the default PATH) → exit 0; `buf generate` run 4× total, checksums of all
  six new generated files identical across runs (stable, no diff).
- Targeted suites, all green:
  - `go test -count=1 -run 'TestInboxAuthEvents|TestNotificationPagingAndOwnership|TestRuleStopsSaveAndNotifiesActor|TestPeriodCloseNotifiesCurrentLedgerWriters|TestWorkspaceAppearancePublicReadAndProtectedWrite|TestAppearanceContrast|TestAppearanceInput' ./internal/server/ ./internal/appearance/` → ok (server 17.0s).
  - `go test -count=1 -run TestAppearanceAndInboxOperatorCommands ./cmd/athena/` → ok (4.2s).

## Full-suite status

- Parallel `make test` (as written): **FAIL only on `internal/auth` hitting the 20m Go timeout**
  (`FAIL athena-erp/internal/auth 1200.683s`). Every other package passed
  (`internal/server` ok 1093.9s, `internal/ledger` ok 157.0s, `internal/files` ok 117.8s, rest ok/cached).
- Diagnosis, all pointing at pre-existing slowness rather than a hang or regression:
  - Verbose solo `internal/auth` run: every executed test **passed**; the timeout fired while
    starting trivial `TestRootUsernameIsReserved`. The time hog is the pre-existing
    `TestLastOwnerProperty` (rapid, 100 iterations with fresh migrated DBs): 635.9s of the run.
  - Base-vs-worktree comparison of that single test (clean worktree at `ee66539` in
    `/tmp/athena-base`, since removed): base **535.2s PASS**, worktree **574.3s PASS**.
    Same order; the delta is run-to-run variance (DB create/migrate/drop, argon2, load). The
    task diff adds at most milliseconds per iteration (one extra in-tx INSERT on some paths,
    two extra migration statements per fresh DB).
  - No new unbounded work: auth retry helper stays capped at 3 attempts; no test uses
    `t.Parallel`; no test holds an open tx across a `Login` (no `BeginTx` in auth tests), so the
    new `SELECT … FOR UPDATE` in the failed-login path cannot deadlock the suite.
- Serial full run (`go test -count=1 -p 1 -timeout 20m ./...` plus `pnpm --dir web run test`,
  same env as `make test`): `internal/auth` ALSO timed out serially (`FAIL 1200.782s`), while
  serial `internal/files` (56.3s, was 117.8s parallel) and `internal/ledger` (81.7s, was 157.0s)
  nearly halved — direct proof the durations are contention-driven. Serial tail: `internal/server`
  ok 654.3s (was 1093.9s parallel — same proof), `cmd/athena` ok 82.4s, all other packages ok or
  no-test-files, web `pnpm run test` 248/248 passed in 1.5s. So the serial full suite is green
  everywhere except the pre-existing `internal/auth` duration overrun.
- Ambient-load evidence (14:36 local): 8 CPUs with load 4.7/8.1/6.6; another worker's test
  database (`test_athena_b2shots`, 2 sleeping conns ~900s) active on the same MariaDB; only 13
  `test_athena_*` databases total, so orphaned DBs are not the cause.
- Cost bound for this task's diff inside the slow suite: per fresh test DB, two extra migration
  statements (~10–50 ms); per revocation/disable op, one extra in-tx INSERT (<1 ms); zero added
  inserts on the invite path in tests (no principal → no issuer → no notification row). Total
  added across the whole `TestLastOwnerProperty` run: single-digit seconds — three orders of
  magnitude below the observed 535–1200s variance, which tracks machine load instead.
