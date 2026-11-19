# Decision log

This is the canonical status of product and architecture choices. Keep [`AGENTS.md`](../../AGENTS.md) aligned when accepted direction changes.

## Accepted decisions and direction

| ID | Decision | Status | Notes |
| --- | --- | --- | --- |
| ADR-0001 | One Go server serves the embedded frontend and API | Accepted | Vite builds static assets; Go serves them with ConnectRPC. Default address is port 3000. |
| ADR-0002 | MariaDB is the first relational database | Accepted | PostgreSQL is a later adapter. See [ADR-0002](adr-0002-mariadb-first.md). |
| ADR-0003 | A Record definition maps to one MariaDB table | Accepted | Go validates the schema and provisions the table. Record-entry CRUD is implemented through `EntryService`; versioned migrations cover platform tables only (ADR-0006); Record-table migrations remain incomplete. See [ADR-0003](adr-0003-record-table-model.md). |
| ADR-0004 | One database per project on the workspace database server | Accepted | `athena new` creates `<workspace>/<slug>` and database `<slug>`, fixed at creation; `athena run` checks the project ID and holds a per-user lock. See [ADR-0004](adr-0004-project-per-database.md). |
| ADR-0005 | Engine-specific SQL sits behind a `Dialect` interface in `internal/store` | Proposed 2026-11-19 | MariaDB is the only implementation and the only tested engine; no PostgreSQL code or support claim. See [ADR-0005](adr-0005-database-dialect-layer.md). |
| ADR-0006 | Platform tables are created by numbered, idempotent system migrations recorded in `athena_migrations` | Accepted 2026-11-19 | Applied at start-up and by `athena migrate status/up`. Record-definition migrations, rollback and Module upgrades are not covered (O-003). See [ADR-0006](adr-0006-versioned-system-migrations.md). |
| ADR-0007 | Exact types live in `internal/types` on `math/big`: decimals, money as `int64` minor units plus currency, quantities, UTC datetimes, deterministic allocation | Proposed 2026-11-19 | Pure Go library, not yet wired into fields or entries (P4). No new dependency. See [ADR-0007](adr-0007-exact-types.md). |
| D-011 | OpenTelemetry for traces and metrics, viewed locally in Grafana (`grafana/otel-lgtm`) | Accepted 2026-11-18 | Optional: off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Server and browser both trace; browser spans go through the Athena origin. Production collector and retention are not chosen. |
| D-002 | Vue 3 + TypeScript + Vite is the single frontend stack | Accepted | Do not introduce another frontend runtime. |
| D-003 | ConnectRPC is the Go RPC library and Protobuf is the API contract | Accepted | Records, entries and system info all use ConnectRPC. The JSON `/api/records` endpoint was removed on 2026-11-17. |
| D-004 | Go owns trusted behavior; Starlark supplies configurable rules | Accepted direction | Rules may reference Record fields and Module relationships through limited Go capabilities. Execution limits and lifecycle remain open. |
| D-005 | Use “Record” for table definitions and “Record entry” for rows | Accepted vocabulary | A Module bundles Records and relationships. |
| D-006 | Cloud provider integrations remain optional to the ERP core | Accepted direction | AWS, Cloudflare, Cognito, queues, and object storage are not core dependencies. |
| D-007 | Athena is an ERP platform core, not a preconfigured industry ERP | Accepted product scope | Start without industry-specific seed Records or Modules. Domain products can be installed later. |
| D-008 | shadcn-vue is the primary component system with a theme toggle | Accepted | Existing Vite app uses preset `aeaM05w`, resolved to `reka-luma`, Zinc tokens, Inter, token radius, pointer cursor, and class-based dark mode. |
| D-009 | Records can be authored in the dashboard and codebase; dashboard authoring generates Go source scaffolds | Accepted direction | Generated Go is source for the developer/codebase workflow and requires build/deploy before it affects the server. A Record save only updates metadata/schema. Generator destination, ownership, synchronization, and upgrades are not implemented; see [Record and Module model](../architecture/record-and-module-model.md). |
| D-010 | Keep the frontend palette and typography as shared semantic token sets | Accepted | Edit colors in `web/src/theme.css` and the supplied type scale in `web/src/typography.css`; action buttons use shadcn-vue semantic variants for readable text in both themes. |
| D-012 | Use pnpm as the only frontend package manager | Accepted | Shared desk pins pnpm 11.22.0, commits `web/pnpm-lock.yaml`, and installs with a frozen lockfile before typecheck/build. npm and Yarn are unsupported. Renumbered from a second D-011 on 2026-11-19. |
| D-013 | Tests: Go `testing` against throwaway MariaDB databases, Vitest for the desk | Accepted 2026-11-19 | `internal/testdb` creates a `test_athena_*` database per test from `ATHENA_TEST_DATABASE_URL` and skips when unset. Vitest is the only added frontend dev dependency; it shares `internal/server/testdata/field_rules.json` with the Go tests so client and server field rules stay in step. `make test` runs both. MariaDB is the only tested database. |

## Open decisions

| ID | Question | Current context |
| --- | --- | --- |
| O-001 | When should the PostgreSQL adapter be added? | MariaDB is the only active database target; wait for a concrete need and a dialect/migration design. |
| O-002 | How do dashboard and codebase Record definitions synchronize? | Decide ownership, precedence, conflict detection, generated-file boundaries, and deployment behavior. |
| O-003 | How are Record schema upgrades reviewed and applied? | A per-save planner handles add, remove, type-change and required-field backfills with confirmation (`PlanRecordSave`). System migrations are versioned (ADR-0006, accepted). Record-definition migrations, rename detection and rollback are still open. |
| O-004 | Which relationship cardinalities are supported first? | Link fields currently represent a direct foreign-key reference. Module composition and join tables are not implemented. Define delete/update rules. |
| O-005 | How are Modules versioned, installed, upgraded, and removed? | Modules must package Record definitions, relationships, rules, UI, dependencies, and migrations. |
| O-006 | What is the tenancy model? | Not selected; do not assume one schema, database, or process per tenant. |
| O-007 | What exactly can Starlark read and change? | Define events, typed inputs, actions, step/memory limits, permissions, failures, and versions. |
| O-008 | What authentication and authorization model is needed before hosted use? | Not implemented; database-backed builder endpoints are currently unauthenticated and intended for local development only. |
| O-009 | Is Athena the final product name? | Working name only; no naming clearance or final decision. |
| O-010 | When should the transitional JSON Record endpoints move to ConnectRPC? | Resolved 2026-11-17: Record definitions use `athena.v1.RecordService` and `/api/records` is removed. |

## Rejected or unverified assumptions

- A MariaDB schema definition is not the same as a persisted Record entry.
- Go source generated by the dashboard does not become active in a running executable without rebuilding/restarting it.
- A Module grouping is not a database relationship until the FK or join-table mapping is materialized.
- ConnectRPC support does not automatically provide conventional REST semantics.
- The core ERP has no default invoice, ticketing, regional, or industry-specific business model.
- Claims of superiority to Frappe or another ERP need evidence and defined user workflows.
