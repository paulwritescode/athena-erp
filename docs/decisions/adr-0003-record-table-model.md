# ADR-0003: A Record definition maps to a MariaDB table

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
- Metadata and the per-Record table are separate: storing a JSON definition does not create Record entries. Record-entry CRUD is not yet implemented.
- Modules must materialize their relationships as foreign keys or join tables; merely listing Records in a Module is insufficient.
- SQL identifiers come from validated stable names, never display labels or arbitrary source text.
- The initial schema updater only adds columns. Rename/drop/type migrations, backfills, rollback, module upgrades, and safe database restoration need future design.
- Dashboard-generated Go code creates a rebuild/restart boundary. It cannot be dynamically compiled inside the running server.
- Until authentication and authorization exist, database-backed authoring endpoints are for local development only.

## Revisit when

Revisit the naming, DDL, or source-generation contract before adding Record-entry writes, production dashboard authoring, module installation, or a second SQL database.
