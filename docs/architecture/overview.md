# Architecture overview

**Last updated:** 2026-11-17
**Status:** Starter implementation with MariaDB metadata persistence and Build/View desk; domain data model remains in progress.

## Product intent

Athena is an extensible ERP platform inspired by Frappe's metadata-driven way of defining business records and building workflows. The goal is to let people shape business capabilities through the application while retaining a dependable Go core. Specific Frappe gaps to fix still need to be named from real user needs.

The core should work without a specific cloud provider. A future installer may guide operators through a small set of supported deployment choices and optional integrations, but its profiles and questions have not been designed yet.

## Current starter architecture

```text
Browser
  └── one origin / one Go process (default :3000)
      ├── Vue 3 + TypeScript app (Vite build, embedded static files)
      ├── ConnectRPC services (Protobuf contract, generated clients/handlers)
      ├── Form-definition JSON endpoints
      ├── MariaDB metadata store (optional configuration)
      └── health endpoint with database state
```

- `make run` builds the frontend and starts the Go server at `:3000` by default.
- `make build` builds the frontend and embeds it in `bin/athena`.
- `athena serve [--addr :8080]` runs the embedded app and API together.
- The frontend calls the API using the same origin.
- `GET /healthz` is a basic HTTP health endpoint. `athena.v1.SystemService/GetSystemInfo` is the initial ConnectRPC method.
- `ATHENA_DATABASE_URL` configures MariaDB using the Go MySQL driver's DSN syntax. When set, Athena connects at startup and creates the initial metadata tables.
- `GET /api/forms` reads persisted Form definitions and `PUT /api/forms` creates or updates one definition. This first metadata endpoint is JSON/HTTP; broader API shape and ConnectRPC coverage remain to be reconciled before expanding it.
- With no database DSN, the server still runs and reports `Not configured`; the UI uses sample Form definitions in browser memory.

## Admin interface

The desk is intentionally a neutral, control-oriented interface. **Build** contains Overview, Forms, Modules, and Rules. The Form builder exposes the definition fields and settings; Modules group Forms conceptually; Rules show the intended event/condition/action area. **View** renders the selected Form as an end-user entry screen. The View page is a preview, not business-record persistence.

The ConnectRPC API follows the official Protobuf code-generation workflow: the `.proto` contract produces Go server/client code and TypeScript descriptors. Generated files are committed so normal builds do not require Buf. `make generate` requires Buf on `PATH` and the web dependencies installed.

## Intended domain model

- A **Form** defines a kind of business record, such as Customer, Invoice, or Ticket. It can carry the metadata needed to describe fields and related behavior.
- A **Record** is one saved instance of a Form.
- A **Module** is an installable business capability that bundles related Forms and supporting workflows, configurable rules, reports, permissions, and navigation. Invoicing and Ticket Management are example modules.
- Modules should be reusable in another Athena project. Dependency declarations, compatibility rules, upgrades, and migrations remain undecided.
- A region-specific pack such as Kenya localization could extend a broader Invoicing module. This is a design direction, not an implemented extension system.

The interface may let an administrator configure Forms and rules. The distinction between Go-authored core definitions, UI-authored metadata, and Starlark-authored configuration still needs a concrete model.

## Business logic boundaries

- **Go** owns trusted core behavior: permissions, transaction boundaries, lifecycle guarantees, and business invariants.
- **Starlark** is intended for configurable conditions, macros, and constrained actions, for example requiring approval when an invoice exceeds a configured amount.
- Starlark must receive only explicit Go-provided capabilities. It must not execute arbitrary SQL or bypass authorization or core invariants.
- The initial project has no Starlark runtime or configurable rule engine yet.

## Persistence and deployment

- The primary store is intended to be relational and capable of serving multiple users from a hosted database.
- MariaDB is the first metadata database adapter. A hosted MariaDB instance is suitable for multi-user server deployments; local Homebrew MariaDB is the current development option.
- PostgreSQL is not implemented. The metadata DDL uses MariaDB JSON columns; SQL dialect and migration compatibility must be designed before adding another relational database.
- Current persistence covers Form definitions only. Modules and Rules are sample UI data; business Records, transactions, tenant boundaries, and audit history are not persisted.
- The single-process Go server is the simple deployment shape. Production persistence still requires an external database, configuration, backups, and operational guidance once implemented.
- AWS, Cloudflare, Cognito, RDS, DynamoDB, object storage, and queues are exploratory optional integrations. None is currently required or implemented.

## Current implementation status

| Capability | Status |
| --- | --- |
| Go command and HTTP server | Implemented |
| Embedded Vue 3 + TypeScript + Vite UI | Implemented; rebuilt by `make run` / `make build` |
| ConnectRPC + Protobuf system info service | Implemented |
| MariaDB connection and initial metadata tables | Implemented when `ATHENA_DATABASE_URL` is configured |
| Form-definition persistence | Implemented via `GET/PUT /api/forms` when MariaDB is connected |
| Business record persistence | Not implemented |
| Authentication and authorization | Not implemented |
| Form builder and View preview | Implemented as starter UI; edits are persisted only when MariaDB is connected |
| Installable Modules | Product concept and sample UI only |
| Starlark rules/macros | Intended direction; not implemented |
| Cloud integrations and installer profiles | Exploratory; not implemented |

## Next architecture decisions

See the [decision log](../decisions/decision-log.md). Near-term choices include the first relational database, the first business workflow, Form metadata and persistence, Starlark capabilities, and module versioning and installation.
