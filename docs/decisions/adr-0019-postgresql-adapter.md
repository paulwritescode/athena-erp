# ADR-0019: PostgreSQL adapter, implemented but untested

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0002, ADR-0005, O-001

## Context

O-001 asked when the PostgreSQL adapter should be added. The project owner decided on 2026-11-18 not to test PostgreSQL for now (ADR-0005), and batch-2 group G2 was scoped to prove the dialect seam can carry a second engine without claiming it works: implemented but untested, no parity claim, no Docker. The adapter must fit the single Go binary (no CGO) and keep `make test` green on MariaDB, the only tested engine.

## Options considered

1. **pgx stdlib plus `otelsql` with a `Dialect` implementation (chosen).** `store.PostgreSQL` covers every `Dialect` method; the driver opens through `pgx/v5/stdlib` wrapped by `otelsql`, so Postgres gets the same traced statements as MariaDB. pgx is pure Go, so the single binary survives. Placeholders stay `?` in shared code and `Rebind` numbers them (`$1`, …) for the future query-layer port; it has no callers yet.
2. **A non-`database/sql` driver (pgx native, `lib/pq`).** Splits connection handling away from the MariaDB path and loses the shared `otelsql` wrapper.
3. **Leave PostgreSQL unimplemented until a deployment need.** Defers proving the seam; the extraction in ADR-0005 would sit unverified against a second engine.

## Decision (proposed)

- `internal/store/postgres.go` implements the whole `Dialect` seam: double-quote quoting, the same 63-byte deterministic shortening, type mapping, Record Type DDL, upsert, explicit row locks, UTC session (`timezone=UTC`, `datestyle=ISO, MDY`), catalogue lookups and SQLSTATE error classification.
- `workspace.DBEndpoint` carries the driver plus its parsed config; `server.Config`, `OpenKernel`, `Migrate` and the CLI open paths switch on it. Each process sets the global dialect once from the endpoint driver when it opens the database. `athena new --driver` must match the home driver and prints an untested warning for postgres; `athena init` stays MariaDB-only, so a postgres home is hand-authored.
- Honest scope: only the dialect seam, the driver wiring and workspace provisioning exist. The versioned system migrations and the server query layer are still MariaDB-shaped, so a postgres workspace provisions with `new` but does not boot with `run` until they are ported. Nothing here claims parity, and nothing requires Docker.

## Consequences

- Amends ADR-0002 (an adapter exists; MariaDB stays the only active target) and ADR-0005 (MariaDB is no longer the only implementation, but stays the only tested one). Resolves O-001 only as "adapter exists".
- `make test` stays green on MariaDB; the postgres tests assert SQL strings, SQLSTATE classification, quoting and endpoint parsing with no database connection.
- Accepting this ADR still claims nothing about running on PostgreSQL; that needs ported migrations and queries plus live tests.

## Revisit when

A concrete PostgreSQL deployment need appears: port the system migrations and the query layer (including `Rebind` call sites), test against a live server, and only then claim parity.
