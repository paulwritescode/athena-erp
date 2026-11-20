# Phase P2 summary: versioned system migrations

Review verdict: APPROVED, no findings. Nothing committed or pushed.

## What changed
- `internal/store/migrate.go` (new): migration runner with contiguous-version validation, schema lock on one connection, a recorded row after each step, `--to`, dry run, refusal of newer databases, and read-only status. Span `athena.migrate.up` carries only the applied count and dry-run flag. `Dialect.CreateMigrationsTable` added to `store.go` and `mariadb.go`.
- `internal/server/sysmigrations.go` (new): `SystemMigrations()` with 1 core_tables, 2 record_operations, 3 entry_history, 4 entry_revisions, plus `Migrate` and `MigrationStatus`. The old `migrate()` and `addEntryRevisions` were deleted from `server.go`.
- `cmd/athena/migrate.go` (new) and `main.go`: `athena migrate status|up` with `--project`, `--json`, `--to`, `--dry-run`. `init/new/run/serve` are unchanged. `--addr` is not offered because the command starts no server.
- Tests: `internal/store/migrate_test.go`, `internal/server/sysmigrations_test.go` (fresh and pre-P2 schemas equal), `cmd/athena/migrate_test.go`. `internal/testdb` gained `NewWithConfig`.
- Docs: ADR-0006 Accepted (roadmap answer 2), decision-log rows, O-003 narrowed, ADR-0003/0005 notes, `docs/README.md`, overview, `README.md`, `AGENTS.md`.
- No proto, UI or dependency change.

## Verification (local MariaDB 11.8, recorded by the coder; the reviewer did not re-run suites)
- `go build ./...`, `go vet ./...`, `gofmt -l internal cmd`: pass, no output.
- `go test -count=1 ./...` with `ATHENA_TEST_DATABASE_URL` set and OTel off: pass. The 13 P0 tests are unedited.
- `make build`, then `athena migrate status`, `up --dry-run`, `up`, `up --json` on the dev database: pending, would apply 1 to 4, applied 1 to 4, then `[]`.
- Web `typecheck`, `build`, `test`: pass (32 tests). The build removed `internal/webui/dist/.gitkeep`; it was restored.
- Not run: `make generate` and browser check (no proto or UI change).

## Non-blocking note
The roadmap lists definition migrations (`renamed_from`, `def_version`) under P2. ADR-0006 defers them to P2a, and the docs say so.

## Next
The roadmap order puts **P2a (vocabulary rename and storage naming ADR)** between P2 and P3. P3 does not depend on P2a structurally, but later phases use the new words. Please say whether to run P2a first or go straight to P3.

**P3: exact types (decimal, money, quantity, dates, allocation).** Spec 3.1, 3.4, decision 17.5.
- New pure-Go package `internal/types`: decimal, money (integer minor units plus currency), quantity with unit of measure, rounding modes half_up and bankers, deterministic `Allocate`. No UI, no proto change. It is wired into `entries.go` later by P4.
- ADR-0007 (Proposed): scaled integers versus decimal strings. Compare `github.com/shopspring/decimal` with `math/big`, pick one and justify any new dependency.
- Property tests with `testing/quick` or `pgregory.net/rapid` (named in the ADR). Invariants: allocation sums back exactly, rounding does not drift across split and re-add, and no float conversion (a test greps the package for `float`).
- CLI parity: the roadmap requires a CLI path for every kernel capability. This phase exposes none, so the plan must state that explicitly.
- Verification: build, vet, gofmt, `go test ./...` on MariaDB, web suite unchanged.
