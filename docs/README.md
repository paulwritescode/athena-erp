# Athena ERP documentation

This directory describes the product and the system as it exists and as decisions are made. The root [`AGENTS.md`](../AGENTS.md) is the short contributor entry point; this page is the map to the detailed docs.

## Current references

- [Architecture overview](architecture/overview.md) — product scope, component boundaries, current implementation status, and not-yet-built capabilities.
- [Record Type and Pack model](architecture/record-type-and-pack-model.md) — Record Types, MariaDB mapping, Go/Starlark boundaries, code generation, and open lifecycle choices.
- [Frontend theme and typography](architecture/frontend-theme-and-type.md) — centralized color tokens, button variants, and the supplied typography scale.
- [Athena design system](design/athena-design-system.md) — Implemented 2026-11-22 (B1–B4): MUJI-based principles, color rule, spacing, type, icons, motion, components per breakpoint, words, ornament, logo, pages by audience. Summary for agents in `.kiro/steering/athena-design.md`.
- [Naming registry](design/naming.md) — Proposed 2026-11-21: product names mapped to CLI, proto service, Go package and UI label; builder and everyday words; forbidden old terms.
- [Athena beta task list](../.kiro/specs/athena-beta/tasks.md) — Draft 2026-11-21: the single ordered plan from design tokens to beta release; supersedes the open tasks of `athena-batch-2` as the working checklist.
- [Bruno API checks](testing/bruno.md) — black-box ConnectRPC smoke tests, local execution and CLI usage.
- [Getting started](guides/getting-started.md) — install, first home, first workspace, first sign-in, first owner; every command tested (B11 slice 1).
- [Decision log](decisions/decision-log.md) — accepted decisions, proposals, unresolved choices, and rejected assumptions.
- [ADR-0002: MariaDB first](decisions/adr-0002-mariadb-first.md) — initial database selection and current persistence limits.
- [ADR-0003: Record table model](decisions/adr-0003-record-table-model.md) — Record Types (then called Record definitions) map to MariaDB tables; amended by ADR-0008 and ADR-0009.
- [ADR-0004: One database per project](decisions/adr-0004-project-per-database.md) — homes, Workspace creation and the run lock; amended by ADR-0008.
- [ADR-0005: Database dialect layer](decisions/adr-0005-database-dialect-layer.md) — proposed: engine-specific SQL behind a `Dialect` interface, MariaDB tested; amended by ADR-0019 (PostgreSQL implemented but untested).
- [ADR-0006: Versioned system migrations](decisions/adr-0006-versioned-system-migrations.md) — accepted: numbered idempotent platform migrations, `athena migrate`.
- [ADR-0007: Exact numeric types](decisions/adr-0007-exact-types.md) — proposed: decimal, money in minor units, quantity, dates and allocation in `internal/types`.
- [ADR-0008: Vocabulary](decisions/adr-0008-vocabulary.md) — proposed: Record Type, Record, Pack, Workspace, home, Grant; supersedes D-005.
- [ADR-0009: Storage shapes](decisions/adr-0009-storage-shapes.md) — proposed: integer minor-unit money, UUIDv7 ids, table naming and the 63-byte identifier rule.
- [ADR-0010: Record kinds and system fields](decisions/adr-0010-record-kinds-and-system-fields.md) — proposed: kinds, system columns, soft delete, field attributes and the money, quantity and enum types.
- [ADR-0011: Document lifecycle](decisions/adr-0011-document-lifecycle.md) — proposed: draft, submitted, cancelled, amend, events, versions and the activity timeline.
- [ADR-0012: Numbering and the audit chain](decisions/adr-0012-numbering-and-audit-chain.md) — proposed: naming series, gap-free counters, hash-chained Audit Log, request correlation and label lookups.
- [ADR-0013: Expression language on cel-go](decisions/adr-0013-expression-language.md) — proposed: restricted CEL, exact decimal and money, formulas and rollups, field rules, SQL-translated filters
- [ADR-0014: Local users, Argon2id, database sessions and Grants](decisions/adr-0014-authentication-and-grants.md) — proposed: sign-in, sessions, CSRF, Roles and Grants with permission levels and row conditions, the permission gate and the Access Log.
- [ADR-0015: Workspace files, and where generated source goes](decisions/adr-0015-workspace-files-and-generated-source.md) — proposed: `FileService`, path and size rules, versions, audit entries, `files:read` and `files:write`, and the `generated/` destination with hash conflict detection (resolves O-002 and D-009 once accepted).
- [ADR-0016: Starlark Rules host on go.starlark.net](decisions/adr-0016-starlark-rules-host.md) — proposed: `internal/rules`, Rule types and events, host functions, limits, Rule Run log, simulate, `rules:read` and `rules:write`; memory-guarded by ADR-0020.
- [ADR-0017: Access model v2](decisions/adr-0017-access-model-v2.md) — proposed: invites with single-use Challenges, scoped dated assignments, whole-assignment evaluation, granular access permissions, no owner bypass, last-owner guard, Permission Explorer.
- [ADR-0018: Workspace root Administrator](decisions/adr-0018-workspace-root-administrator.md) — accepted: one full-bypass account per workspace from the database password at setup, always signs in, cannot be suspended.
- [ADR-0019: PostgreSQL adapter](decisions/adr-0019-postgresql-adapter.md) — proposed: `store.PostgreSQL`, pgx stdlib with `otelsql`, `new` provisions and warns; implemented but untested, no parity, no Docker.
- [ADR-0020: Starlark resource guard](decisions/adr-0020-starlark-resource-guard.md) — proposed: value and row caps, save-time repeat ban, `memory_limit`, `row_limit` and `cascade_limit` codes; amends ADR-0016.
- [ADR-0021: Atomic child and parent Record Type save](decisions/adr-0021-atomic-record-type-group-save.md) — proposed: `save_group` journal plus child compensation; DDL autocommit rules out one transaction.
- [ADR-0022: Ledger foundation and kernel masters](decisions/adr-0022-ledger-foundation-and-kernel-masters.md) — proposed: kernel owns ledger mechanism, seeds nothing; D-007 relation open.
- [ADR-0023: Posting engine and operations](decisions/adr-0023-posting-engine-and-operations.md) — proposed: single-transaction posts, Operation_ID replay, post-once groups; O-011 and O-012 open.
- [ADR-0024: Recipient-owned notifications and Workspace appearance](decisions/adr-0024-notifications.md) — proposed: transactional inbox events; appearance reuses the G5 Settings chain, so no ADR-0025.
- [ADR-0026 Session lifetimes, step-up and rate limiting](decisions/adr-0026-session-policy.md) — proposed B7 server policy and operator settings.
- [ADR-0027: Form layout storage, Image fields and image files](decisions/adr-0027-form-layout-and-images.md) — proposed: layout in the definition JSON; Image values reference `images/`; PutImage with metadata stripping and on-read thumbnails.
- [ADR-0028: Events, jobs and workflows](decisions/adr-0028-events-jobs-and-workflows.md) — proposed: transactional outbox/inbox, leased jobs, schedules, workflows; P16 before P14; secrets and lifetimes deferred (Z4 open).
- [ADR template](decisions/adr-template.md) — format for recording a consequential architecture decision.
- [Project README](../README.md) — setup, run, build, and repository layout.

## Documentation rules

- Label statements as current implementation, accepted direction, proposal, or open question when that distinction could be unclear.
- The decision log is the source of truth for decision status. `AGENTS.md` summarizes it for agents and must be updated in the same change when accepted decisions or key open choices change.
- Record the reason, consequences, and reconsideration trigger for important choices. Do not copy unverified claims from brainstorming into accepted decisions.
- Keep docs close to code changes and verify file paths and commands against the repository before publishing them.
