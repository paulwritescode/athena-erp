# ADR-0005: Database dialect layer
- **Status:** Proposed
- **Date:** 2026-11-19
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0002, ADR-0003, O-001
## Context
MariaDB SQL was written inline in the server: backtick quoting, `GET_LOCK`, `ADD COLUMN IF NOT EXISTS`, `information_schema` lookups, MySQL error numbers, and regex and date functions in the data-loss checks. The kernel specification (section 20) asks for engine-specific SQL behind an interface because a second engine is planned. The project owner decided on 2026-11-18 not to test PostgreSQL for now.
## Options considered
1. **`Dialect` interface in `internal/store` with one MariaDB implementation (chosen).** Keeps engine syntax in one package and lets tests cover it. Costs an extra layer that has a single implementation today.
2. **Leave SQL inline until a second engine exists.** No new code, but the later extraction touches every query at once.
3. **A generic query builder or ORM.** Hides more of the SQL, but the kernel's DDL and journaled saves need exact statements.
## Decision (proposed)
- `internal/store` defines `Dialect`: identifier quoting, length limit and deterministic hash shortening, type mapping, Record-table DDL (create, add, alter, drop, foreign keys), upsert, row lock clause, schema lock, JSON type, `Now()`, catalogue lookups, and error classification (duplicate key, deadlock, serialization failure, lock timeout, foreign key, not null, value too long).
- `store.MariaDB` is the only implementation. Behavior is unchanged: table names, statements and error messages are the same as before.
- `ShortenIdentifier(name, limit)` keeps a name unchanged when it fits and otherwise ends it with `_` and 12 hex characters of its SHA-256, so the result is exactly `limit` bytes. MariaDB uses its own 64-byte limit so existing table names do not change. The invariant test checks the 63-byte limit of PostgreSQL.
- MariaDB never reports a serialization failure separately from a deadlock, so `ErrSerializationFailure` exists for a later engine and MariaDB does not return it.
- Not covered yet: the metadata tables in `server.go` (`migrate`) keep inline DDL until versioned migrations (P2) replace them. Pagination (`LIMIT ?`) is the same on both planned engines and is not abstracted.
## Consequences
- Adding an engine means one new `Dialect` implementation plus its tests; nothing in this ADR claims PostgreSQL support, and PostgreSQL stays untested.
- ADR-0002 is unchanged. Add a "superseded in part" note there only if this ADR is accepted.
- New kernel SQL for Records and entries goes through the dialect.
## Revisit when
A second engine is implemented (roadmap P10), or P2 changes how metadata tables are created.
