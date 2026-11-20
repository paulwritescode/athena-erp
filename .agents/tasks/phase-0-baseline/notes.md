# Phase P0: baseline tests and doc reconciliation

Nothing was committed or pushed. The working tree still holds the earlier uncommitted changes, untouched.

## Deviations from the roadmap

- **Baseline commit not made.** Roadmap P0 step 1 asks for a local baseline commit after a secret scan. This run's instruction said never to commit, so the step is skipped. The exclusion scan was not run. `.gitignore` already covers `.env.*` (except `.env.example`), `.cache/`, `.pnpm-store/`, `web/node_modules/`, `bin/` and `internal/webui/dist/*`. Do the scan and commit when you approve.
- **No CLI command.** P0 adds no user-facing kernel operation, so no `athena <command>` is added. `init`, `new`, `run` and `serve` are unchanged.
- **No proto, telemetry or UI change.** No `.proto` edit, so `make generate` was run twice only to prove it is stable. `make restart` and the browser check were skipped because no screen changed. The only web change is the `fieldRules.ts` date check, covered by Vitest.

## What was added

- `internal/testdb`: `New(t)` creates a `test_athena_*` database from `ATHENA_TEST_DATABASE_URL`, drops it on cleanup and skips when the variable is unset. The `test_athena_` prefix is used because the local `athena_app` user has no global CREATE, but MariaDB's default grants allow `test\_%` databases to every user.
- `internal/server/{helpers,entries,records,fieldrules}_test.go`. Invariants covered:
  - stale revision rejected on entry update and delete, and the refused update changes nothing;
  - an unchanged update keeps the revision and writes no history;
  - create, update and delete roll back when the history write fails (same transaction);
  - keyset paging returns every entry once, in order, and a page token is refused for another Record;
  - exact decimal round trip (`0.100000` reads back as `0.1`);
  - `SaveRecord`: table provisioned, duplicate key, stale revision, invalid definitions;
  - `PlanRecordSave` and field removal need `confirm_data_loss` when values exist;
  - `DeleteRecord`: linked Records, stale revision, missing Record, `delete_entries` for non-empty Records.
- `internal/server/testdata/field_rules.json`: 30 cases read by both the Go test (`bindValues`) and `web/src/fieldRules.test.ts`.
- Vitest 5.1.1 added as a dev dependency in `web/package.json` (Vite-native runner, matches Vite 8; roadmap decision E). `web/pnpm-lock.yaml` updated. Script `test` runs `vitest run`.
- `make test` (Makefile): Go tests, then Vitest. `ATHENA_TEST_DATABASE_URL` defaults to `ATHENA_DATABASE_URL` from `.env.local`.

## Findings

- Client bug fixed: `fieldRules.ts` accepted `2026-02-30` (`Date.parse` rolls it over); the server rejects it. It now checks the calendar date.
- Known gap left as is (no behavior change in P0): the server accepts an email such as `name@example` (no dot in the domain); the client rejects it. The case is not in the shared table. Decide later whether the server should be stricter.
- The tests ran as `athena_app` and left no `test_athena_*` databases behind (checked with SHOW DATABASES).
- I started the local MariaDB with `brew services start mariadb@11.8` because it was not running.

## Docs reconciled

- `AGENTS.md`: replaced the "only additive changes" rule with the implemented plan-and-confirm behavior; added a tests guardrail.
- `README.md`: same correction, new Tests section, `make test` in the command list.
- `docs/decisions/decision-log.md`: second D-011 (pnpm) renumbered D-012; new D-013 (test approach, Accepted); O-003 context updated.
- `docs/decisions/adr-0002-mariadb-first.md` and `adr-0003-record-table-model.md`: removed stale JSON-endpoint, "CRUD not implemented" and "only adds columns" statements.
- `docs/architecture/overview.md`: date, entry status row, new Automated tests row.
- `docs/architecture/implementation.yaml`: P0 entry.
- `docs/README.md` needed no change (no new doc). No new ADR: P0 changes no boundary, data, contract, security or deployment behavior.

## Commands run and results

| Command | Result |
| --- | --- |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `ATHENA_TEST_DATABASE_URL=<from .env.local> go test -count=1 ./...` | pass (`internal/server` 6.1s, 12 MariaDB tests plus the shared-table test) |
| `go test ./...` with no test URL | pass; MariaDB tests skip |
| `make generate` twice, checksum of `gen/` and `web/src/gen/` | both exit 0, checksums identical |
| `CI=true pnpm --dir web run typecheck` | pass |
| `CI=true pnpm --dir web run build` | pass; `internal/webui/dist` rebuilt (index.html and assets dated 2026-11-19 18:26) |
| `CI=true pnpm --dir web run test` | 31 of 31 pass |
| `make test` | pass |

The test DSN value was never printed.

## Files changed in this phase

New: `internal/testdb/testdb.go`, `internal/server/helpers_test.go`, `internal/server/entries_test.go`, `internal/server/records_test.go`, `internal/server/fieldrules_test.go`, `internal/server/testdata/field_rules.json`, `web/src/fieldRules.test.ts`, `.agents/tasks/phase-0-baseline/notes.md`.

Modified: `web/src/fieldRules.ts`, `web/package.json`, `web/pnpm-lock.yaml`, `Makefile`, `AGENTS.md`, `README.md`, `docs/architecture/overview.md`, `docs/architecture/implementation.yaml`, `docs/decisions/decision-log.md`, `docs/decisions/adr-0002-mariadb-first.md`, `docs/decisions/adr-0003-record-table-model.md`. `internal/webui/dist/*` is rebuilt output (ignored). `gen/` and `web/src/gen/` were regenerated with no content change.
