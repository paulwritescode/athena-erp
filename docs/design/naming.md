# Naming registry

**Status:** Proposed, 2026-11-21. Source: `.kiro/specs/athena-beta/tasks.md`
section N (open decision Z11: the user approves before any rename). Product
names are labels only: code identifiers, proto services and table names stay
as they are, so nothing breaks. A rename of a code identifier needs its own
ADR and a compatibility alias (R6.2). No code renames were made with this
registry.

Pattern from section N: **Athena + one concrete noun**, 1 to 2 syllables,
works as `athena-<noun>` in the CLI and as a word a user understands. The
everyday user mostly sees the plain section names ("Books", "Stock",
"Reports"); the product names appear in docs, the CLI and Settings.

## Product names to layers

| Product name | CLI | Proto service | Go package | UI label |
| --- | --- | --- | --- | --- |
| Athena Core | `athena init`, `new`, `run`, `serve`, `migrate`, `workspace` | `SystemService`, `WorkspaceService` | `internal/server`, `internal/workspace`, `cmd/athena` | Overview, Setup |
| Athena Wire | — (the API itself, no CLI) | `athena.v1.*` (`RecordTypeService`, `RecordService`, `AuthService`, `FileService`, `RuleService`, `LedgerService`, `NotificationService`, `WorkspaceService`, `SystemService`) | `gen/athena/v1` (generated), handlers in `internal/server` | — (powers the desk; no page of its own) |
| Athena Script | `athena rules` | `RuleService` | `internal/rules` | Rules |
| Athena Vault | `athena audit`, `athena access-log` | Record history on `RecordService`; Access Log reads (planned surface) | `internal/audit`, `internal/store` | History (record detail), Access |
| Athena Loom | planned (G8) | planned (G8) | `internal/jobs` (planned, G8) | Jobs |
| Athena Ledger | `athena ledger` | `LedgerService` | `internal/ledger` | Ledger |
| Athena Stock | planned (G11) | planned (G11) | `internal/stock` (planned, G11) | Count |
| Athena Prism | planned (G9) | planned (G9) | `internal/report` (planned, G9) | Reports |
| Athena Lens | — (no CLI surface) | — (no service) | `internal/telemetry` | — (no UI) |
| Athena Bridge | planned importers (G15) | planned (G15) | planned (G15) | Jobs (imports and exports) |

Notes:

- Backups have no CLI or service yet; when they land they join Athena Vault
  in this table.
- `athena user`, `athena role`, `athena security`, `athena record-types`,
  `athena records`, `athena files`, `athena notifications`,
  `athena expressions` are Core operator commands over the services above;
  they are not separate product names.
- Planned rows name the batch-2 group that defines them; the Go package and
  service names there are the current working names, not promises.

## Builder and everyday words

Builder vocabulary (ADR-0008, Proposed) is used in Build mode, the CLI, the
API and the docs: Record Type, Record, Pack, Module, Workspace, home, Rule,
Grant. View mode and task screens use everyday words from
`web/src/copy/glossary.ts`, which is the only source for those labels
(design-system section 8, R6.1); statuses such as "Posted" show as "Recorded
in the books". Build mode keeps the builder words and explains each term
where it first appears with a `HelpHint`.

## Forbidden old terms

Do not use the pre-ADR-0008 words for their old meanings: Record entry,
entry (as a noun), project (for a Workspace), Module (for an installable
bundle). They live only in the frozen legacy files
(`internal/server/legacy_names.go`, `sysmigrations.go`,
`vocabulary_rename.go`, `internal/workspace/legacy.go`), their tests,
Accepted ADR bodies and the D-005 decision-log row; `terms_test.go`
enforces this for source files. Desk CSS class names, element ids and the
`athena-entry-columns:` localStorage key keep their old spelling on purpose.
`terms_test.go` is extended to new forbidden terms only after user approval
(task B6).

## How a name maps between layers

- CLI: `athena <verb>`, lowercase with dashes (`record-types`,
  `access-log`). One verb per capability; flags use `--workspace`, never
  `--project`.
- Proto: `<Name>Service` in package `athena.v1`, methods in CamelCase
  (`ListRecords`, `SaveRecordType`). Field numbers are never reused.
- Go: `internal/<noun>` for the owning package (`internal/rules`,
  `internal/ledger`); generated code in `gen/athena/v1`. Exported
  identifiers in CamelCase.
- UI: one or two plain words per page ("Ledger", "Work queue"), sentence
  case in descriptions, from the glossary in View mode.
- Database: platform tables `athena_<name>`; one Record Type table
  `p_workspace__<key>` per type (ADR-0009, Proposed).
