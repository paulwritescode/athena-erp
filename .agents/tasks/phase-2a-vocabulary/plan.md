# Phase P2a plan: vocabulary rename and storage naming

**Status:** PLAN ONLY (written 2026-11-19). No file outside `.agents/` has been edited by this step. Work happens in place on `main`; the 1 uncommitted change at planning time is `.agents/`; the implementer must not revert or discard anything and must not commit or push.
**Source of the decision:** roadmap section 8, answers A (rename to spec vocabulary, supersedes D-005) and B (agent decides storage shapes, recorded as a Proposed ADR).
**Spec:** `/Users/iampaulking/Downloads/ERP Kernel Specification Records, Ledgers, Operations, Reports.md` sections 22 (22.1 verdicts, 22.2 Pack/Workspace, 22.3 Record Type/Record, 22.4 Module, 22.5 Rule/Grant, 22.6 glossary, 22.7 earlier wording), 20.3 and 20.4 (identifier limits, table names), 23.2 (naming).
**Depends on:** P2 (ADR-0006, `server.SystemMigrations()` versions 1 to 4) and P3 (ADR-0007, `internal/types`, untouched by this phase).

## 0. Findings that shape the plan

1. **Name swap hazard.** The old words are reused by the new ones: today `Record` is the definition and `Entry` the row; after the rename `Record` is the row and `RecordType` the definition. `RecordService` (definitions) becomes `RecordTypeService`, and `EntryService` becomes `RecordService`. Files `records.proto`, `records.go`, `RecordsPage.vue` must be renamed before the files that take their names. Every rename step below is ordered for this.
2. **Migration 4 reads old names.** `addEntryRevisions` (sysmigrations.go) selects from `athena_records` and calls `recordTableName`. Versions 1 to 4 are permanent (ADR-0006), and a pre-versioning database still runs them before the new migration. After the rename `recordTableName` returns the new name, so migration 4 would silently skip every legacy table and leave it without `revision`. Migration 4 must use a frozen copy of the old naming function.
3. **The identity check runs before migrations.** `checkProject` (server.go, also called from `openMigrationDB`) reads `athena_project` before `store.Up`. After the rename the table is `athena_workspace`, which does not exist in an old database until the migration runs. The identity check needs a one-time read of the old table, confined to one legacy file.
4. **`athena_project` is created outside the migration system** (`internal/project/create.go`, `createDatabase`). New databases create `athena_workspace` directly; old ones are renamed by the migration.
5. **MariaDB identifier limit is 64 in code** (`store.MariaDB.MaxIdentLength`), the spec and roadmap require 63 so a name valid on MariaDB is valid on PostgreSQL. Generated names must use a 63 limit.
6. **`docs/architecture/implementation.yaml` uses "workspace" in both senses** (line 8 and 60 mean the spec Workspace; lines 51 to 52, 179 to 182 mean the ADR-0004 parent folder). Each occurrence must be classified, not bulk replaced.
7. **No `internal/project` name survives.** The Go package manages both the folder concept (home) and the Workspace; it becomes `internal/workspace`.

## 1. Mapping: old term to new term

| Old term (where it lives today) | New term | Spec section | Notes |
| --- | --- | --- | --- |
| **Record** = definition that maps to a table (D-005, ADR-0003, proto `Record`, `RecordService`, `athena_records`) | **Record Type** | 22.1, 22.3, 22.7 ("Record definition" read as Record Type) | Identifier stays lowercase snake_case, called "key" in the API. |
| **Record entry** = one row (proto `Entry`, `EntryService`, `athena_entry_history`) | **Record** | 22.3 | "Entry" is dropped as a noun. |
| **Module** = installable bundle (D-005, `athena_modules`, `ModulesPage`, Build nav "Modules") | **Pack** | 22.1, 22.2, 22.7 ("Module as installable read as Pack") | Nothing is implemented; only the placeholder page, table and docs change. |
| **Module** in its narrowed sense | **Module** (a functional area inside a Pack, not installable) | 22.4 | No code today; used only in docs. |
| **Project** = the database-backed place you work in (ADR-0004: `athena new`, `athena_project`, `--project`, `internal/project`, `server.Project`) | **Workspace** | 22.1, 22.2 | "A database plus installed packs, data, users and overlays". |
| **Project** = installable bundle (spec 19) | **Pack** | 22.2, 22.7 | Not present in this repository yet. |
| **workspace** = parent folder holding projects and the server login (ADR-0004: `athena init`, `athena-workspace.yaml`) | **home** (proposed, see section 5) | none; chosen here to avoid a clash with Workspace | Needs the user's answer. |
| **Rule** (Starlark program) | **Rule** (code only) | 22.5 | Unchanged. `athena_rules` stays; its `module_name` column becomes `pack_name`. |
| Permission rule (spec 21) | **Grant** | 22.5, 22.7 | Not in the repository yet. Later phases (P8) use the word; no rename now. |
| "Record definition" in docs | **Record Type** | 22.7 | |
| "Record" key fields `target_record_key` | `target_record_type_key` | 22.3 | |

Words that stay: Core, Overlay, Registry, Kind, Ledger, Field, Query, Report, Workflow, Role (22.1, 22.6).

## 2. Inventory of names that change

### 2.1 Persisted names (database)

| Old | New | Kind |
| --- | --- | --- |
| `athena_records` (cols `name, label, revision, definition, created_at, updated_at`) | `athena_record_types` (same columns) | table |
| `athena_record_<key>` (one table per Record) | `p_workspace__<key>` (reserved pack name `workspace`, roadmap P2a; shortened with the hash rule at 63) | table, one per saved Record Type |
| `athena_record_operations` (cols `record_name`, `idx_record_operations_status`) | `athena_record_type_operations` (col `record_type_name`, index `idx_record_type_operations_status`) | table, column, index |
| `athena_entry_history` (cols `record_name`, `entry_id`; index `idx_entry_history_entry`) | `athena_record_history` (cols `record_type_name`, `record_id`; index `idx_record_history_record`) | table, columns, index |
| `athena_modules` | `athena_packs` | table (empty today) |
| `athena_rules.module_name` default `'Core'` | `athena_rules.pack_name` default `'core'` | column |
| `athena_project` (`id`, `slug`) | `athena_workspace` (`id`, `slug`) | table, created by `internal/project/create.go` |
| `fk_record_<hash16>` on each Link column | `fk_rtype_<hash16>` (same hash input `<type>:<field>`) | foreign key name |
| JSON key `target_record` inside `athena_record_types.definition` and `athena_record_type_operations.definition` | `target_record_type` | metadata key (journal rows included) |
| Journal `action` values `save`/`delete`, history `action` values `create`/`update`/`delete`, `last_step` values | unchanged | not names of the old vocabulary |
| `athena_migrations` | unchanged | system table |

Entry data columns (`id`, `revision`, `created_at`, `updated_at`, field columns) are unchanged. Journal rows and history rows are kept and rewritten, never dropped.

### 2.2 Protobuf (`proto/athena/v1/`, `make generate` regenerates `gen/` and `web/src/gen/`)

| Old | New |
| --- | --- |
| `records.proto` (RecordService) | `record_types.proto` |
| `entries.proto` (EntryService) | `records.proto` |
| `RecordService` | `RecordTypeService` |
| `ListRecords`, `PlanRecordSave`, `SaveRecord`, `DeleteRecord` (+ Request/Response) | `ListRecordTypes`, `PlanRecordTypeSave`, `SaveRecordType`, `DeleteRecordType` |
| messages `Record`, `RecordField` | `RecordType`, `RecordTypeField` |
| `RecordField.target_record_key` | `RecordTypeField.target_record_type_key` |
| `SaveRecordRequest.record`, `SaveRecordResponse.record`, `PlanRecordSaveRequest.record` | field `record_type` |
| `DeleteRecordRequest.delete_entries` | `delete_records` |
| `MigrationStep`, `confirm_data_loss`, `expected_revision` | unchanged |
| `EntryService` | `RecordService` |
| `ListEntries`, `GetEntry`, `CreateEntry`, `UpdateEntry`, `DeleteEntry`, `ListEntryHistory` (+ Request/Response) | `ListRecords`, `GetRecord`, `CreateRecord`, `UpdateRecord`, `DeleteRecord`, `ListRecordHistory` |
| messages `Entry`, `EntryEvent` | `Record`, `RecordEvent` |
| request fields `record_key` (in list/get/create/update/delete/history) | `record_type_key` |
| `ListEntriesResponse.entries` | `records` |
| `GetSystemInfoResponse.project_name`, `project_slug` | `workspace_name`, `workspace_slug` (field numbers 3 and 4 kept; nothing is released) |
| package `athena.v1` | unchanged |
| `FieldValue` | unchanged |

Do not add `GetRecordType` (spec 22.3 mentions it as an example of distinct calls; nothing needs it today).

### 2.3 Go

| Old | New |
| --- | --- |
| `internal/project/` package, `project.Project`, `ProjectDatabase`, `ProjectID`, `Create`, `Acquire`, `Active.Project` | `internal/workspace/`, `workspace.Workspace` (identity + manifest), `WorkspaceDatabase`, `WorkspaceID`, `Create`, `Acquire`, `Active.Workspace` |
| `project.Workspace`, `WorkspaceFile`, `InitWorkspace`, `FindWorkspace`, `LoadWorkspace`, `WorkspaceDatabase` (the folder concept) | `workspace.Home`, `HomeFile`, `InitHome`, `FindHome`, `LoadHome`, `HomeDatabase` (if the answer in section 5 is yes) |
| `server.Project`, `Config.Project`, `checkProject`, `openMigrationDB(..., project)` | `server.Workspace`, `Config.Workspace`, `checkWorkspace` |
| `internal/server/records.go`, `recordService`, `recordDefinition`, `recordField`, `recordTableName`, `recordForeignKeyName`, `recordKeyPattern`, `recordFromProto`, `recordToProto`, `SaveRecord`, `PlanRecordSave` | `record_types.go`, `recordTypeService`, `recordTypeDefinition`, `recordTypeField`, `recordTypeTableName`, `recordTypeForeignKeyName`, `recordTypeKeyPattern`, `recordTypeFromProto`, `recordTypeToProto`, `SaveRecordType`, `PlanRecordTypeSave` |
| `internal/server/entries.go`, `entryService`, `loadEntry`, `scanEntry`, `entryEvent` code | `records.go`, `recordService`, `loadRecord`, `scanRecord` |
| `internal/server/migrations.go` (per-save planner, `PlanRecordSave`) | file name kept; symbols renamed inside |
| `internal/server/testdata/field_rules.json` | file name kept; check content for old terms |
| tests `records_test.go`, `entries_test.go`, helpers (`newServices`, `saveRecord`) | `record_types_test.go`, `records_test.go`, `saveRecordType` etc. |
| `internal/store`: `CreateRecordTable`, `AddRevisionColumn` comments, `ForeignKeyTarget` | `CreateRecordTypeTable`; add `RenameTable`, `RenameColumn`, `RenameIndex`, `ColumnExists`, `IndexExists` (needed by the migration) |
| Telemetry attributes `athena.project.slug`, `athena.project.id` | `athena.workspace.slug`, `athena.workspace.id` |
| Messages and errors that say "Record", "entry", "project" | reviewed one by one (they reach the user as ConnectError text and toasts) |
| `store.MariaDB.ShortenIdent` limit 64 | generated identifiers use limit 63 (`store.GeneratedIdentLimit = 63`); `MaxIdentLength()` stays 64 as the engine fact |

### 2.4 CLI (`cmd/athena/`)

| Old | New |
| --- | --- |
| `athena init [directory]` "Create a workspace" | `athena init [directory]` "Create a home" (answer-dependent) |
| `athena new [name]` "Create a project in the current workspace" | `athena new [name]` "Create a Workspace in the current home" |
| `athena run [--project slug]` | `athena run [--workspace slug]` |
| `athena migrate status/up [--project slug]` | `[--workspace slug]` |
| `printUsage` text, messages "Workspace ready", "Create a project with", `resolveProject`, `migrationTarget`, flag descriptions | reworded; no `--project` alias is kept, so `--project` fails with Go's "flag provided but not defined" |
| `athena serve` | unchanged name; help says "without a Workspace" |
| files: `cmd/athena/main.go`, `migrate.go`, `migrate_test.go` | edited in place |

### 2.5 Web (`web/src/`)

| Old | New |
| --- | --- |
| `types.ts` `RecordDefinition` | `RecordType` |
| `api.ts` `listRecords`, `planRecordSave`, `saveRecord`, `deleteRecord`, `recordClient`, `entryClient` | `listRecordTypes`, `planRecordTypeSave`, `saveRecordType`, `deleteRecordType`, `recordTypeClient`, `recordClient` (rename the first, then the second) |
| `systemClient` fields `projectName`, `projectSlug` | `workspaceName`, `workspaceSlug` |
| `pages/RecordsPage.vue` | `pages/RecordTypesPage.vue` (rename first) |
| `pages/EntriesPage.vue` | `pages/RecordsPage.vue` |
| `pages/ModulesPage.vue` | `pages/PacksPage.vue` |
| `components/EntriesTable.vue` | `components/RecordsTable.vue` (prop `record-key` becomes `record-type-key`) |
| `components/MigrationDialog.vue`, `fieldRules.ts`, `fieldRules.test.ts`, `RulesPage.vue`, `OverviewPage.vue`, `App.vue`, `telemetry.ts` | edited in place |
| `Page` ids `'records'`, `'modules'` | `'record-types'`, `'packs'` |
| URL query `record=<key>`, `entry=<id>` | `record_type=<key>`, `record=<id>`; `page=record-types|packs`; `mode=build|view` unchanged |
| Build nav: Overview, Records, Modules, Rules | Overview, Record Types, Packs, Rules |
| View sidebar heading "Records", empty text "No saved Records." | "Record Types", "No saved Record Types." |
| Labels: "Untitled Record", "New Record", "Save the Record first", "Define Records, group them into Modules and add Rules.", Modules page copy, toasts "Could not save this Record", "Could not delete this Record", "Entries" button, "Entries could not be loaded" | Record Type / Pack / Record wording, each string reviewed in context |
| `aria-label="Workspace mode"` on the Build/View switch | `Desk mode` (it is not about a Workspace) |
| Overview `<h1>Workspace</h1>` | stays; it is now correct (the page shows the Workspace) |
| Brand `projectName` | `workspaceName` |

Component colours, tokens and Sonner placement are untouched.

### 2.6 Docs and rules

| File | Change |
| --- | --- |
| `AGENTS.md` | Vocabulary, Data, Runtime shape, Field naming, Frontend (Build pages), Implementation guardrails (service names, `internal/server/records.go` path becomes `record_types.go`), proto names, home term. Done in the same change as the code. |
| `README.md` | "Workspaces and projects" section, commands with `--workspace`, repository layout (`internal/workspace/`), entry wording. |
| `docs/README.md` | Rename the link for the renamed model doc; ADR-0008 and ADR-0009 entries. |
| `docs/architecture/overview.md` | Status table, sections about Records, entries, project identity. |
| `docs/architecture/record-and-module-model.md` | Rename file to `record-type-and-pack-model.md` (`git mv`), rewrite terms, fix every link (grep for the old file name). |
| `docs/architecture/implementation.yaml` | Classify each "workspace", "project", "Record", "Module", "entry" occurrence (finding 6). |
| `docs/architecture/frontend-theme-and-type.md` | Check for old terms only. |
| `docs/decisions/decision-log.md` | D-005 row set to Superseded by ADR-0008; ADR-0003 and ADR-0004 rows annotated "Accepted, amended by ADR-0008 (Proposed)"; rows for ADR-0008, ADR-0009; O-002/O-005/O-006 wording uses the new terms. |
| `docs/decisions/adr-0003`, `adr-0004` | Banner "Amended by ADR-0008" plus an Amendments section; bodies keep period wording (Accepted ADRs are not rewritten). |
| `docs/decisions/adr-0001`, `0002`, `0006` | Accepted: banner line only if they use the old terms; no body edits. |
| `docs/decisions/adr-0005`, `adr-0007` | Proposed: edit terms in place. |
| `internal/project/templates.go` (new `internal/workspace/templates.go`) | Generated `AGENTS.md`, README, `.gitignore` text for new Workspaces uses new terms. Existing generated files in users' Workspaces are not rewritten (they may be edited). |
| `internal/server/testdata`, `Makefile`, `.env.example`, `scripts/stop-athena.sh` | check for old terms; `.env.example` and the Makefile have none today. |

## 3. Rename migration design (P2-style)

### 3.1 Shape

Add **one** numbered entry in `server.SystemMigrations()` (`internal/server/sysmigrations.go`): **version 5, name `vocabulary_rename`**, appended after version 4. Versions 1 to 4 text is not edited. The runner (`store.Up`) already takes the schema lock, records version 5 after success, and re-runs the whole step after a crash, so the step is a sequence of existence-guarded sub-steps (ADR-0006).

New dialect methods in `internal/store` (MariaDB implementation only; PostgreSQL untested): `RenameTable(from, to)`, `RenameColumn(table, from, to)`, `RenameIndex(table, from, to)`, `ColumnExists`, `IndexExists`. All names pass through `QuoteIdent`. Table existence uses the existing `TableExists`.

Guard rule for every rename: `from` exists and `to` does not, rename; `to` exists and `from` does not, skip; **both exist, stop with an explicit error naming both** (never merge, never drop); neither exists, skip.

### 3.2 Sub-steps, in order

1. **Rename system tables:** `athena_records` to `athena_record_types`, `athena_record_operations` to `athena_record_type_operations`, `athena_entry_history` to `athena_record_history`, `athena_modules` to `athena_packs`, `athena_project` to `athena_workspace` (skip when absent: a `serve` development database has none).
2. **Rename columns and indexes:** `record_name` to `record_type_name` in the operations and history tables; `entry_id` to `record_id`; `athena_rules.module_name` to `pack_name` with default `'core'`; indexes `idx_record_operations_status`, `idx_entry_history_entry` to their new names. Index columns follow automatically.
3. **Rewrite metadata JSON:** read every row of `athena_record_types` and of `athena_record_type_operations` (all statuses, including `pending`), decode `definition` as a generic map, rename each field's `target_record` to `target_record_type`, write back only rows that changed. Reading tolerates both keys, so a second run changes nothing. Done in Go, not with `JSON_REPLACE`, so a field list of any size works.
4. **Rename Record tables:** for each name in `athena_record_types`, compute the old table with the frozen legacy function (lowercase, `athena_record_` prefix, shorten at **64**) and the new table with `p_workspace__<lowercase key>` shortened at **63**. Apply the guard rule. A legacy key that differs from another only by case produces the same lowercase name already, so the both-exist rule surfaces it as an error before any change is made: run a pre-pass that computes all pairs and checks for duplicates and conflicts before renaming anything.
5. **Swap foreign key names:** for each Link field, if the old `fk_record_<hash>` exists, drop it and add `fk_rtype_<hash>` against the new target table; if neither exists and the target table exists, add the new one (recovery after a crash between drop and add). MariaDB updates FK targets on `RENAME TABLE` but keeps constraint names, which is why names are rewritten here. Link columns hold valid references, so the add succeeds.

Order matters: 3 before 4 (4 reads the `definition` key set), and metadata tables (1) before anything that reads them.

### 3.3 Legacy names live in one file

Create `internal/server/legacy_names.go` holding every pre-rename name the migration needs: the frozen `legacyRecordTableName` (used by migration 4 **and** step 4), the old table, column and index names, `legacyForeignKeyName`, and `legacyWorkspaceIdentity` (reads `athena_project` when `athena_workspace` is absent). It is the only non-test file allowed to contain the old names. Migration 4 is edited to call `legacyRecordTableName` instead of `recordTableName` (this keeps its behaviour byte for byte; it is not a change of an applied migration's effect).

### 3.4 Identity check across the rename

`checkWorkspace` reads `athena_workspace`. When that table does not exist it calls `legacyWorkspaceIdentity`, which reads `athena_project`, so the "is this the right database" check still runs before migrations touch an old database. Once migration 5 has run, only `athena_workspace` is read.

### 3.5 Files outside the database

- The workspace identity row is unchanged (same id and slug), so `athena.yaml` needs no new keys and `SchemaFormatVersion` stays 1.
- If section 5 is accepted, `athena-workspace.yaml` becomes `athena-home.yaml`. `FindHome` upgrades an old marker once: if `athena-workspace.yaml` exists and `athena-home.yaml` does not, it renames the file (content keys are unchanged) and tells the user. The legacy filename constant lives in `internal/workspace/legacy.go` beside `UpgradeLegacyHome`, and nothing else reads it. Existing generated project files (`AGENTS.md`, README) keep their text.
- `Acquire`'s lock message and `Active` struct use "Workspace".

### 3.6 Old names after the rename

After migration 5, no table, column, index, FK, JSON key, proto name, flag, label or doc uses an old term except: `legacy_names.go`, `legacy.go`, migrations 1 to 4 (permanent), their tests, Accepted ADR bodies, and the decision log's superseded D-005 row. Section 6 includes a test that enforces this list.

## 4. ADR draft plan (both Proposed)

Numbers follow the next free ones in `docs/decisions/` (0008, 0009); the roadmap's earlier "ADR-0008" for auth (P8) and later ADR mentions become the next free number when those phases run, and the roadmap note says so. Use `docs/decisions/adr-template.md`; date 2026-11-19 or the day written; decision owner "project owner".

### ADR-0008: Vocabulary: Record Type, Record, Pack, Workspace, Grant (Proposed)

- **Supersedes** D-005 (decision log row set to "Superseded by ADR-0008 once accepted"; marked Proposed meanwhile). **Amends** ADR-0003 (title and body read "Record Type maps to a table"; Record entry becomes Record; the table name becomes `p_workspace__<key>`) and ADR-0004 (project becomes Workspace; the parent folder becomes home if section 5 is accepted; `--project` becomes `--workspace`; `athena_project` becomes `athena_workspace`).
- Context: spec 22; the clash between "Record" as definition and as row; the clash of "project" with business Projects (22.2); D-005 and ADR-0004 usage.
- Decision: the mapping table of section 1, the persisted renames of 2.1 by reference to ADR-0009 and the migration, the proto/CLI/web renames, the rule that Rule means code only and permissions are Grants, the reserved pack name `workspace`, and the list of allowed historical mentions.
- Options: keep old words (rejected by the user, answer A); rename only the UI (rejected: names become table, proto and permission keys, spec 22 intro); rename once now (chosen).
- Consequences: breaking proto change accepted because nothing is released; every later phase reads old words through the mapping; AGENTS.md changes in the same change as the code.
- Not decided: pack packaging (P15), Grant model (P8), final product name (O-009).

### ADR-0009: Storage shapes: money, ids, table and identifier naming (Proposed)

- Money is stored as integer minor units (`BIGINT`) plus a `CHAR(3)` currency code (spec 20.2, ADR-0007). User `Number` fields stay `DECIMAL` decimal strings until P4.
- Ids are UUIDv7 in `CHAR(36)` (time-ordered, portable to PostgreSQL `UUID`, no auto-increment gaps). ULID and bigint are rejected (spec lists them as options; the repository already uses UUIDv7 and the user approved answer B).
- Table names: `p_<pack>__<record_type>`, child tables `p_<pack>__<record_type>__<child_field>`, ledgers `l_<ledger>`, system tables `athena_<name>`. Record Types authored before any Pack exists live under the reserved pack `workspace`. Identifiers are ASCII, lowercase snake_case, permanent (spec 23.2).
- Limit 63 bytes for every generated identifier on every engine, with the deterministic hash shortening of `store.ShortenIdentifier` (prefix + `_` + 12 hex characters of SHA-256). Index and FK names use the same rule. `MaxIdentLength()` stays the engine fact (64 on MariaDB).
- A new Record Type key is at most 50 characters (`recordTypeKeyPattern`), so `p_workspace__` (13) plus the key is at most 63 and needs no shortening; child tables and legacy keys may.
- Case and collation: keys are stored lowercase; no code relies on a case-insensitive collation.
- Consequences: P4 `def_version` and system columns land on the renamed tables; P11 ledger tables use `l_`; PostgreSQL stays untested.

## 5. NEEDS YOUR ANSWER (ANSWERED: YES, see Answer log)

**Question: rename the ADR-0004 parent-folder concept "workspace" to "home"? (yes/no)**

Why it is needed: spec 22.2 gives the word Workspace to the thing ADR-0004 calls a project (a database plus installed packs, data and users). ADR-0004 already uses "workspace" for something else: the directory that holds `athena-workspace.yaml`, the database server login in `.env.local`, and the sibling project directories. After the rename both would be called Workspace, and `athena new` would "create a Workspace in the current workspace". 22.1 and the roadmap require that no word means two things.

Why "home": it appears nowhere in the spec glossary or this repository (checked in the spec and by search), it reads naturally ("Athena home holds your Workspaces and the server login"), and it is short in CLI help.

If **yes**: the parent folder is a **home**; the marker file is `athena-home.yaml`; Go `Home`, `HomeFile`, `InitHome`, `FindHome`, `LoadHome`; help text "Create a home". An old `athena-workspace.yaml` is renamed once on first use (3.5).
If **no**: you must name another word for the folder (for example "base" or "root") because Workspace cannot mean both. Say which.

Files affected by the folder rename:
- `internal/project/project.go` (`Workspace`, `WorkspaceDatabase`, `WorkspaceFile`, `LoadWorkspace`, `FindWorkspace`, doc comment), `internal/project/create.go` (`InitWorkspace` and header text), `internal/project/templates.go`, `internal/project/lock.go` if it mentions the folder
- `cmd/athena/main.go` (init and new help text, messages, variables), `cmd/athena/migrate.go`, `cmd/athena/migrate_test.go`
- `README.md` ("Workspaces and projects" section, layout line), `docs/README.md`, `docs/architecture/overview.md` line 22, `docs/architecture/implementation.yaml` (lines 51 to 52, 179 to 182 for the folder meaning)
- `docs/decisions/adr-0004-project-per-database.md` (amendment), `docs/decisions/decision-log.md`
- `web/src/App.vue` (aria label), `web/src/pages/OverviewPage.vue` only for the Workspace heading check

Secondary default (no answer needed unless you object): the reserved pack for Record Types authored before any Pack exists is named `workspace` (roadmap P2a), so tables read `p_workspace__<key>`. It means "owned by the Workspace" (spec 23.2 uses "pack or workspace" for custom field ownership).

## 6. Ordered implementation steps and verification

Each step leaves `go build ./...` working unless noted. The coder runs the step's verification before moving on. Never commit or push; note changed files in `.agents/tasks/phase-2a-vocabulary/notes.md`.

- [ ] 1. **Migration first.** Add the dialect methods (`store.go`, `mariadb.go`, unit tests in `store_test.go` and `mariadb_db_test.go`), add `legacy_names.go`, point migration 4 at `legacyRecordTableName`, add migration 5 `vocabulary_rename` per section 3, add `checkWorkspace` fallback. `go build ./...` still passes after this step, but the existing server tests (which run every migration, then use the old names) fail until step 3 renames the Go code. Steps 1 to 3 are one unit; the full suite is green only after step 3. The migration tests run against a database built with `store.Up(Target: 4)` and do not depend on it.
  Files: `internal/store/store.go`, `internal/store/mariadb.go`, `internal/server/sysmigrations.go`, `internal/server/legacy_names.go`, `internal/server/server.go`, tests in `internal/server/sysmigrations_test.go`.
  Verify: `go test ./internal/store ./internal/server -run 'Migration|Rename|Legacy|Identity'`.
- [ ] 2. **Proto edit.** `git mv proto/athena/v1/records.proto proto/athena/v1/record_types.proto`, then `git mv proto/athena/v1/entries.proto proto/athena/v1/records.proto`, then rewrite names per 2.2. Delete stale generated files (`gen/athena/v1/entries.pb.go`, `gen/athena/v1/athenav1connect/entries.connect.go`, `web/src/gen/athena/v1/entries_pb.ts`) after generation.
  Files: `proto/athena/v1/*.proto`. Verify: `make generate` succeeds, a second `make generate` leaves no diff (`git status --short gen web/src/gen` unchanged between runs).
- [ ] 3. **Go.** Rename `internal/project` to `internal/workspace` (`git mv`), then `records.go` to `record_types.go`, then `entries.go` to `records.go` (that order), and apply 2.3: types, functions, table-name function (`p_workspace__`, limit 63), FK name prefix, error messages, telemetry attributes, dialect `CreateRecordTypeTable`, `store.GeneratedIdentLimit`, `internal/server/testdata` check. Rename tests alongside.
  Verify: `go build ./...`, `go vet ./...`, `go test ./...` pass with `ATHENA_TEST_DATABASE_URL` set.
- [ ] 4. **Web.** Rename files in the order `RecordsPage.vue` to `RecordTypesPage.vue`, then `EntriesPage.vue` to `RecordsPage.vue`; apply 2.5 including labels, query parameters, aria label. Keep tokens and components.
  Verify: `CI=true pnpm --dir web run typecheck`, `CI=true pnpm --dir web run test`, `CI=true pnpm --dir web run build`.
- [ ] 5. **CLI help and flags.** Apply 2.4 in `cmd/athena/main.go`, `migrate.go`, `migrate_test.go`, and the home upgrade in `internal/workspace`.
  Verify: `go test ./cmd/athena ./internal/workspace`; `go run ./cmd/athena help` shows only new terms; `go run ./cmd/athena migrate status --project x` fails with an unknown-flag error.
- [ ] 6. **Docs.** ADR-0008 and ADR-0009 (Proposed), decision log, amendments to ADR-0003 and ADR-0004, AGENTS.md, README, overview, model doc rename and links, `implementation.yaml`, `docs/README.md`.
  Verify: `git diff --stat` reviewed against section 2.6; every link in `docs/` resolves (open each renamed target); the term test (below) passes.
- [ ] 7. **Full verification and browser pass** (list below).

### Verification list

1. `go build ./...`, `go vet ./...`, `make test` (Go and web) pass; `make generate` twice leaves no further diff.
2. **Pre-rename database opens and keeps its data** (`sysmigrations_test.go`): build a database with `store.Up(Target: 4)` using the old names, then create by SQL: two Record tables with rows, one Link FK named `fk_record_<hash>`, a legacy `athena_project` row, a `pending` and a `completed` journal row whose `definition` has `target_record`, history rows. Run `store.Up`. Assert: new tables exist and old ones do not; row counts, values and `revision` of every data row equal the originals; FK `fk_rtype_<hash>` exists and targets the new table; JSON keys rewritten in both tables; history rows readable through `RecordService.ListRecordHistory`; `checkWorkspace` passes with the same id; `resumeOperations` completes the pending row against the new names.
3. **Idempotent and resumable:** second `Up` applies nothing and the schema signature is identical; an `Execer` wrapper that fails after the k-th statement, for every k in migration 5, followed by a clean `Up`, ends with the same signature and data.
4. **Fresh equals upgraded:** the existing `TestFreshAndPreVersionedDatabasesReachTheSameSchema` is updated so the fresh database and the legacy one converge to the same final schema (names of every table, column, index and FK).
5. **Migration 4 still works on legacy data:** a database at version 3 with a table lacking `revision` ends with `revision` on the renamed table (guards finding 2).
6. **Conflict handling:** both old and new table present returns an error naming both and changes nothing; two legacy keys differing only by case are refused before any rename.
7. **Identifier invariant tests:** `recordTypeTableName` is deterministic, lowercase ASCII, at most 63 bytes, for a 50-character key (no shortening) and a 63-character legacy key (hash suffix); FK and index names at most 63.
8. **Term guard test** (`internal/server/terms_test.go`): scans non-test Go sources and `web/src`, proto and `.vue` files for the old identifiers (`athena_record_`, `athena_entry`, `athena_project`, `EntryService`, `target_record`, `--project`, `entries.proto`) and fails outside the allowed list from section 3.6. It supplements the behavioural tests above; it does not replace them.
9. **Wire and CLI:** `RecordTypeService` and `RecordService` save, list, create, update (stale revision rejected), delete and history tests (the renamed P0 characterization tests) pass unchanged in behaviour; `athena new` and `athena run --workspace` work against a throwaway database; the home upgrade renames an old `athena-workspace.yaml` once.
10. **Telemetry:** the span-attribute test still passes; resource attributes are `athena.workspace.*`; no entry value appears in a span.
11. **Browser (UI phase):** `make restart` against local MariaDB; the desk shows the database connected; Build shows Overview, Record Types, Packs, Rules; View lists saved Record Types and their Records; create, edit, delete a Record; a conflict toast; save a Record Type with a Link and a destructive change dialog; both themes; confirm `internal/webui/dist` was rebuilt. Use the user's real pre-rename development database as the migration check if one exists (back it up first with `mysqldump`; the plan never touches it without that backup and the user's go-ahead).

## Digest

Verified by reading (not by running): build and test commands from `Makefile` (`make generate`, `make test`, `go build ./...`, `CI=true pnpm --dir web run typecheck|test|build`); `ATHENA_TEST_DATABASE_URL` defaults to `ATHENA_DATABASE_URL` in `.env.local`; tests exist today in `internal/server`, `internal/store`, `cmd/athena`, `internal/types`, `web/src/fieldRules.test.ts`. No test suite was run by this planning step.

| File | Block to change | Covering test |
| --- | --- | --- |
| `internal/server/sysmigrations.go` | `SystemMigrations()` (append version 5), `addEntryRevisions` (use frozen legacy name function), `execAll` helper unchanged | `sysmigrations_test.go`: `TestFreshAndPreVersionedDatabasesReachTheSameSchema`, `TestSystemMigrationsAreValid` (lines 79 and 123), plus new tests in 6.2 to 6.6 |
| `internal/server/legacy_names.go` (new) | `legacyRecordTableName`, old names, `legacyWorkspaceIdentity` | migration tests |
| `internal/server/server.go` | `Config.Project`, `Project` struct, `Serve` attrs and `checkProject` call, `systemService.GetSystemInfo`, `checkProject` (lines about 37 to 60, 100 to 125, 176 to 195), handler registration `RegisterRecordServiceHandler`/`RegisterEntryServiceHandler` (lines 118 to 119) | `server` tests, `cmd/athena/migrate_test.go` |
| `internal/server/records.go` to `record_types.go` | `recordDefinition`, `recordField` JSON `target_record`, `recordKeyPattern`, `recordTableName` (line 61), `recordForeignKeyName` (line 563), SQL against `athena_records` and `athena_record_operations`, `SaveRecord`, `DeleteRecord`, `recordFromProto`, `recordToProto` | `records_test.go` to `record_types_test.go` |
| `internal/server/entries.go` to `records.go` | `entryService` and its RPC methods, history SQL on `athena_entry_history` (lines about 235, 284), `athena_records` lookup (line 310), `loadEntry`, `scanEntry` | `entries_test.go` to `records_test.go`, `fieldrules_test.go` |
| `internal/server/migrations.go` | `PlanRecordSave` and per-save planner symbols | `records_test.go` migration cases |
| `internal/store/store.go`, `mariadb.go` | `Dialect` interface (lines 45 to 108), `CreateRecordTable`, `MaxIdentLength` (mariadb.go line 41), `ShortenIdent` (line 44); add rename and exists methods and `GeneratedIdentLimit` | `store_test.go`, `mariadb_db_test.go` |
| `internal/project/*` to `internal/workspace/*` | `project.go` constants and types (lines 21 to 60), `create.go` `InitWorkspace`, `Create`, `createDatabase` (lines 18 to 36, 168 to 215), `ProjectID` (line 220), `templates.go`, `lock.go` | `cmd/athena` tests; add `internal/workspace` tests for `FindHome` upgrade |
| `cmd/athena/main.go`, `migrate.go` | `printUsage` (line 66), `initCommand`, `newCommand`, `runCommand` flag `--project` (line 187), `resolveProject`, `migrationTarget` (migrate.go line 111) and flag (line 39) | `cmd/athena/migrate_test.go` |
| `proto/athena/v1/records.proto`, `entries.proto`, `system.proto` | per 2.2 (`service RecordService` and `service EntryService` blocks; `system.proto` fields 3 and 4) | `make generate`, Go and web tests |
| `web/src/api.ts`, `types.ts`, `App.vue`, pages, `EntriesTable.vue`, `MigrationDialog.vue`, `fieldRules.ts`, `telemetry.ts` | per 2.5 | `web/src/fieldRules.test.ts`, typecheck, build, browser pass |
| `AGENTS.md`, `README.md`, `docs/**` | per 2.6 | link check, term test |

---

## Answer log

**Answered by the user:** yes, rename the ADR-0004 parent folder "workspace" to "home". Section 5 therefore takes its "If yes" branch: marker file `athena-home.yaml`, Go `Home`/`HomeFile`/`InitHome`/`FindHome`/`LoadHome`/`HomeDatabase`, help text "Create a home", and a one-time rename of an old `athena-workspace.yaml` (3.5). The reserved pack name `workspace` stands (no objection raised). Plan approval was not stated separately; the answer is treated as approval to proceed.
