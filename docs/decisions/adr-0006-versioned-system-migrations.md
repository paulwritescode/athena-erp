# ADR-0006: Versioned system migrations

> **Vocabulary:** this ADR keeps the terms and table names of its date. [ADR-0008](adr-0008-vocabulary.md) (Proposed) renames them; system migration 5 (`vocabulary_rename`) applies the rename after migrations 1 to 4, which are unchanged.
- **Status:** Accepted 2026-11-19 (project owner approved the design in roadmap decision 2; accepted when P2 landed)
- **Date:** 2026-11-19
- **Decision owner:** project owner
- **Related decisions:** ADR-0003, ADR-0005, O-003
## Context
The platform tables (`athena_records`, `athena_modules`, `athena_rules`, `athena_record_operations`, `athena_entry_history`) were created by an ad hoc `migrate()` that re-ran every statement at each start. Later phases add columns to populated tables, so schema changes need an order, a record of what ran, and a way to inspect and run them without starting the server. MariaDB commits DDL implicitly, so a migration cannot be wrapped in one transaction.
## Options considered
1. **Numbered Go migrations with a state table (chosen).** Each step is idempotent. A row in `athena_migrations` is written after the step succeeds.
2. **SQL files with an external tool** (goose, golang-migrate). A new dependency and a second place for engine syntax that the dialect layer already owns.
3. **Keep re-running idempotent statements.** No record of what ran and no way to order a change that is not naturally idempotent.
## Decision
- `internal/store/migrate.go` holds the runner. `athena_migrations(version INT PK, name, applied_at)` is created by `Dialect.CreateMigrationsTable`.
- `server.SystemMigrations()` is the ordered list: 1 `core_tables`, 2 `record_operations`, 3 `entry_history`, 4 `entry_revisions`. Versions are contiguous from 1, permanent, and appended only.
- `store.Up` takes the schema lock on one connection, applies pending steps in order and records each one after it succeeds. A crash between a step and its record row re-runs that step, which is safe because every step uses `IF NOT EXISTS` forms. A second run changes nothing.
- A database that records a version this build does not know is refused (a newer database needs a newer `athena`). `status` and `--dry-run` write nothing.
- A database created before versioning re-runs steps 1 to 4 harmlessly and is then recorded, so it reaches the same schema as a fresh one. A test compares the two.
- The server applies pending migrations at start-up. `athena migrate status` and `athena migrate up [--to N] [--dry-run]` do the same through the same package. Neither opens an HTTP endpoint.
- The runner records one span, `athena.migrate.up`, with the applied count and the dry-run flag only.
- Migration statements are MariaDB syntax. PostgreSQL stays untested; a later engine phase translates them.
## Not decided here
Record-definition migrations (renames through `renamed_from`, `def_version`, system columns per Record kind), rollback, and Module upgrades. They stay under O-003 and roadmap phase P2a onward.
## Consequences
- New columns on platform tables ship as a new numbered migration, not an edit to an applied one.
- The per-save planner (`PlanRecordSave`) is unchanged and still owns Record tables.
## Revisit when
Record-definition migrations are designed (P2a), or a second engine needs different DDL.
