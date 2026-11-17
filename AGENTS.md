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
- **API:** ConnectRPC for typed RPC over the Go HTTP server, with Protobuf as the service contract and generated Go/TypeScript code.
- **Runtime shape:** One Go process serves the embedded frontend and API. The normal entry point is `make run`; `make build` produces `bin/athena`.
- **Data:** MariaDB is the first supported metadata store. Configure it with `ATHENA_DATABASE_URL`; when configured, Go verifies the connection and creates starter metadata tables. Form definitions can be loaded and saved. Record persistence, module/rule persistence, PostgreSQL support, and multi-tenancy are not implemented.
- **Vocabulary:** A **Form** defines a kind of business record; a **Record** is one instance. A **Module** is an installable business capability made from related Forms and supporting workflows, rules, reports, permissions, and navigation. A user-facing editor can be called a **Form view** to distinguish it from the definition.
- **Extensions:** Modules should be reusable/installable in another Athena project. Packaging, dependency resolution, versioning, and upgrade migrations are still open design decisions.
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
- The default listen address is `:3000`; keep the README, CLI help, and Makefile aligned if it changes.
- The admin interface must retain separate Build and View modes. Build covers Overview, Forms, Modules, and Rules; View previews the end-user record form. Keep the desk neutral and control-oriented rather than adding a decorative product palette.
- Keep generated Protobuf artifacts in sync with `proto/athena/v1/` when the schema changes; use `make generate` (requires Buf on `PATH`).
- Do not add database, authentication, Starlark execution, cloud provider, or module-install behavior and describe it as production-ready until it is implemented and its relevant docs are updated.
- MariaDB is the current metadata adapter. Keep the DSN out of source control and say clearly which metadata is actually persisted; sample records and the View form preview are not business-record persistence.
- Preserve the current separation: Go enforces trusted rules and boundaries; Starlark supplies controlled configuration.
- Do not commit credentials, local cache directories, `node_modules`, or build output unless the project docs explicitly establish a reason.
