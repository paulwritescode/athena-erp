# ADR-0002: Use MariaDB first for platform metadata

- **Status:** Accepted
- **Date:** 2026-11-17
- **Decision owner:** Project owner
- **Related decisions:** ADR-0001

## Context

The project needs a relational store that can run locally during development and be hosted for a multi-user deployment. The owner selected the already-installed MariaDB as the initial database. Supporting PostgreSQL at the same time would expand the SQL, schema migration, and operations surface before the first use case is proven.

## Decision

Use MariaDB as the first supported platform metadata database. Configure it with the `ATHENA_DATABASE_URL` environment variable using the Go MySQL driver's DSN format. On startup, Athena verifies the connection and creates initial InnoDB tables for Forms, Modules, and Rules. The first persisted entity is a Form definition, exposed through same-origin JSON endpoints.

The application may run without a configured database so the Build/View desk remains inspectable. In that mode, sample Forms are held in browser memory and are not persistent.

## Consequences

- The default application address is `:3000`; the Go server remains the only runtime process for UI and API.
- MariaDB credentials are supplied through the environment, not checked into the repository.
- Current JSON definition storage is an initial metadata implementation; business Records, module installation, rule execution, and audit history are not implemented.
- PostgreSQL remains a possible later adapter and will require an explicit dialect/migration design.
- The current Form metadata endpoints are JSON/HTTP while the initial system information API uses ConnectRPC. Decide the durable API contract before expanding metadata endpoints.

## Revisit when

Reconsider this choice when a real deployment requires PostgreSQL, when multiple SQL dialects become a customer requirement, or when the first metadata model needs stronger schema migration and versioning guarantees.
