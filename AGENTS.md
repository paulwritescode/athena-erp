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
- **Runtime shape:** One Go process serves the embedded frontend and API. The development entry point in this repository is `make run` (`athena serve` with `ATHENA_DATABASE_URL`); `make build` produces `bin/athena`. Installed use goes through `athena init`, `athena new` and `athena run`: one database per project, named from the project slug at creation (ADR-0004), and one running project per user.
- **Product scope:** Athena is the reusable ERP core, not an ERP for one industry or company. Do not seed domain-specific Records or Modules into the platform shell.
- **Data:** MariaDB is the current database. A **Record definition** maps to its own MariaDB table; a **Record entry** is one row. Saving a definition provisions the table and stores definition metadata. Record-entry CRUD runs through the ConnectRPC `EntryService` (`proto/athena/v1/entries.proto`) with optimistic revisions and keyset paging, without authentication. Platform tables use versioned system migrations (`internal/store/migrate.go`, `server.SystemMigrations`, ADR-0006); Record-definition migrations, module relationships, and PostgreSQL are not implemented.
- **Field naming:** New dashboard fields derive a lowercase `snake_case` database key from the label, avoid reserved names and same-Record collisions, and allow manual key adjustment until that field saves successfully. Preserve keys after save; renames require a migration design.
- **Vocabulary:** A **Record** means the definition that maps to a table; use **Record entry** for a row. A **Module** groups Records and explicit relationships into an installable capability.
- **Code authoring:** Records must be definable from the dashboard and the codebase. Dashboard-created Records are intended to generate Go type/hook source in the developer/codebase workflow. A Record save cannot compile that source into a running server; compiled hooks need a new build and deployment. The generator, writable destination, and synchronization policy are not implemented.
- **Frontend design:** Use shadcn-vue as the primary component system. The existing app uses the requested `aeaM05w` preset output; preserve its Reka Luma style, Zinc semantic tokens, the central palette in `web/src/theme.css`, typography scale in `web/src/typography.css`, pointer cursors, and class-based light/dark theme toggle. Keep component colors on semantic tokens rather than hard-coded values. Show action results (saved, deleted, conflicts, failures) as Sonner toasts in the bottom right, not inline banners.
- **Extensions:** Modules should be reusable/installable in another Athena project. Packaging, dependency resolution, versioning, and upgrade migrations are still open design decisions.
- **Telemetry:** OpenTelemetry is optional and must stay off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Never put entry values, credentials or request bodies into spans or span attributes.
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
- The admin interface must retain separate Build and View modes. Build covers Overview, Records, Modules, and Rules. View shows only saved Records in its sidebar (no Build navigation), lists a Record's entries in a resizable TanStack/shadcn-vue data table, and creates, edits and deletes them. Entry forms validate each field with `web/src/fieldRules.ts`, which mirrors the server rules; keep the two in step. Keep the desk neutral and token-driven.
- Keep generated Protobuf artifacts in sync with `proto/athena/v1/` when the schema changes; use `make generate` (requires Buf on `PATH`).
- Do not add database, authentication, Starlark execution, cloud provider, or module-install behavior and describe it as production-ready until it is implemented and its relevant docs are updated.
- MariaDB is the current database. Keep the DSN out of source control and say clearly that entry CRUD has no authentication or authorization yet.
- Keep the product's API on the ConnectRPC/Protobuf contract (`RecordService`, `EntryService`, `SystemService`). `/api/records` has been removed; do not add parallel JSON endpoints.
- Record saves must go through the journaled publisher in `internal/server/records.go`. Destructive changes (removing a field, changing its type or link target, adding a required field to a table with entries) are planned by `PlanRecordSave` and run only with `confirm_data_loss` and any needed backfill values. Record-definition migrations, rollback and module upgrades are not implemented. Platform-table changes are new numbered entries in `server.SystemMigrations()`, never edits to applied ones; `athena migrate status|up` runs them.
- Preserve the current separation: Go enforces trusted rules and boundaries; Starlark supplies controlled configuration.
- Tests: `make test` runs Go and web tests. MariaDB tests use `ATHENA_TEST_DATABASE_URL` (defaults to `ATHENA_DATABASE_URL` from `.env.local`), each in its own `test_athena_*` database, and skip when unset. `internal/server/testdata/field_rules.json` is shared by the Go and Vitest field-rule tests; change a rule in `internal/server/entries.go` and `web/src/fieldRules.ts` together with that table.
- Do not commit credentials, local cache directories, `node_modules`, or build output unless the project docs explicitly establish a reason.
