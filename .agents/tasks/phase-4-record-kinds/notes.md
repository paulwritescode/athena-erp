# Phase P4 notes: record kinds, system fields, field attributes
Nothing committed or pushed. The pre-existing untracked `.agents/` and the deleted `internal/webui/dist/.gitkeep` (the build wipes dist) were left alone.

## What changed
- **Kinds** (`internal/server/kinds.go`): master, document (class posting/non_posting/work/batch), child (fixed parent), setting. Kernel kinds (ledger, balance, log, snapshot, meta) are rejected. Kind and parent are fixed after first save; empty kind reads as master.
- **System fields**: `created_by`, `modified_by`, `def_version`, `request_id` (all); `owner` (master, document); `deleted_at` (master); `parent_id`, `idx` (child). All reserved as field keys. `revision` is the spec's `version` (ADR-0010), so no second counter.
- **Behavior** (`records.go`): master delete is soft (row kept, hidden, history `deleted`); setting is a singleton (lock on the definition row) and undeletable; child needs a live parent, gets the next `idx`, and blocks parent deletion; `ListRecords` takes `parent_id` for children.
- **Attributes** enforced in Go (`fieldArgument`, `bindValues`, `UpdateRecord`) and mirrored in `web/src/fieldRules.ts`: unique (Go check plus DB unique index), default, min, max, pattern (RE2, whole value), max_length, read_only, set_once, index. `no_copy` is stored and validated only: nothing duplicates or amends yet.
- **Types**: Money (BIGINT minor units, fixed currency and scale, extra places rejected, never rounded; uses `internal/types`), Quantity (DECIMAL(18,6), fixed uom, scale up to 6), Enum (permanent values). Currency, scale, unit or enum-list changes clear values through the existing confirmed data-loss plan (`clearPredicate`).
- **Migration 6** `record_kinds_and_system_fields`: existing Record Types become masters, gain columns, backfill local actor and revision. It ran against the real dev database (the `events` Record Type kept its Record).
- **Dialect**: `BigIntType`, `TimestampType`, `AddIndex`, `DropIndex`; Money/Quantity/Enum column types.
- **Proto**: `RecordType` kind/class/parent; `RecordTypeField` attributes; `EnumValue`; `Record` system fields; `parent_id` on create and list. Regenerated with `make generate`.
- **Telemetry** (`span.go`): create, update and delete Record each open a span `athena.<op>` with only operation, Record Type key, request ID and outcome code. No-op unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. `go.opentelemetry.io/otel/trace` moved from indirect to direct in `go.mod`; no new module.
- **CLI**: `athena record-types list|show|save|delete` and `athena records list|create|delete` (`cmd/athena/kernel.go`) call the same services through `server.OpenKernel` (applies pending migrations like `serve`). Flags `--workspace`, `--json`; `--addr` does not apply because no server starts and no endpoint is added (same as `migrate`). `init/new/run/serve` unchanged.
- **Frontend**: kind, class and parent selectors in Record Types; field-settings dialog (shadcn `dialog` and `checkbox` added with the shadcn-vue CLI; no change to `package.json`); Money/Quantity/Enum inputs; read-only and set-once fields locked; child parent-ID input; Setting hides New/Delete when it has its Record; system field markers. Colors use semantic tokens only; toasts stay bottom-right.
- **Docs**: ADR-0010 (Proposed), decision log, `docs/README.md`, `docs/architecture/overview.md`, README. `AGENTS.md` unchanged (nothing accepted changed).

## Invariant tests
`internal/server/kinds_test.go`: master soft delete never removes the row; set_once; unique and required on the server (including a deleted master's reserved value, index existence); unique refused while values repeat; default/read-only/min/max/pattern/max_length; money stored as minor units and extra places refused; currency change needs confirmation; kind rules; setting singleton; child parent, idx and delete guards; system columns table; migration 6 on a legacy Record Type (run twice); spans carry no Record value. Shared table `testdata/field_rules.json` grew 31 rows for attributes and new types and runs in Go and Vitest. CLI test: `cmd/athena/kernel_test.go`.

## Verification (MariaDB 11.8 from `.env.local`, values not printed)
- `go build ./...`, `go vet ./...`: pass. `gofmt -l internal cmd`: empty.
- `go test -count=1 ./...` with `ATHENA_TEST_DATABASE_URL` from `.env.local` and `OTEL_EXPORTER_OTLP_ENDPOINT=`: all packages pass.
- `make generate` twice: second run adds no diff (compared `git diff -- gen web/src/gen` checksums).
- `CI=true pnpm --dir web run typecheck`, `build`, `test`: pass (64 Vitest cases).
- `make restart`: `/healthz` returned `"database":"Connected"`; `internal/webui/dist` rebuilt (index.html and assets timestamped at restart).
- Browser pass (headless Chrome driving the desk, light and dark, temporary Record Types since deleted): Build shows Kind locked after save and the field-settings dialog; View new-record form showed pattern and money-places errors, created a Record with a Money value (shows `1234.50`, header `Credit limit (USD)`), enum label in the table, set-once field disabled on edit; Quantity places error. Screenshots were in `/tmp/p4shots` (not kept in the repo).

## Files changed
README.md, cmd/athena/{main.go,migrate_test.go,kernel.go,kernel_test.go}, docs/{README.md,architecture/overview.md,decisions/decision-log.md,decisions/adr-0010-record-kinds-and-system-fields.md}, gen/athena/v1/* (generated), go.mod, internal/server/{attributes,kernel,kind_migration,kinds,kinds_test,span}.go, internal/server/{fieldrules_test,migrations,record_types,records,sysmigrations,vocabulary_rename_test}.go, internal/server/testdata/field_rules.json, internal/store/{mariadb,store,store_test}.go, proto/athena/v1/{record_types,records}.proto, web/src/{App.vue,api.ts,fieldRules.ts,fieldRules.test.ts,style.css,types.ts}, web/src/components/{FieldSettingsDialog.vue,RecordsTable.vue,ui/checkbox/*,ui/dialog/*}, web/src/pages/{RecordTypesPage.vue,RecordsPage.vue}, web/src/gen/* (generated), internal/webui/dist (rebuilt, ignored).

## Open points for the owner
1. A soft-deleted master keeps its unique values reserved. Release them instead?
2. Reserved field keys now include `owner`, `idx`, `parent_id`; migration 6 refuses a legacy field that uses one (none in the dev database).
3. `docstate`, `name`, naming, `parent_type`/`parent_field`, `currency_field`, `required_if` and other spec attributes are deferred to later phases as noted in ADR-0010.
4. Child Records are entered by parent ID in the desk (no table-composition UI yet).

## Iteration 2 (review fix)
- `DeleteRecord` (`internal/server/records.go`) now calls `ensureNoChildRecords` inside the delete transaction. It returns `FailedPrecondition` ("other records belong to this record") when any child Record Type of the target has a Record with that `parent_id`. This covers Master parents, whose soft delete never triggers the foreign key.
- New test `TestMasterWithChildRecordsCannotBeDeleted` (`internal/server/kinds_test.go`): delete refused with a child, child deleted, parent delete succeeds.
- Ran: `go build ./...`, `go vet ./...`, `gofmt -l internal cmd` (empty), `go test -count=1 ./...` on MariaDB with `OTEL_EXPORTER_OTLP_ENDPOINT=` (all pass). No proto or web changes, so generate and pnpm checks were not rerun.
