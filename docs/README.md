# Athena ERP documentation

This directory describes the product and the system as it exists and as decisions are made. The root [`AGENTS.md`](../AGENTS.md) is the short contributor entry point; this page is the map to the detailed docs.

## Current references

- [Architecture overview](architecture/overview.md) — product scope, component boundaries, current implementation status, and not-yet-built capabilities.
- [Record Type and Pack model](architecture/record-type-and-pack-model.md) — Record Types, MariaDB mapping, Go/Starlark boundaries, code generation, and open lifecycle choices.
- [Frontend theme and typography](architecture/frontend-theme-and-type.md) — centralized color tokens, button variants, and the supplied typography scale.
- [Decision log](decisions/decision-log.md) — accepted decisions, proposals, unresolved choices, and rejected assumptions.
- [ADR-0002: MariaDB first](decisions/adr-0002-mariadb-first.md) — initial database selection and current persistence limits.
- [ADR-0003: Record table model](decisions/adr-0003-record-table-model.md) — Record Types (then called Record definitions) map to MariaDB tables; amended by ADR-0008 and ADR-0009.
- [ADR-0004: One database per project](decisions/adr-0004-project-per-database.md) — homes, Workspace creation and the run lock; amended by ADR-0008.
- [ADR-0005: Database dialect layer](decisions/adr-0005-database-dialect-layer.md) — proposed: engine-specific SQL behind a `Dialect` interface, MariaDB only.
- [ADR-0006: Versioned system migrations](decisions/adr-0006-versioned-system-migrations.md) — accepted: numbered idempotent platform migrations, `athena migrate`.
- [ADR-0007: Exact numeric types](decisions/adr-0007-exact-types.md) — proposed: decimal, money in minor units, quantity, dates and allocation in `internal/types`.
- [ADR-0008: Vocabulary](decisions/adr-0008-vocabulary.md) — proposed: Record Type, Record, Pack, Workspace, home, Grant; supersedes D-005.
- [ADR-0009: Storage shapes](decisions/adr-0009-storage-shapes.md) — proposed: integer minor-unit money, UUIDv7 ids, table naming and the 63-byte identifier rule.
- [ADR-0010: Record kinds and system fields](decisions/adr-0010-record-kinds-and-system-fields.md) — proposed: kinds, system columns, soft delete, field attributes and the money, quantity and enum types.
- [ADR-0011: Document lifecycle](decisions/adr-0011-document-lifecycle.md) — proposed: draft, submitted, cancelled, amend, events, versions and the activity timeline.
- [ADR-0012: Numbering and the audit chain](decisions/adr-0012-numbering-and-audit-chain.md) — proposed: naming series, gap-free counters, hash-chained Audit Log, request correlation and label lookups.
- [ADR-0013: Expression language on cel-go](decisions/adr-0013-expression-language.md) — proposed: restricted CEL, exact decimal and money, formulas and rollups, field rules, SQL-translated filters
- [ADR template](decisions/adr-template.md) — format for recording a consequential architecture decision.
- [Project README](../README.md) — setup, run, build, and repository layout.

## Documentation rules

- Label statements as current implementation, accepted direction, proposal, or open question when that distinction could be unclear.
- The decision log is the source of truth for decision status. `AGENTS.md` summarizes it for agents and must be updated in the same change when accepted decisions or key open choices change.
- Record the reason, consequences, and reconsideration trigger for important choices. Do not copy unverified claims from brainstorming into accepted decisions.
- Keep docs close to code changes and verify file paths and commands against the repository before publishing them.
