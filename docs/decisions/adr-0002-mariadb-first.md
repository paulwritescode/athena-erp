# ADR-0002: Use MariaDB as the first relational database

> **Vocabulary:** this ADR keeps the terms of its date. [ADR-0008](adr-0008-vocabulary.md) (Proposed) renames Record definition to Record Type, Record entry to Record and Module to Pack.
>
> **Amended in part by [ADR-0019](adr-0019-postgresql-adapter.md) (Proposed).** A PostgreSQL adapter exists (dialect, driver wiring, provisioning) but is untested and claims no parity; MariaDB stays the only active target.

- **Status:** Accepted
- **Date:** 2026-11-17
- **Decision owner:** Project owner
- **Related decisions:** ADR-0001, ADR-0003

## Context

Athena needs a relational database that can run locally and be hosted for multiple users. The owner selected the already-installed MariaDB for the first implementation. Supporting PostgreSQL at the same time would expand the SQL, migration, and operations surface before the core Record model is proven.

## Decision

Use MariaDB as the only active relational database target for now. Configure the Go MySQL driver with `ATHENA_DATABASE_URL`. On startup, Athena verifies the connection and creates metadata tables. Saving a Record definition stores its metadata and provisions the Record's table, as detailed in [ADR-0003](adr-0003-record-table-model.md).

The app can run without a database DSN so the core desk remains inspectable. It does not seed domain-specific Records or Modules.

## Consequences

- The default Go server address is `127.0.0.1:3000` and UI/API share the same process. Loopback is required while database-backed authoring endpoints lack authentication.
- Credentials are supplied through environment/configuration and never committed.
- Module persistence/composition, authentication, Starlark execution, and versioned migrations remain incomplete. Record-entry CRUD exists without authentication.
- PostgreSQL is a later adapter and requires a deliberate dialect/migration plan.
- Record metadata, entries and system info use same-origin ConnectRPC with a Protobuf contract (D-003). The earlier JSON endpoint was removed.

## Revisit when

Reconsider when there is a concrete PostgreSQL deployment need, when SQL dialects become a customer requirement, or before the current schema updater is used for production upgrades.
