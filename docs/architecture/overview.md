# Architecture overview

**Last updated:** 2026-11-17
**Status:** Core-platform direction accepted; starter implementation is being reshaped around Records, MariaDB tables, and Go/Starlark extension points.

## Product intent

Athena is the reusable core on which different ERP products and business modules can be built. It is not currently an ERP configured for one industry, country, or company. The core provides the runtime, data model, permissions, API, rules, and module boundaries; domain-specific Records and Modules are added on top.

## Runtime shape

```text
Browser
  └── one origin / one Go process (:3000 by default)
      ├── Vue 3 + TypeScript + Vite frontend, embedded at build time
      ├── ConnectRPC API and generated Protobuf clients
      ├── MariaDB metadata and per-Record tables
      └── optional generated Go behavior compiled into the server
```

- `make run` builds the Vite application and starts Go at port `3000`.
- `make build` embeds the built frontend in `bin/athena`.
- ConnectRPC remains the selected Go RPC library and Protobuf remains the intended API contract. The initial metadata JSON endpoints are transitional and should be replaced before the API surface expands.
- MariaDB is the only relational database being implemented now. PostgreSQL is a later adapter, not a current choice.
- AWS, Cloudflare, identity providers, queues, and object storage remain optional integrations around the core.

## Frontend direction

- One supported frontend: Vue 3 + TypeScript + Vite.
- Primary component system: shadcn-vue, using the existing project's Vite setup.
- The current `components.json` records the requested `aeaM05w` preset output: `reka-luma`, Zinc base tokens, Inter, `0.625rem` base radius, and pointer cursors. The exact command was applied to the existing `web/` app; no replacement frontend was created.
- Theme is class-based and token-driven. A visible toggle follows the operating-system preference on first use and stores an explicit choice in local storage.
- The **Build** desk contains Overview, Records, Modules, and Rules. **View** previews how a selected Record entry will be presented.
- The core UI starts without industry-specific sample Records or Modules.

## Domain vocabulary and data model

- A **Record definition** names a data structure, its fields, validation, and behavior hooks. A Record definition maps to a dedicated MariaDB table. A saved **Record entry** is a row in that table.
- A field has a stable code name, display label, type, and validation attributes. A Link field identifies another Record and represents a database relationship.
- A **Module** groups related Record definitions and their explicit relationships, rules, actions, reports, permissions, and navigation into a reusable capability.
- Module relationships must have concrete database meaning: foreign keys for direct links, and join tables for many-to-many relationships when supported. Relationship cardinality and delete/update rules need to be explicit.
- Definitions must be usable from both the dashboard and the codebase. Dashboard-authored definitions are stored in MariaDB. Code-authored definitions live in the repository. Their merge, precedence, upgrade, and export rules remain to be designed.
- Creating a Record is intended to generate a Go source file with typed fields and hook points for conditions, actions, controls, and trusted behavior. This generator and its compile/reload workflow are not implemented yet. A running Go process does not compile new source at runtime; generated code must be built into a new executable before Go hooks change.

## Logic boundaries

- Go owns trusted lifecycle code, permission checks, invariants, persistence, and transaction boundaries.
- Starlark is the configurable rule language for conditions and bounded actions. It receives typed Record/Module values and explicit Go-provided capabilities; it does not execute arbitrary SQL or bypass Go enforcement.
- A rule may reference Record and field identifiers, and Module relationships. The rule editor/compiler must validate references against the current definitions.
- No Starlark runtime, Go hook generator, or general Record-entry CRUD is currently implemented.

## MariaDB persistence

- `ATHENA_DATABASE_URL` configures the Go MySQL driver DSN. When configured, Athena connects at startup and creates metadata tables.
- `athena_records` stores Record definitions. Saving a Record provisions `athena_record_<name>` with an `id`, timestamps, and columns mapped from its fields. Supported field types currently map to MariaDB types in Go.
- Link fields require a known target Record and create a foreign-key constraint when its table exists. The relationship editor and module grouping workflow are not implemented yet.
- Current table evolution is additive only. Renames, drops, type changes, data migration, and incompatible required-field changes need a versioned migration design before production use.
- Record-entry CRUD, modules, rules, multi-tenancy, audit history, authentication, and backups are not production-ready.

## Current implementation status

| Capability | Status |
| --- | --- |
| Single Go server and port 3000 | Implemented |
| Vue 3 + TypeScript + Vite | Implemented |
| shadcn-vue and requested preset | Initialized in existing app; core UI primitives generated |
| Light/dark theme toggle | Implemented in the desk |
| Record builder without seeded business examples | Implemented |
| MariaDB metadata connection | Implemented when `ATHENA_DATABASE_URL` is configured; live local service still needs verification |
| Per-Record MariaDB table provisioning | Implemented on Record-definition save; live DDL is not yet verified against the local service |
| Link-field foreign keys | Implemented for new/updated Record definitions when the target table exists; relationship migration limits apply |
| Record-entry CRUD | Not implemented |
| Module composer and relation/join table management | Product direction accepted; implementation pending |
| Go Record source generation | Product direction accepted; implementation pending |
| Starlark rules and safe execution | Product direction accepted; implementation pending |
| PostgreSQL adapter | Later; not implemented |

See the [decision log](../decisions/decision-log.md) and [Record and Module model](record-and-module-model.md) for details and remaining design work.
