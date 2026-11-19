# ADR-0009: Storage shapes: money, ids, table and identifier naming

- **Status:** Proposed
- **Date:** 2026-11-19
- **Decision owner:** Project owner
- **Related decisions:** ADR-0003 (amended by ADR-0008), ADR-0005, ADR-0007, ADR-0008
- **Source:** ERP kernel specification sections 20.2 to 20.4 (storage choices, identifier limits, table names) and 23.2 (naming); roadmap section 8, answer B (the agent decides the storage shapes and records them here)

## Context

Later phases add typed money fields, ledgers and packs. Each stores data in a shape that is expensive to change once rows exist: how money is stored, what an id looks like, how tables are named, and how long an identifier may be. The specification lists options for each. MariaDB is the only tested database, and a name that is valid on MariaDB (64 bytes) must also be valid on PostgreSQL (63 bytes) so a later adapter does not force a rename.

## Options considered

1. **Money:** integer minor units plus a currency code (chosen); `DECIMAL` for all money (keeps rounding decisions in the database engine and differs between engines); floating point (rejected, inexact).
2. **Ids:** UUIDv7 in `CHAR(36)` (chosen; time-ordered, already used by Records and journal rows, maps to PostgreSQL `UUID`); ULID (no benefit over UUIDv7 in this stack); `BIGINT` auto-increment (gaps, engine-specific, leaks volume).
3. **Table names:** `p_<pack>__<record_type>`, `l_<ledger>`, `athena_<name>` (chosen); a single prefix with a counter (opaque); the bare Record Type key (collides with system names and across Packs).
4. **Identifier limit:** 63 bytes everywhere with a deterministic hash (chosen); the engine limit per engine (a name valid on one engine fails on the other).

## Decision

- **Money** is stored as `BIGINT` minor units plus a `CHAR(3)` currency code (ADR-0007, spec 20.2). User-defined `Number` and `Currency` fields stay `DECIMAL(18,6)` decimal strings until the typed-field phase (P4) changes them.
- **Ids** are UUIDv7 in `CHAR(36)`.
- **Table names:** Record Type tables are `p_<pack>__<record_type>`, child tables `p_<pack>__<record_type>__<child_field>`, ledgers `l_<ledger>`, and platform tables `athena_<name>`. Record Types authored in the Workspace before any Pack exists live under the reserved pack name `workspace`, so the first table is `p_workspace__<key>`.
- **Identifiers** are ASCII, lowercase snake_case, deterministic and permanent. Case-insensitive collation is never relied on: keys are stored lowercase.
- **Length:** every generated identifier (table, column, index, foreign key) is at most 63 bytes (`store.GeneratedIdentLimit`). A longer name is cut to a prefix, `_` and the first 12 hex characters of the SHA-256 of the full name (`store.ShortenIdentifier`). `Dialect.MaxIdentLength()` stays the engine fact (64 on MariaDB).
- A new Record Type key is at most 50 characters, so `p_workspace__` (13 characters) plus the key is at most 63 and is never shortened. Child tables and keys saved before this change (up to 63 characters) can be, which is why the hash rule exists.
- Foreign keys are named `fk_rtype_<16 hex>` from the SHA-256 of `<record_type>:<field>`.

## Consequences

- System migration 5 moves existing `athena_record_<key>` tables to `p_workspace__<key>` and rewrites foreign key names, without touching row data. It stops and names both tables if an old and a new table exist at once, and refuses to rename when two keys would map to one table.
- The phase that adds typed money fields (P4) writes `BIGINT` plus currency columns on these tables. The ledger phase (P11) uses `l_<ledger>`.
- PostgreSQL stays untested. Nothing here claims it works; the 63-byte rule only keeps names portable.

## Revisit when

A second database engine is implemented, a Pack needs a name longer than the key pattern allows, or the specification changes section 20.
