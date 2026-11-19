# ADR-0008: Vocabulary: Record Type, Record, Pack, Workspace, Grant

- **Status:** Proposed
- **Date:** 2026-11-19
- **Decision owner:** Project owner
- **Related decisions:** D-005 (superseded by this ADR once accepted), ADR-0003 and ADR-0004 (amended by this ADR once accepted), ADR-0009, O-005
- **Source:** ERP kernel specification section 22 (22.1 verdicts, 22.2 Pack and Workspace, 22.3 Record Type and Record, 22.4 Module, 22.5 Rule and Grant, 22.7 earlier wording) and section 23.2 (naming)

## Context

D-005 used "Record" for a definition that maps to a table and "Record entry" for a row. ADR-0004 used "project" for the database-backed place you work in and "workspace" for the parent folder that holds projects and the server login. The kernel specification (section 22) gives each of these words another meaning: Record is the row, Record Type is the definition, Workspace is the database plus its installed packs and data, Pack is the installable bundle, and Module is a functional area inside a Pack. Keeping the old words would mean two terms for one thing and one term for two things, and these names become table names, Protobuf services and permission keys, so they are costly to change later. The project owner chose to adopt the specification vocabulary now (roadmap section 8, answer A). Nothing has been released, so a breaking change is acceptable.

## Options considered

1. **Keep the old words.** No work, but every later phase reads the specification through a translation table and "workspace" keeps two meanings. Rejected by the project owner.
2. **Rename only the labels in the desk.** Cheap, but the old names stay in tables, Protobuf and the CLI, so the clash is only hidden. Rejected.
3. **Rename everything once, now (chosen).** One migration, one proto change, one documentation pass, before other phases build on the names.

## Decision

The mapping below applies to code, database, API, CLI, desk and documentation.

| Old term | New term | Notes |
| --- | --- | --- |
| Record (definition that maps to a table) | **Record Type** | Key stays lowercase snake_case. |
| Record entry (a row) | **Record** | "Entry" is no longer a noun. |
| Module (installable bundle) | **Pack** | Installation is not implemented. |
| Module (functional area) | **Module** | Only a documentation term for now (spec 22.4). |
| Project (ADR-0004) | **Workspace** | A database plus installed Packs, Records, users and overlays. |
| workspace (ADR-0004 parent folder) | **home** | Chosen to avoid a clash with Workspace; the project owner approved it. |
| Rule (Starlark program) | **Rule** | Code only. |
| Permission rule | **Grant** | Used by the permission phase; nothing is renamed now. |

- Persisted names: `athena_records` becomes `athena_record_types`, `athena_record_<key>` becomes `p_workspace__<key>`, `athena_record_operations` becomes `athena_record_type_operations`, `athena_entry_history` becomes `athena_record_history`, `athena_modules` becomes `athena_packs`, `athena_project` becomes `athena_workspace`, `athena_rules.module_name` becomes `pack_name`, and the stored link key `target_record` becomes `target_record_type`. System migration 5 (`vocabulary_rename`) does this and is covered in ADR-0009 for the naming rules.
- API: `RecordTypeService` (`ListRecordTypes`, `PlanRecordTypeSave`, `SaveRecordType`, `DeleteRecordType`) and `RecordService` (`ListRecords`, `GetRecord`, `CreateRecord`, `UpdateRecord`, `DeleteRecord`, `ListRecordHistory`). `SystemInfo` reports `workspace_name` and `workspace_slug`. The old services are not kept.
- CLI: `athena init` creates a home (`athena-home.yaml`), `athena new` creates a Workspace, and `--project` becomes `--workspace`. An old `athena-workspace.yaml` is renamed once, in place, the first time a command finds it.
- Desk: Build shows Overview, Record Types, Packs and Rules; View lists Record Types and their Records.
- `Pack` and the reserved pack name `workspace` (Record Types authored before any Pack exists) are separate things.
- After migration 5 the old names may appear only in the frozen legacy files (`internal/server/legacy_names.go`, `sysmigrations.go`, `vocabulary_rename.go`, `internal/workspace/legacy.go`), their tests, Accepted ADR bodies and the decision log row for D-005. A test enforces this for source files.

### Not decided here

Pack packaging and versioning (O-005), the Grant model, and the final product name (O-009).

## Consequences

- Breaking change to the Protobuf contract, the CLI flags and the table names. Databases and homes created before this change are upgraded by migration 5 and the home marker rename, and keep their data.
- Every later phase uses these terms. The roadmap and the kernel specification read directly against the code.
- ADR-0003 and ADR-0004 stay as written, with an amendment note pointing here. D-005 is marked superseded when this ADR is accepted.
- CSS class names in the desk (for example `records-layout`, `entries-panel`) keep their old spelling; they are not part of any contract.

## Revisit when

The specification changes section 22, or a clash appears between Pack and a third-party meaning of the word.
