# Phase P2 notes: versioned system migrations
Nothing committed or pushed. Existing uncommitted state (`.agents/`) untouched.
## What changed
- `internal/store/migrate.go` (new): `Migration`, `Execer`, `ValidateMigrations`, `MigrationStatus` (read-only), `Up` (schema lock on one connection, record after each step, `--to`, dry run, refuses newer databases, span `athena.migrate.up` with applied count and dry-run flag only). `Dialect.CreateMigrationsTable` added to `store.go` and `mariadb.go`.
- `internal/server/sysmigrations.go` (new): `SystemMigrations()` (1 core_tables, 2 record_operations, 3 entry_history, 4 entry_revisions), `Migrate`, `MigrationStatus` wrappers. Old `migrate()` and `addEntryRevisions` deleted from `server.go`; `utcConfig` extracted there and shared.
- `cmd/athena/migrate.go` (new) and `main.go`: `athena migrate status|up` with `--project`, `--json`, `--to`, `--dry-run`, help text; shared `resolveProject` extracted from `runCommand`. `init/new/run/serve` behavior unchanged. `--addr` is not offered: the command starts no server.
- Tests: `internal/store/migrate_test.go`, `internal/server/sysmigrations_test.go` (fresh vs pre-P2 schema equal), `cmd/athena/migrate_test.go`; `internal/testdb` gained `NewWithConfig`; `helpers_test.go` uses `store.Up`.
- Docs: ADR-0006 (Accepted, per roadmap decision 2), decision-log rows (ADR-0006, O-003, ADR-0003, ADR-0005 note), `docs/README.md`, `docs/architecture/overview.md`, `README.md`, `AGENTS.md`.
- No proto, UI or dependency change (OTel API already in go.mod). Definition migrations, renames, rollback stay for P2a and later.
## Files changed
cmd/athena/main.go, cmd/athena/migrate.go, cmd/athena/migrate_test.go, internal/store/store.go, internal/store/mariadb.go, internal/store/migrate.go, internal/store/migrate_test.go, internal/server/server.go, internal/server/sysmigrations.go, internal/server/sysmigrations_test.go, internal/server/helpers_test.go, internal/testdb/testdb.go, docs/decisions/adr-0006-versioned-system-migrations.md, docs/decisions/decision-log.md, docs/decisions/adr-0005-database-dialect-layer.md, docs/README.md, docs/architecture/overview.md, README.md, AGENTS.md, this file.
## Verification (local MariaDB 11.8, .env.local sourced, values not printed)
- `go build ./...`, `go vet ./...`: pass. `gofmt -l internal cmd`: no output.
- `ATHENA_TEST_DATABASE_URL=... OTEL_EXPORTER_OTLP_ENDPOINT= go test -count=1 ./...`: pass (cmd/athena, internal/server, internal/store). The 13 P0 tests are unedited.
- `make build`, then `bin/athena migrate status`, `up --dry-run`, `up`, `up --json` against the dev database: pending, would apply 1 to 4, applied 1 to 4, then `[]`. `athena help` lists migrate.
- `CI=true pnpm --dir web run typecheck`, `build`, `test`: pass (32 tests). `internal/webui/dist/.gitkeep` removed by the build was restored with `git checkout`.
- Not run: `make generate` (no proto change); `make restart` and browser check (no UI change).
