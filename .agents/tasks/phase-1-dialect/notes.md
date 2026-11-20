# Phase P1: storage dialect layer (MariaDB only)
Nothing was committed or pushed. Earlier uncommitted work (`.agents/`) is untouched.

## Deviations and scope notes
- **No CLI command.** P1 is a refactor with no user-facing operation, so no `athena <command>` is added. `init`, `new`, `run`, `serve` are unchanged.
- **No proto, telemetry or UI change.** `make generate` was not run (no `.proto` edit). `make restart` and the browser check were skipped because no screen changed. No new spans or attributes.
- **Not moved into the dialect:** the metadata-table DDL in `server.go` `migrate` (inline until P2), `internal/project` SQL (`information_schema` schema lookups for project creation; outside the roadmap's file list), and `LIMIT ?` pagination (identical on both planned engines).
- The 64-byte MariaDB limit is kept for table names so existing tables do not change. `ShortenIdentifier` is tested at 63 bytes.
- `Upsert`, `JSONType` and `Now` are in the interface per the roadmap; only `Now` is used (in `CreateRecordTable`). `Upsert` and `JSONType` are covered by unit tests only.
- No new dependency. `.env.local` sets `OTEL_EXPORTER_OTLP_ENDPOINT`; I unset it for the test run only.

## What changed
- New `internal/store`: `Dialect` interface and `ShortenIdentifier` (`store.go`), `MariaDB` implementation (`mariadb.go`).
- `internal/server/{records,entries,migrations,server}.go` call a package-level `dialect` for quoting, DDL, catalogue lookups, schema lock, row lock, type mapping, data-loss predicates and error classification. Removed: `mariadbColumnType`, `incompatiblePredicate`, `tableExists`, the `mysql` imports in `records.go` and `entries.go`.
- Docs: ADR-0005 (Proposed), decision-log row, `docs/README.md`, `docs/architecture/overview.md` status rows, README layout line. `AGENTS.md` unchanged (no accepted decision changed).

## Verification
Run with `.env.local` sourced, `ATHENA_TEST_DATABASE_URL` set to the project DSN (value never printed), MariaDB 11.8 running under brew services.
- `go build ./...`: pass.
- `go vet ./...`: pass.
- `gofmt -l internal cmd`: no output.
- `go test -count=1 ./...`: `internal/server` ok (6.2s, all 13 P0 MariaDB characterization tests ran and passed, none edited), `internal/store` ok (unit tests plus `TestMariaDBCatalogueAndLock` against MariaDB).
- `CI=true pnpm --dir web run typecheck`, `build`, `test`: pass (32 Vitest tests). The build deleted the tracked `internal/webui/dist/.gitkeep`; restored with `git checkout -- internal/webui/dist/.gitkeep`.

## Files
Modified: `README.md`, `docs/README.md`, `docs/architecture/overview.md`, `docs/decisions/decision-log.md`, `internal/server/entries.go`, `internal/server/migrations.go`, `internal/server/records.go`, `internal/server/server.go`.
Added: `docs/decisions/adr-0005-database-dialect-layer.md`, `internal/store/store.go`, `internal/store/mariadb.go`, `internal/store/store_test.go`, `internal/store/mariadb_db_test.go`, `.agents/tasks/phase-1-dialect/notes.md`.
