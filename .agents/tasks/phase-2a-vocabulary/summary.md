# Phase P2a summary: vocabulary rename

Status: review APPROVED (pass 1, no findings). Nothing is committed or pushed; the work is in place on `main`.

## What changed

- **Vocabulary (ADR-0008, ADR-0009, both Proposed).** Record Type is the definition and maps to a table `p_workspace__<key>`. A Record is a row. Workspace replaces project. Rule means Starlark only; permissions will be Grants. Generated identifiers are lowercase, deterministic and at most 63 bytes (`store.GeneratedIdentLimit`).
- **Migration 5 `vocabulary_rename`** (`internal/server/vocabulary_rename.go`, registered in `sysmigrations.go`). It renames Record tables, system tables, columns, indexes and foreign keys (`fk_rtype_*`), rewrites the JSON key `target_record` to `target_record_type`, and sets the `pack_name` default. Every sub-step is guarded by an existence check, so it is idempotent and resumable after a crash. It refuses to merge or guess when old and new names both exist.
- **Store.** New helpers: `RenameTable`, `RenameColumn`, `RenameIndex`, `ColumnExists`, `IndexExists`, `CreateRecordTypeTable`.
- **Proto and API.** `RecordTypeService` (`record_types.proto`), `RecordService` and `FieldValue` (`records.proto`), and `workspace_name` / `workspace_slug` in `system.proto`. `MigrationStep.affected_entries` became `affected_records`. The old entries files were deleted and the code regenerated.
- **Go.** `internal/project` became `internal/workspace`. Telemetry attributes are now `athena.workspace.*`. The old names live only in `legacy_names.go`, `sysmigrations.go`, `vocabulary_rename.go`, `internal/workspace/legacy.go`, their tests, Accepted ADR bodies and the D-005 row.
- **CLI.** `--workspace` replaces `--project`, which now fails as an unknown flag. Help text says home and Workspace.
- **Web.** Pages and components were renamed (`RecordTypesPage`, `RecordsPage`, `PacksPage`, `RecordsTable`). Client names are `recordTypeClient` and `recordClient`. URL params are `record_type` and `record`. `field_rules.json` uses `target_record_type`, shared by the Go and Vitest tests. CSS classes, element ids and the `athena-entry-columns:` localStorage key keep their old spelling on purpose, so the look and saved column widths do not change.
- **Docs.** ADR-0008 and ADR-0009 were added. The decision log, ADR-0002 to ADR-0007 (banners and amendments), `record-type-and-pack-model.md`, overview, README, docs/README, AGENTS.md and `implementation.yaml` were updated.

## Verification

| Check | Result |
| --- | --- |
| `gofmt -l cmd internal gen` | empty |
| `go build ./...`, `go vet ./...` | pass |
| `go test -count=1 ./...` with MariaDB (`ATHENA_TEST_DATABASE_URL`) | pass in all packages |
| `make generate` run three times | no diff between runs |
| `pnpm --dir web` typecheck, test, build | pass; 32 Vitest tests |
| `athena help` | only new terms |
| `athena migrate status --project x` | `flag provided but not defined: -project` |
| `TestOldVocabularyIsGoneOutsideLegacyFiles` | pass; enforces the term rule for source files |
| Migration tests | data survives, idempotent, crash-resume at every statement, merge refusal, legacy key that names a system table |
| P0 tests | 4 record-type and 8 record tests, renamed and none dropped |
| Live dev database (after user go-ahead) | migration 5 applied; `/healthz` ok; the `events` Record Type and its Records survived; Connect calls for list, save, create, delete and history succeeded |

## Caveats

- **No backup before migrating the dev database.** Reading credentials for `mariadb-dump` was blocked, and the user allowed proceeding. Migration 5 cannot be rolled back.
- **No browser pass.** Build and View in light and dark were not checked visually. The bundle builds, typechecks and is served with HTTP 200.
- AGENTS.md's list of files allowed to hold old names does not mention the CSS classes, element ids and localStorage key. Consider adding them.
- `docs/architecture/implementation.yaml` was already invalid YAML at HEAD (parse error near line 278) and was left alone.
- The roadmap still names the auth ADR as ADR-0008. The next free number is ADR-0010, and the roadmap file was not edited.

## Next: phase P4, record kinds and system fields

Use the new vocabulary throughout (Record Type, Record, Workspace, Pack, Grants).

1. Read the roadmap (`.agents/tasks/erp-kernel-roadmap.md`) for P4 scope and the kernel spec for the kinds and system fields.
2. Propose the decision first. Record it as Proposed in the decision log and in an ADR (ADR-0010 or later). Cover which kinds exist and which system fields each kind gets, and say whether they are columns on `p_workspace__<key>`.
3. Add them as new numbered entry 6 in `server.SystemMigrations()` and update `PlanRecordTypeSave` and the journaled publisher in `internal/server/record_types.go`. Never edit applied migrations.
4. Extend `proto/athena/v1/` and run `make generate`. Keep `web/src/fieldRules.ts` and `internal/server/records.go` in step through `field_rules.json`.
5. Add dashboard support with shadcn-vue components, semantic tokens, and Sonner toasts for action results.
6. Test against MariaDB with `make test`. Take a backup before touching the dev database, and do the browser pass that P2a skipped.
