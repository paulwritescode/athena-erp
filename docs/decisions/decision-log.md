# Decision log

This is the canonical status of product and architecture choices. Keep [`AGENTS.md`](../../AGENTS.md) aligned when accepted direction changes.

## Accepted decisions and direction

| ID | Decision | Status | Notes |
| --- | --- | --- | --- |
| ADR-0001 | One Go server serves the embedded frontend and API | Accepted | Vite builds static assets; Go serves them with ConnectRPC. Default address is port 3000. |
| ADR-0002 | MariaDB is the first relational database | Accepted | PostgreSQL is a later adapter. See [ADR-0002](adr-0002-mariadb-first.md). |
| ADR-0003 | A Record definition maps to one MariaDB table | Accepted, amended by ADR-0008 and ADR-0009 (Proposed) | Go validates the schema and provisions the table. Read "Record definition" as Record Type and "Record entry" as Record: the definition is saved through `RecordTypeService`, rows through `RecordService`, and the table is `p_workspace__<key>`. Versioned migrations cover platform tables only (ADR-0006); Record Type migrations remain incomplete. See [ADR-0003](adr-0003-record-table-model.md). |
| ADR-0004 | One database per project on the workspace database server | Accepted, amended by ADR-0008 (Proposed) | Read "project" as Workspace and the parent "workspace" folder as home. `athena new` creates `<home>/<slug>` and database `<slug>`, fixed at creation; `athena run` checks the Workspace ID and holds a per-user lock. See [ADR-0004](adr-0004-project-per-database.md). |
| ADR-0005 | Engine-specific SQL sits behind a `Dialect` interface in `internal/store` | Proposed 2026-11-19 | MariaDB is the only implementation and the only tested engine; no PostgreSQL code or support claim. See [ADR-0005](adr-0005-database-dialect-layer.md). |
| ADR-0006 | Platform tables are created by numbered, idempotent system migrations recorded in `athena_migrations` | Accepted 2026-11-19 | Applied at start-up and by `athena migrate status/up`. Record Type migrations, rollback and Pack upgrades are not covered (O-003). Migration 5 (`vocabulary_rename`) applies ADR-0008 and ADR-0009. See [ADR-0006](adr-0006-versioned-system-migrations.md). |
| ADR-0007 | Exact types live in `internal/types` on `math/big`: decimals, money as `int64` minor units plus currency, quantities, UTC datetimes, deterministic allocation | Proposed 2026-11-19 | Pure Go library, not yet wired into fields or entries (P4). No new dependency. See [ADR-0007](adr-0007-exact-types.md). |
| ADR-0008 | Vocabulary: Record Type (definition), Record (row), Pack (installable bundle), Workspace (database plus installed packs and data), home (parent folder), Grant (permission rule) | Proposed 2026-11-19 | Supersedes D-005 and amends ADR-0003 and ADR-0004. Implemented by system migration 5, the Protobuf rename (`RecordTypeService`, `RecordService`) and `--workspace`. See [ADR-0008](adr-0008-vocabulary.md). |
| ADR-0009 | Storage shapes: integer minor-unit money, UUIDv7 ids, `p_<pack>__<record_type>` / `l_<ledger>` / `athena_<name>` table names, lowercase deterministic identifiers, 63-byte limit with hash shortening | Proposed 2026-11-19 | PostgreSQL remains untested. See [ADR-0009](adr-0009-storage-shapes.md). |
| ADR-0010 | Record kinds (master, document, child, setting), kernel system fields, master soft delete and enforced field attributes; money, quantity and enum field types | Proposed 2026-11-19 | `revision` serves as the spec's `version`. Migration 6 makes existing Record Types masters. Ledger, balance, log, snapshot and meta kinds stay kernel-only. See [ADR-0010](adr-0010-record-kinds-and-system-fields.md). |
| ADR-0012 | Numbering series on counter rows (gap-free option), a single hash-chained Audit Log with anchors and `athena audit verify`, request correlation through an interceptor, label lookups for pickers | Proposed 2026-11-19 | See [ADR-0012](adr-0012-numbering-and-audit-chain.md). |
| D-014 | Soft-deleted Masters keep their unique values reserved | Accepted by default 2026-11-19 (owner may override) | A restore never collides, and references and history stay unambiguous. To reuse a value, rename or hard-purge the old Record; purge is not in scope yet. Applies ADR-0010. |
| D-015 | Documents have a kernel-owned draft, submitted, cancelled state with versions and a timeline | Proposed 2026-11-19 | Only the Submit, Cancel and Amend RPCs move `docstate`; submitted documents change only in `allow_on_submit` fields; cancel is blocked by a live downstream document; amend copies all but unique, read-only and no-copy fields. See ADR-0011. |
| D-016 | Numbering by counter rows, a hash-chained Audit Log, and one request ID per call | Proposed 2026-11-19 | Series numbers are taken at submit (gap-free inside the transaction, otherwise outside it); names never change after they are set; the Audit Log replaces the Record history and verifies with `athena audit verify`; Link pickers search by label through `LookupRecords`. See ADR-0012. |
| D-011 | OpenTelemetry for traces and metrics, viewed locally in Grafana (`grafana/otel-lgtm`) | Accepted 2026-11-18 | Optional: off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Server and browser both trace; browser spans go through the Athena origin. Production collector and retention are not chosen. |
| D-002 | Vue 3 + TypeScript + Vite is the single frontend stack | Accepted | Do not introduce another frontend runtime. |
| D-003 | ConnectRPC is the Go RPC library and Protobuf is the API contract | Accepted | Record Types, Records and system info all use ConnectRPC. The JSON `/api/records` endpoint was removed on 2026-11-17. |
| D-004 | Go owns trusted behavior; Starlark supplies configurable rules | Accepted direction | Rules may reference Record Type fields and Pack relationships through limited Go capabilities. Execution limits and lifecycle remain open. |
| D-005 | Use “Record” for table definitions and “Record entry” for rows | Superseded by ADR-0008 (Proposed 2026-11-19) | Original wording: a Module bundles Records and relationships. Until ADR-0008 is accepted, the repository already uses its vocabulary: Record Type, Record, Pack, Workspace. |
| D-006 | Cloud provider integrations remain optional to the ERP core | Accepted direction | AWS, Cloudflare, Cognito, queues, and object storage are not core dependencies. |
| D-007 | Athena is an ERP platform core, not a preconfigured industry ERP | Accepted product scope | Start without industry-specific seed Record Types or Packs. Domain products can be installed later. |
| D-008 | shadcn-vue is the primary component system with a theme toggle | Accepted | Existing Vite app uses preset `aeaM05w`, resolved to `reka-luma`, Zinc tokens, Inter, token radius, pointer cursor, and class-based dark mode. |
| D-009 | Record Types can be authored in the dashboard and codebase; dashboard authoring generates Go source scaffolds | Accepted direction | Generated Go is source for the developer/codebase workflow and requires build/deploy before it affects the server. A Record Type save only updates metadata/schema. Generator destination, ownership, synchronization, and upgrades are not implemented; see [Record Type and Pack model](../architecture/record-type-and-pack-model.md). |
| D-010 | Keep the frontend palette and typography as shared semantic token sets | Accepted | Edit colors in `web/src/theme.css` and the supplied type scale in `web/src/typography.css`; action buttons use shadcn-vue semantic variants for readable text in both themes. |
| D-012 | Use pnpm as the only frontend package manager | Accepted | Shared desk pins pnpm 11.22.0, commits `web/pnpm-lock.yaml`, and installs with a frozen lockfile before typecheck/build. npm and Yarn are unsupported. Renumbered from a second D-011 on 2026-11-19. |
| D-013 | Tests: Go `testing` against throwaway MariaDB databases, Vitest for the desk | Accepted 2026-11-19 | `internal/testdb` creates a `test_athena_*` database per test from `ATHENA_TEST_DATABASE_URL` and skips when unset. Vitest is the only added frontend dev dependency; it shares `internal/server/testdata/field_rules.json` with the Go tests so client and server field rules stay in step. `make test` runs both. MariaDB is the only tested database. |

## Open decisions

| ID | Question | Current context |
| --- | --- | --- |
| O-001 | When should the PostgreSQL adapter be added? | MariaDB is the only active database target; wait for a concrete need and a dialect/migration design. |
| O-002 | How do dashboard and codebase Record Type definitions synchronize? | Decide ownership, precedence, conflict detection, generated-file boundaries, and deployment behavior. |
| O-003 | How are Record Type schema upgrades reviewed and applied? | A per-save planner handles add, remove, type-change and required-field backfills with confirmation (`PlanRecordTypeSave`). System migrations are versioned (ADR-0006, accepted), including the vocabulary rename (migration 5). Record Type migrations, rename detection and rollback are still open. |
| O-004 | Which relationship cardinalities are supported first? | Link fields currently represent a direct foreign-key reference. Pack composition and join tables are not implemented. Define delete/update rules. |
| O-005 | How are Packs versioned, installed, upgraded, and removed? | Packs must package Record Types, relationships, rules, UI, dependencies, and migrations. |
| O-006 | What is the tenancy model? | Not selected; do not assume one schema, database, or process per tenant. |
| O-007 | What exactly can Starlark read and change? | Define events, typed inputs, actions, step/memory limits, permissions, failures, and versions. |
| O-008 | What authentication and authorization model is needed before hosted use? | Not implemented; database-backed builder endpoints are currently unauthenticated and intended for local development only. |
| O-009 | Is Athena the final product name? | Working name only; no naming clearance or final decision. |
| O-010 | When should the transitional JSON Record endpoints move to ConnectRPC? | Resolved 2026-11-17: Record Types use `athena.v1.RecordTypeService` (named `RecordService` at the time) and `/api/records` is removed. |

## Rejected or unverified assumptions

- A MariaDB schema definition is not the same as a persisted Record.
- Go source generated by the dashboard does not become active in a running executable without rebuilding/restarting it.
- A Pack grouping is not a database relationship until the FK or join-table mapping is materialized.
- ConnectRPC support does not automatically provide conventional REST semantics.
- The core ERP has no default invoice, ticketing, regional, or industry-specific business model.
- Claims of superiority to Frappe or another ERP need evidence and defined user workflows.
