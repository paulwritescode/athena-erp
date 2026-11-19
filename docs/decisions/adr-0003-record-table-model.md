# ADR-0003: A Record definition maps to a MariaDB table

> **Amended by [ADR-0008](adr-0008-vocabulary.md) and [ADR-0009](adr-0009-storage-shapes.md) (both Proposed).** This ADR keeps the vocabulary of its date: read "Record definition" as Record Type, "Record entry" as Record and "Module" as Pack. The table name `athena_record_<key>` is now `p_workspace__<key>`. See the Amendments section.

- **Status:** Accepted
- **Date:** 2026-11-17
- **Decision owner:** Project owner
- **Related decisions:** ADR-0001, ADR-0002

## Context

Athena is a reusable ERP core, not an application preconfigured for one industry's workflows. Users and code packages define Records on top of the platform. Each Record definition must have a direct relational representation so the database remains understandable and can enforce keys and relationships.

## Decision

A Record definition describes one table; each Record entry is one row. Go validates the stable Record/field identifiers and owns SQL type mapping and DDL. The dashboard can author definitions, and the codebase can declare them. Link fields identify another Record and are represented by a MariaDB foreign key when both tables exist.

The accepted product direction also requires dashboard-authored Records to produce Go source files for typed definitions and behavior hooks. Generated Go code only takes effect after a new executable is built and started. Source generation and the dashboard/codebase synchronization workflow are not implemented yet.

## Consequences

- MariaDB is the current and only active relational target. PostgreSQL can be added later behind an explicit dialect and migration design.
- Metadata and the per-Record table are separate: storing a JSON definition does not create Record entries. Record-entry CRUD is implemented through `EntryService`.
- Modules must materialize their relationships as foreign keys or join tables; merely listing Records in a Module is insufficient.
- SQL identifiers come from validated stable names, never display labels or arbitrary source text.
- Each Record save is planned and applied by a per-save planner (`PlanRecordSave`, `internal/server/migrations.go`). It adds, removes and retypes columns and backfills required fields, with confirmation for data loss. It is not a versioned migration system: rename detection, rollback, module upgrades, and safe database restoration need future design (O-003).
- Dashboard-generated Go code creates a rebuild/restart boundary. It cannot be dynamically compiled inside the running server.
- Until authentication and authorization exist, database-backed authoring endpoints are for local development only.

## Revisit when

Revisit the naming, DDL, or source-generation contract before adding Record-entry writes, production dashboard authoring, module installation, or a second SQL database.

## Amendments

- **ADR-0008 (Proposed):** the definition is called a Record Type and a row is called a Record. `EntryService` is now `RecordService`, and `RecordService` (definitions) is `RecordTypeService`. `PlanRecordSave` is `PlanRecordTypeSave`.
- **ADR-0009 (Proposed):** a Record Type's table is `p_<pack>__<key>` (`p_workspace__<key>` for the Workspace's own Record Types) instead of `athena_record_<key>`, shortened at 63 bytes. System migration 5 renames existing tables.
