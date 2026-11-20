# Implementation Plan: P2 versioned migrations

Base: commit 4b38b85 (P1). Source: `.agents/tasks/erp-kernel-roadmap.md` section P2. User decision: P2 must ship `athena migrate` (status/up) with help text and docs.

Design choice (the roadmap marks it OPEN, ADR-0006 stays **Proposed**): an ordered Go slice of system migrations, each with a version number, name and idempotent `Up(ctx, db, dialect)`. State lives in `athena_migrations(version INT PK, name, applied_at)`. A migration is recorded after its statements succeed. Because MariaDB commits DDL implicitly, every step uses `IF NOT EXISTS` forms so a crash between a step and its record row is repaired by re-running. The runner holds `dialect.LockSchema` while applying. Definition-driven migrations (renames, `renamed_from`) stay out of P2; they belong to P2a.

Pre-P2 databases: v1 is the current `migrate()` body plus `addEntryRevisions`, all idempotent. On a pre-P2 database v1 re-runs harmlessly and is then recorded, so fresh and pre-P2 databases reach the same schema.

- [ ] 1. Add the runner in `internal/store/migrate.go`.
      Types: `Migration{Version int; Name string; Up func(ctx, Querier) error}`, `Status{Version, Name, Applied bool, AppliedAt}`. Functions: `Status(ctx, db, dialect, []Migration)`, `Up(ctx, db, dialect, []Migration, UpOptions{Target int, DryRun bool})`. It creates `athena_migrations` itself (dialect DDL), takes the schema lock, applies pending migrations in order, and records each. It errors if the database records a version this build does not know (newer database). It refuses duplicate or non-contiguous versions.
      Files: `internal/store/migrate.go`, `internal/store/migrate_test.go`, possibly one new `Dialect` method for the state-table DDL in `store.go` and `mariadb.go`.
      Verify: `go test ./internal/store` with `ATHENA_TEST_DATABASE_URL`. Tests: runs twice and changes nothing; simulated crash (Up fails after a step, before its record) resumes and completes; unknown newer version is rejected; DryRun writes nothing.

- [ ] 2. Move system DDL into numbered migrations, and wire the server.
      Create `internal/server/sysmigrations.go` with `SystemMigrations()`: v1 `core_tables` (athena_records, athena_modules, athena_rules, revision column), v2 `record_operations` (journal plus `action` and `options` columns), v3 `entry_history`, v4 `entry_revisions` (the `addEntryRevisions` logic, with `legacyRecordKeyPattern`). Statements are copied verbatim from `server.go` `migrate`. Replace `migrate(db)` at `server.go:103` with `store.Up`. Delete `migrate`, `addEntryRevisions` and their now-unused imports in the same change. `internal/server/helpers_test.go:15` calls `migrate(db)`; switch it to the new entry point. Start-up behavior is unchanged: pending migrations apply on `athena run` and `athena serve`.
      Files: `internal/server/sysmigrations.go`, `internal/server/server.go`, `internal/server/helpers_test.go`.
      Verify: `go test -count=1 ./...` with MariaDB. All 13 P0 tests are unedited and pass. New test in `internal/server/sysmigrations_test.go`: a database built from the old statements (copied into the test as a fixture of the pre-P2 schema) and a fresh database have identical `information_schema` columns for the `athena_*` tables after `Up`.

- [ ] 3. Add `athena migrate` in `cmd/athena/main.go`.
      Subcommands: `athena migrate status` and `athena migrate up [--to N] [--dry-run]`. Both accept `--project slug` (as `run` does) and fall back to `ATHENA_DATABASE_URL` when no project resolves (as `serve` does). Extract the shared project/DSN resolution from `runCommand`/`serveCommand` into one small helper rather than copying it (rule: reuse). `status` prints version, name, applied or pending, and applied time. `up` prints each applied version. Neither prints the DSN. Add `migrate` to the `run` switch and to `printUsage` with help text. Export a thin `server.Migrate`/`server.MigrationStatus` pair so the CLI does not import the migration list directly. Open the database with the project's existing connection code; do not start the HTTP server or take the project run lock.
      Files: `cmd/athena/main.go`, `internal/server/sysmigrations.go` (exported wrappers), `cmd/athena/main_test.go` if a CLI test pattern exists (check first; otherwise test the wrappers in `internal/server`).
      Verify: `go build ./... && bin/athena migrate status` and `athena migrate up --dry-run` against the local MariaDB (use `make build`). `athena help` lists `migrate`. Run `up` twice; the second prints nothing pending.

- [ ] 4. Docs.
      ADR-0006 (`docs/decisions/adr-0006-versioned-system-migrations.md`, status Proposed, from `adr-template.md`), a decision-log row (O-003 remains open until you accept), `docs/README.md` entry, `docs/architecture/overview.md` status rows (system migrations implemented; definition migrations, rollback, module upgrades not implemented), and `README.md` commands for `athena migrate`. `AGENTS.md`: update the line saying "Versioned migrations ... are not implemented" to separate system migrations (implemented) from Record-definition migrations (not) only if the ADR is accepted; otherwise leave it and say so.
      Files: the docs listed above.
      Verify: commands in the README run from the repo root and match `athena help` output.

- [ ] 5. Full verification.
      `go build ./...`, `go vet ./...`, `gofmt -l internal cmd` (no output), `go test -count=1 ./...` with `ATHENA_TEST_DATABASE_URL` set, `CI=true pnpm --dir web run typecheck`, `build`, `test`. The web build deletes `internal/webui/dist/.gitkeep`; restore it with `git checkout -- internal/webui/dist/.gitkeep`. No proto change, so `make generate` is not needed. No UI change.

## Digest
- `internal/server/server.go`: `migrate(db *sql.DB) error` at line 168 (statements through line ~233) and `addEntryRevisions` at 237, called at line 103. Covered by every MariaDB test through `helpers_test.go:15`. Verified: `migrate` is the only caller chain; nothing else references `addEntryRevisions` (confirm with a symbol search before deleting).
- `internal/store/store.go`: `Dialect` has `LockSchema(ctx, q Querier) (acquired bool, release func(), err error)` (line ~102), `AddRevisionColumn`, `AddColumn`, `TableExists`. Covered by `internal/store/store_test.go` and `mariadb_db_test.go`.
- `cmd/athena/main.go`: `run` switch at line ~43, `printUsage` at line 64, `runCommand` at 177, `serveCommand` at 212, `parse(flags, args)` helper. `runCommand` uses `project.Resolve`, `project.LoadEnv`, `project.ServerConfig`; `serveCommand` uses `mysql.ParseDSN(ATHENA_DATABASE_URL)`. No CLI test file was seen; check before assuming.
- Build and test commands (from the roadmap's common verification and the P1 notes): `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, web commands above. `.env.local` sets `OTEL_EXPORTER_OTLP_ENDPOINT`; unset it for test runs only. MariaDB 11.8 runs under brew services. At commit 4b38b85 all of these passed (P1 review).
- Not read in this plan: `internal/project` connection code and `helpers_test.go` contents. The coder opens them to edit.
