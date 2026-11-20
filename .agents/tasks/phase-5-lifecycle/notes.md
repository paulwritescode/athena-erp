# Phase P5 notes: document lifecycle
Nothing committed or pushed. The pre-existing untracked `.agents/` and the deleted `internal/webui/dist/.gitkeep` (the build wipes dist) were left alone.
## What changed
- **`internal/lifecycle`** (new): `State` (draft, submitted, cancelled), `Next` state machine, event names and `Order` (spec 5.2), `Sequence(action)` giving the fixed firing order and which steps run in the transaction, and a Go hook `Registry` (`Kernel.Hooks`). Starlark attaches in P9.
- **Record Type flags:** `submittable` (documents except class work), `amendable` (needs submittable), field `allow_on_submit`. Validation in `validateLifecycle`; a type cannot stop being submittable or amendable once documents are submitted or amended.
- **System columns** on submittable documents: `docstate`, `name` (starts as the Record ID, unique index), `amended_from` (amendable). `docstate` and `amended_from` are reserved field keys everywhere; `name` only on submittable documents. Existing rows become drafts when a type becomes submittable (`ensureLifecycle`).
- **RPCs on `RecordService`:** `SubmitRecord`, `CancelRecord`, `AmendRecord`, `ListRecordVersions`, `ListRecordActivity` (`proto/athena/v1/records.proto`; regenerated). No JSON endpoints.
- **Behavior** (`internal/server/lifecycle.go`, `records.go`): each transition is one transaction with the revision check, state change, history, Document Version, Activity row and in-transaction hooks; `after_*` events run after commit. Submitted documents change only in `allow_on_submit` fields (audited as `edited_after_submit`); cancelled documents never change; only drafts can be deleted (`on_trash`). Child Records follow the parent state. Cancel is blocked by a submitted downstream document (direct Link field or through a child line). Amend creates a draft with `amended_from`, name `<root>-N`, copies values and child Records except unique, read-only and `no_copy` fields; a required field left empty makes the amend fail.
- **Migration 7** `document_lifecycle`: `athena_document_versions`, `athena_activity`.
- **Refactors:** `insertRow` extracted from `CreateRecord`; `ensureNoChildRecords` now uses `childRecordTypes`; `ensureSystemColumns` takes a column list; `recordTypeDefinition.systemColumns()`.
- **Telemetry:** submit, cancel and amend each open `athena.<op>` spans with record type key, request ID, `athena.state_from`, `athena.state_to` and outcome only (test checks no value leaks). Off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set.
- **CLI:** `athena records submit|cancel|amend <type> <id> --revision N`, `versions`, `activity` (`--workspace`, `--json`; `--addr` does not apply, no server starts). Same kernel services. Test: `TestRecordsLifecycleCommands`. init/new/run/serve unchanged.
- **Frontend:** Submittable and Amendable checkboxes in Build, "Allow on submit" in field settings, Status column and badge, Submit / Cancel document / Amend with alert-dialog confirmations, Activity and Versions lists, locked inputs by state (`fieldEditable` in `web/src/fieldRules.ts`, with Vitest cases). Semantic tokens only; toasts bottom-right; no new dependency.
- **Docs:** ADR-0011 (Proposed), D-015, `docs/README.md`, overview, README. `AGENTS.md` unchanged (no accepted decision changed).
## Invariant tests
`internal/server/lifecycle_test.go` and `internal/lifecycle/lifecycle_test.go`: immutability of every non-`allow_on_submit` field after submit, cancelled documents fixed, lines follow the parent, cancel blocked by live downstream link (and allowed after it is cancelled), amend keeps the original cancelled and names X-1, X-2, no second amend, versions and diffs, event order equals `lifecycle.Sequence`, `on_trash` only for drafts, failure in `on_submit` rolls back state, versions, activity and history, stale revision rejected, definition rules, legacy Records become drafts, spans.
## Verification (MariaDB 11.8 from `.env.local`, values not printed)
- `go build ./...`, `go vet ./...`: pass. `gofmt -l cmd internal`: empty.
- `go test -count=1 ./...` with `OTEL_EXPORTER_OTLP_ENDPOINT=`: all packages pass (migration-count assertions in `migrate_test.go` and `vocabulary_rename_test.go` updated to 7).
- `make generate` twice: second run adds no diff.
- `CI=true pnpm --dir web run typecheck`, `build`, `test`: pass (67 Vitest cases).
- `make restart`: `/healthz` returned `"database":"Connected"`; `internal/webui/dist` rebuilt at restart.
- Browser pass (headless Chrome over DevTools protocol, light and dark, temporary type `p5_order`, since deleted): create draft, submit (title locked, allow-on-submit note editable, Delete hidden), cancel document (Save hidden), amend (new Draft named `<id>-1`, activity shows "Created by amending"). Screenshots in `/tmp/p5` (not kept in the repo).
## Files changed
README.md, cmd/athena/{kernel.go,kernel_test.go,main.go,migrate_test.go}, docs/{README.md,architecture/overview.md,decisions/decision-log.md,decisions/adr-0011-document-lifecycle.md}, gen/athena/v1/* (generated), internal/lifecycle/{lifecycle.go,lifecycle_test.go}, internal/server/{lifecycle.go,lifecycle_test.go,kernel.go,kind_migration.go,kinds.go,record_types.go,records.go,sysmigrations.go,vocabulary_rename_test.go}, proto/athena/v1/{record_types,records}.proto, web/src/{api.ts,fieldRules.ts,fieldRules.test.ts,style.css,types.ts}, web/src/components/{FieldSettingsDialog,RecordsTable}.vue, web/src/pages/{RecordTypesPage,RecordsPage}.vue, web/src/gen/* (generated), internal/webui/dist (rebuilt, ignored).
## Open points for the owner
1. Amending a document with a required field that is also unique, read-only or no-copy fails, because those fields are left empty on the successor. Is that acceptable, or should cancelled documents release their unique values?
2. `name` equals the Record ID until P6 numbering.
3. Cancel writes no ledger reversal (no ledgers yet); period locks, workflows and business status are not built.
4. Version snapshots hold Record values; they need the same permissions as Records once P8 lands.
5. Lines of a submitted document are edited by child Record ID in the desk, as in P4.
