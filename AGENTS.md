# Athena ERP agent and contributor guide

This file is the short, current guide for anyone working in this repository. Keep it aligned with the canonical project docs; do not let a proposal silently become an accepted decision.

## Read first

1. [`docs/README.md`](docs/README.md) — documentation map and update rules.
2. [`docs/architecture/overview.md`](docs/architecture/overview.md) — current system shape and implementation status.
3. [`docs/decisions/decision-log.md`](docs/decisions/decision-log.md) — accepted decisions and open choices.

## Current direction

- **Working product name:** Athena. The public name is not finalized.
- **Backend:** Go owns the platform runtime and trusted core business logic.
- **Configurable logic:** Starlark is intended for administrator-configured conditions, macros, and bounded actions. It must not bypass Go permission checks, invariants, or transaction boundaries.
- **Frontend:** Vue 3 + TypeScript + Vite is the one supported frontend stack. Vite builds static files; it is not a production server/runtime dependency.
- **Frontend package manager:** Use the pinned pnpm version and committed `web/pnpm-lock.yaml`. Do not use npm or Yarn.
- **API:** ConnectRPC for typed RPC over the Go HTTP server, with Protobuf as the service contract and generated Go/TypeScript code.
- **Runtime shape:** One Go process serves the embedded frontend and API. The development entry point in this repository is `make run` (`athena serve` with `ATHENA_DATABASE_URL`); `make build` produces `bin/athena`. Installed use goes through `athena init`, `athena new` and `athena run`: a home holds the database server login, each Workspace has its own database named from its slug at creation (ADR-0004, amended by ADR-0008), and one Workspace runs per user. Use `--workspace`, not `--project`.
- **Product scope:** Athena is the reusable ERP core, not an ERP for one industry or company. Do not seed domain-specific Record Types or Packs into the platform shell.
- **Data:** MariaDB is the current database. A **Record Type** maps to its own MariaDB table, named `p_workspace__<key>` (ADR-0009, Proposed); a **Record** is one row. Saving a Record Type provisions the table and stores its metadata in `athena_record_types`. Record CRUD runs through the ConnectRPC `RecordService` (`proto/athena/v1/records.proto`) with optimistic revisions and keyset paging, without authentication. Platform tables use versioned system migrations (`internal/store/migrate.go`, `server.SystemMigrations`, ADR-0006); migration 5 (`vocabulary_rename`) applies ADR-0008. Record Type migrations, Pack relationships, and PostgreSQL are not implemented. Generated identifiers are lowercase, deterministic and at most 63 bytes (`store.GeneratedIdentLimit`).
- **Field naming:** New dashboard fields derive a lowercase `snake_case` database key from the label, avoid reserved names and same-Record collisions, and allow manual key adjustment until that field saves successfully. Preserve keys after save; renames require a migration design.
- **Vocabulary (ADR-0008, Proposed, supersedes D-005):** A **Record Type** is the definition that maps to a table; a **Record** is a row. A **Pack** is an installable bundle of Record Types and relationships; a **Module** is a functional area inside a Pack (not implemented). A **Workspace** is a database plus its installed Packs and data. A **home** is the directory that holds the server login and the Workspace directories. **Rule** means Starlark code only; permissions will be **Grants**. Do not use the old terms (Record entry, entry, project, Module as a bundle). The old names live only in `internal/server/legacy_names.go`, `sysmigrations.go`, `vocabulary_rename.go`, `internal/workspace/legacy.go`, their tests, Accepted ADR bodies and the D-005 row; `internal/server/terms_test.go` enforces it for source files. Web CSS class names, element ids and the `athena-entry-columns:` localStorage key also keep their old spelling on purpose, so styling and saved column widths survive; do not rename them.
- **Code authoring:** Record Types must be definable from the dashboard and the codebase. Dashboard-created Record Types are intended to generate Go type/hook source in the developer/codebase workflow. A Record Type save cannot compile that source into a running server; compiled hooks need a new build and deployment. The generator, writable destination, and synchronization policy are not implemented.
- **Frontend design:** Use shadcn-vue as the primary component system. The existing app uses the requested `aeaM05w` preset output; preserve its Reka Luma style, Zinc semantic tokens, the central palette in `web/src/theme.css`, typography scale in `web/src/typography.css`, pointer cursors, and class-based light/dark theme toggle. Keep component colors on semantic tokens rather than hard-coded values. Show action results (saved, deleted, conflicts, failures) as Sonner toasts in the bottom right, not inline banners.
- **Extensions:** Packs should be reusable/installable in another Athena Workspace. Packaging, dependency resolution, versioning, and upgrade migrations are still open design decisions.
- **Telemetry:** OpenTelemetry is optional and must stay off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Never put Record values, credentials or request bodies into spans or span attributes.
- **Cloud services:** AWS, Cloudflare, identity providers, queues, and object storage are optional future integrations. They are not core dependencies or selected deployment commitments.

## Keep documentation current

- Before changing a material product or architecture decision, read the docs above and mark the decision as **accepted**, **proposed**, **open**, **rejected**, or **superseded**. Never write an unresolved option as settled.
- When accepting or changing a material decision, update `docs/decisions/decision-log.md` and the relevant architecture/product doc in the same change. Add or update an ADR using `docs/decisions/adr-template.md` when the decision changes system boundaries, durable data, extension contracts, security, or deployment behavior.
- For implementation changes, update the affected architecture/status docs and README commands so they describe what the repository actually does. Separate implemented behavior from intended future behavior.
- Keep examples and setup commands reproducible from the repository root. If a command or build step changes, update the docs with it.
- Add new docs to `docs/README.md`. Prefer short, linked documents with dates/status over duplicating large design essays.
- Record new open questions with an owner/next decision point when known; do not invent a due date or owner.

## Implementation guardrails

- Keep the application frontend and ConnectRPC API on the same Go server/origin.
- The default listen address is `127.0.0.1:3000`; keep the README, CLI help, and Makefile aligned if it changes. Do not expose database-backed authoring endpoints beyond the local machine until authentication and authorization exist.
- The admin interface must retain separate Build and View modes. Build covers Overview, Record Types, Packs, and Rules. View shows only saved Record Types in its sidebar (no Build navigation), lists a Record Type's Records in a resizable TanStack/shadcn-vue data table, and creates, edits and deletes them. Record forms validate each field with `web/src/fieldRules.ts`, which mirrors the server rules; keep the two in step. Keep the desk neutral and token-driven.
- Keep generated Protobuf artifacts in sync with `proto/athena/v1/` when the schema changes; use `make generate` (requires Buf on `PATH`).
- Do not add database, authentication, Starlark execution, cloud provider, or Pack-install behavior and describe it as production-ready until it is implemented and its relevant docs are updated.
- MariaDB is the current database. Keep the DSN out of source control and say clearly that Record CRUD has no authentication or authorization yet.
- Keep the product's API on the ConnectRPC/Protobuf contract (`RecordTypeService`, `RecordService`, `SystemService`). `/api/records` has been removed; do not add parallel JSON endpoints.
- Record Type saves must go through the journaled publisher in `internal/server/record_types.go`. Destructive changes (removing a field, changing its type or link target, adding a required field to a table with Records) are planned by `PlanRecordTypeSave` and run only with `confirm_data_loss` and any needed backfill values. Record Type migrations, rollback and Pack upgrades are not implemented. Platform-table changes are new numbered entries in `server.SystemMigrations()`, never edits to applied ones; `athena migrate status|up` runs them.
- Preserve the current separation: Go enforces trusted rules and boundaries; Starlark supplies controlled configuration.
- Tests: `make test` runs Go and web tests. MariaDB tests use `ATHENA_TEST_DATABASE_URL` (defaults to `ATHENA_DATABASE_URL` from `.env.local`), each in its own `test_athena_*` database, and skip when unset. `internal/server/testdata/field_rules.json` is shared by the Go and Vitest field-rule tests; change a rule in `internal/server/records.go` and `web/src/fieldRules.ts` together with that table.
- Do not commit credentials, local cache directories, `node_modules`, or build output unless the project docs explicitly establish a reason.
