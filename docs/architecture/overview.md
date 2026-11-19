# Architecture overview

**Last updated:** 2026-11-19
**Status:** Core-platform direction accepted; implementation has started with the pinned pnpm toolchain. Vocabulary follows ADR-0008 (Proposed).

## Product intent

Athena is the reusable core on which different ERP products and business packs can be built. It is not currently an ERP configured for one industry, country, or company. The core provides the runtime, data model, permissions, API, rules, and module boundaries; domain-specific Record Types and Packs are added on top.

## Runtime shape

```text
Browser
  └── one origin / one Go process (:3000 by default)
      ├── Vue 3 + TypeScript + Vite frontend, embedded at build time
      ├── ConnectRPC API and generated Protobuf clients
      ├── MariaDB metadata and per-Record-Type tables
      └── optional generated Go behavior compiled into the server
```

- `make run` builds the Vite application and starts Go on loopback at port `3000` against `ATHENA_DATABASE_URL` (development).
- `athena init` creates a home with the database server login; `athena new` creates a Workspace folder and its own database named from the slug; `athena run` serves one Workspace, checks the database's `athena_workspace` ID against `athena.yaml`, and holds a per-user lock so only one Workspace runs at a time. See [ADR-0004](../decisions/adr-0004-project-per-database.md).
- `make build` embeds the built frontend in `bin/athena`.
- ConnectRPC remains the selected Go RPC library and Protobuf remains the intended API contract. Record Types (`RecordTypeService`), Records (`RecordService`) and system info (`SystemService`) all use it; there is no separate JSON API.
- MariaDB is the only relational database being implemented now. PostgreSQL is a later adapter, not a current choice.
- AWS, Cloudflare, identity providers, queues, and object storage remain optional integrations around the core.

## Frontend direction

See [Frontend theme and typography](frontend-theme-and-type.md) for the centralized palette and type scale.

- One supported frontend: Vue 3 + TypeScript + Vite.
- pnpm 11.22.0 is the pinned package manager. `web/pnpm-lock.yaml` is the sole frontend lockfile; `make web-build` uses frozen installation before typecheck/build.
- Primary component system: shadcn-vue, using the existing project's Vite setup.
- The current `components.json` records the requested `aeaM05w` preset output: `reka-luma`, Zinc base tokens, Inter, `0.625rem` base radius, and pointer cursors. The exact command was applied to the existing `web/` app; no replacement frontend was created.
- Theme is class-based and token-driven. A visible toggle follows the operating-system preference on first use and stores an explicit choice in local storage.
- The **Build** desk contains Overview, Record Types, Packs, and Rules. **View** lists a saved Record Type's Records and creates, edits and deletes them.
- The core UI starts without industry-specific sample Record Types or Packs.
- App colors and light/dark semantic tokens live in `web/src/theme.css`; the supplied typography scale lives in `web/src/typography.css`. Buttons use shadcn-vue semantic primary, secondary, outline, ghost, and destructive variants so palette changes flow from the shared theme.

## Domain vocabulary and data model

- A **Record Type** names a data structure, its fields, validation, and behavior hooks. A Record Type maps to a dedicated MariaDB table. A saved **Record** is a row in that table. These terms follow ADR-0008 (Proposed), which supersedes D-005.
- A **Workspace** is a database plus its installed Packs, Records, users and overlays. A **home** is the directory that holds the database server login and the Workspace directories.
- A field has a stable code name, display label, type, and validation attributes. A Link field identifies another Record Type and represents a database relationship.
- New dashboard fields derive a lowercase `snake_case` code name from their label. Reserved names and collisions are disambiguated; the generated name is editable before that field saves and remains stable after a successful save. The editor preserves row identity while names update so typing and pasting do not lose focus.
- A **Pack** is an installable bundle of Record Types, explicit relationships, rules, actions, reports, permissions, and navigation. A **Module** is a functional area inside a Pack and is not implemented.
- Pack relationships must have concrete database meaning: foreign keys for direct links, and join tables for many-to-many relationships when supported. Relationship cardinality and delete/update rules need to be explicit.
- Record Type definitions must be usable from both the dashboard and the codebase. Dashboard-authored definitions are stored in MariaDB. Code-authored definitions live in the repository. Their merge, precedence, upgrade, and export rules remain to be designed.
- Dashboard-authored Record Types are intended to generate Go source scaffolds in the developer/codebase workflow. Saving a Record Type updates metadata and database schema; it does not compile or activate Go code in the running server. The generator, writable destination, and synchronization lifecycle are not implemented.

## Logic boundaries

- Go owns trusted lifecycle code, permission checks, invariants, persistence, and transaction boundaries.
- Starlark is the configurable rule language for conditions and bounded actions. It receives typed Record values and explicit Go-provided capabilities; it does not execute arbitrary SQL or bypass Go enforcement.
- A rule may reference Record Type and field identifiers, and Pack relationships. The rule editor/compiler must validate references against the current definitions.
- No Starlark runtime or Go hook generator is currently implemented.

## MariaDB persistence

- `ATHENA_DATABASE_URL` configures the Go MySQL driver DSN. When configured, Athena connects at startup and applies system migrations.
- `athena_record_types` stores Record Type definitions. Saving a Record Type provisions `p_workspace__<key>` (the reserved pack name `workspace`, at most 63 bytes, ADR-0009) with an `id`, a `revision`, timestamps, and columns mapped from its fields. Supported field types currently map to MariaDB types in Go.
- Link fields require a known target Record Type and create a foreign-key constraint (`fk_rtype_<hash>`) when its table exists. The relationship editor and Pack composition workflow are not implemented yet.
- Saving a Record Type goes through `RecordTypeService.SaveRecordType`: Go validates the definition, takes a per-database schema lock, checks the expected revision, journals the operation in `athena_record_type_operations`, applies the table changes, then activates the new revision with a compare-and-swap. Pending operations resume at startup.
- A saved Record Type can be deleted with `RecordTypeService.DeleteRecordType`, which drops its table and Records through the same journal and revision check. It is refused while other Record Types link to it, and needs `delete_records` when Records exist. The desk asks for confirmation first; an unsaved Record Type is just discarded. An interrupted delete resumes at startup.
- Each MariaDB DDL statement commits on its own, so the table steps are written to be safe to repeat. If a table change fails, the definition stays at its previous revision and the operation is marked failed with the step and error; tables or columns already created stay and are reused on the next save.
- `RecordTypeService.PlanRecordTypeSave` lists the migration a save would run: adding fields, removing fields (dropping their column and values), changing a field's type or link target (values that cannot convert are counted), and making a field required (empty Records need a fill-in value). `SaveRecordType` refuses a plan that deletes values unless `confirm_data_loss` is set, and a plan that leaves required fields empty unless `backfills` supplies a value. The answers are stored with the journaled operation; every step checks or tolerates current state, so resume is safe. Field and Record Type keys stay fixed after the first save; renaming a key is not supported. In Build, saving a saved Record Type first fetches the plan; if it deletes values or needs fill-in values, a review dialog collects the answers before `SaveRecordType` runs.
- `RecordService` (ConnectRPC) lists, gets, creates, updates and deletes Records. Go loads the saved definition, validates each value by field type, binds every value and takes table and column names only from that definition. Updates and deletes require the Record's current `revision`. Lists use keyset paging on the UUIDv7 `id`, capped at 200 rows, with a cursor bound to the Record Type.
- Each Record create, update and delete also writes a row to `athena_record_history` in the same transaction, listing the changed field keys and the actor (`Local administrator` until sign-in exists). `RecordService.ListRecordHistory` returns it newest first.
- System migration 5 (`vocabulary_rename`, ADR-0008 and ADR-0009) renames the tables, columns, indexes, foreign keys and stored keys of a database created before the rename, keeps all row data, and is safe to resume or repeat. It stops with an error naming both tables when an old and a new name exist together.
- Record kinds and system fields (ADR-0010, Proposed): a Record Type has a `kind` (master, document, child or setting), fixed after the first save. The kernel adds `created_by`, `modified_by`, `def_version` and `request_id` to every table, `owner` to masters and documents, `deleted_at` to masters, and `parent_id` and `idx` to children. Deleting a master Record soft-deletes it; a setting holds one Record; a child needs a live parent. Fields can declare `unique`, `default`, `min`, `max`, `pattern`, `max_length`, `read_only`, `set_once` and `index`, enforced in Go and mirrored in `web/src/fieldRules.ts`; `no_copy` is stored only. New types: Money (integer minor units plus fixed currency), Quantity (fixed unit) and Enum. Document lifecycle, numbering, ledger and the other kinds are not implemented. Every Record write opens a kernel span carrying only the operation, Record Type key, request ID and outcome code.
- Database sessions run in UTC so `TIMESTAMP` values read back correctly.
- OpenTelemetry is optional and off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set (`internal/telemetry`). The server traces ConnectRPC calls and MariaDB statements (statement text only, no parameter values) and exports metrics. The desk (`web/src/telemetry.ts`) starts tracing when `SystemInfo.telemetry_enabled` is true and posts spans to `/otlp/v1/traces`, which the server forwards to the collector; the server trusts the browser's `traceparent` while it is loopback-only. `make telemetry` runs `grafana/otel-lgtm` locally on 127.0.0.1.
- Packs, rules, multi-tenancy, audit history, authentication, and backups are not production-ready.

## Current implementation status

| Capability | Status |
| --- | --- |
| Single Go server and port 3000 | Implemented |
| Vue 3 + TypeScript + Vite | Implemented |
| Pinned pnpm install with frozen lockfile | Implemented; lockfile policy check enabled |
| shadcn-vue and requested preset | Initialized in existing app; core UI primitives generated |
| Light/dark theme toggle | Implemented in the desk |
| Record Type builder without seeded business examples | Implemented |
| MariaDB metadata connection | Implemented when `ATHENA_DATABASE_URL` is configured; live local service still needs verification |
| Per-Record-Type MariaDB table provisioning | Implemented on Record Type save; live DDL is not yet verified against the local service |
| Link-field foreign keys | Implemented for new/updated Record Types when the target table exists; relationship migration limits apply |
| Record CRUD | Implemented through `RecordService` and the View page, with optimistic revisions and per-Record change history; no authentication or filtering |
| Automated tests | Go tests for `RecordTypeService` and `RecordService` against throwaway MariaDB databases (skipped when `ATHENA_TEST_DATABASE_URL` is unset) and Vitest for `web/src/fieldRules.ts`, both driven by one shared rule table; run with `make test`. No end-to-end browser tests; PostgreSQL is untested |
| Pack composer and relation/join table management | Product direction accepted; implementation pending |
| Go Record Type source generation | Product direction accepted; implementation pending |
| Starlark rules and safe execution | Product direction accepted; implementation pending |
| Database dialect layer (`internal/store`) | Implemented for MariaDB only (proposed in ADR-0005): identifier quoting and hash shortening, type mapping, Record Type table DDL and rename statements, upsert, row lock, schema lock, catalogue lookups and error classification. Platform-table DDL now lives in versioned system migrations |
| System migrations (`internal/store/migrate.go`, `server.SystemMigrations`) | Implemented (ADR-0006, accepted; six migrations): numbered idempotent steps recorded in `athena_migrations`, applied at start-up and by `athena migrate status/up [--to N] [--dry-run]`; refuses a database newer than the build. Migration 5 applies the vocabulary rename; migration 6 gives existing Record Types the master kind and system columns. Record Type migrations, key renames, rollback and Pack upgrades are not implemented |
| Exact types (`internal/types`) | Implemented as a standalone Go library (ADR-0007, proposed): `Decimal` on `math/big` with `half_up` and `half_even` rounding, `Money` as `int64` minor units plus currency with rounding differences returned, `Quantity` with a UoM key, date and UTC datetime parsing, and deterministic `Allocate`. Not yet used by Record Type fields, Records or the API (P4); no CLI command or UI, since it has no user-facing operation |
| PostgreSQL adapter | Later; not implemented and untested. The dialect interface exists so one can be added |

See the [decision log](../decisions/decision-log.md) and [Record Type and Pack model](record-type-and-pack-model.md) for details and remaining design work.
