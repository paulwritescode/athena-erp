# ADR-0002: Use MariaDB as the first relational database

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
