# Phase P7 notes: expressions on cel-go
Nothing committed or pushed (the approval step commits). `review.json` is created by the reviewer. `internal/webui/dist/.gitkeep` shows as deleted because the web build wipes dist (same as P6); pre-existing untracked `.agents/` left alone.

## What changed
- **`internal/expr`** (new): restricted CEL on `cel.dev/cel-go` v0.33.4 (the module moved from `github.com/google/cel-go`). Std lib limited to `&& || ! ?:`; macros off (no loops); no floats, unsigned, bytes, lists or maps; 2000-char and recursion-32 limits. Exact `decimal` and `money` types over `internal/types`; `date`. Null rules: arithmetic null, ordering with null false, `==`/`!=` null-safe. `today()` is injected via a macro to a hidden variable. `Compile` type-checks and reports kind and dependencies; `Eval` is deterministic; `Translate` makes a SQL predicate through 4 new `store.Dialect` methods (`NullSafeEqual`, `BinaryString` = `utf8mb4_nopad_bin`, `DecimalParam`, `DateParam`) with constant folding.
- **Field attributes** (proto 21 to 25, JSON definition, no table change): `formula`, `required_if`, `read_only_if`, `validate`, `depends_on`. Compiled at save (`validateExpressions`): result kind must fit the field, cycles and self-reference refused, `today()` refused in formulas, a child save re-checks the parent's expressions.
- **Write path** (`internal/server/expressions.go`): formulas stored in dependency order on create and update; clients cannot set them; `required_if` and `validate` checked on final values; `read_only_if` on stored values (updates). Child create, update and delete recalculate the parent's rollups in the same transaction, without a parent revision change, with a `recalculated` Audit Log row. Changing a formula adds a `recalculate` plan step and recalculates stored Records (per-Record transactions, idempotent).
- **API:** `ListRecordsRequest.filter`; `RecordService.PreviewRecord` and `EvaluateExpression`. ConnectRPC only.
- **CLI:** `athena expressions check|eval|preview` (new, `cmd/athena/expressions.go`), `athena records list --filter`. Flags `--workspace` (not `--project`, per AGENTS.md and P2a) and `--json`; `--addr` does not apply (no server starts). init/new/run/serve unchanged. Test: `TestExpressionsCommand`.
- **Telemetry:** `preview_record` and `evaluate_expression` spans via `startKernelSpan` (operation, type key, request ID, outcome). `TestExpressionSpansNeverCarryRecordValues` checks no value or expression appears. Off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set.
- **Frontend:** the field settings dialog has Formula, Required when, Read only when, Must be true, Show when. The record form calls `PreviewRecord` (debounced) to show calculated values, required marks, locked and hidden fields; the table has a filter box. `fieldRules.ts`: `isCalculated`, `validateExpressionText`; formula fields are never editable or required (Vitest, 72 cases). No new components, no hard-coded colors, Sonner toasts for errors.
- **Docs:** ADR-0013 (Proposed, why cel-go fits), D-017, decision log, `docs/README.md`, overview (status row, logic boundaries), README commands. `AGENTS.md` unchanged (nothing accepted).

## Invariant tests
`internal/expr/expr_test.go`: table-driven evaluation, null semantics, money arithmetic and currency mismatch, rejected forms, determinism (50 runs and recompile), translation shape and not-translatable cases. `internal/server/expressions_test.go`: dependency order (declared in reverse), cycle, self-reference, bad kind, float and today refused at save, client cannot set a formula, `required_if`/`read_only_if`/`validate`, rollup equals recomputation from child rows after add, edit and delete (parent revision unchanged, 4 `recalculated` audit rows), formula change recalculates stored Records and shows in the plan, a child change that breaks the parent is refused, preview, 36 filters give the same rows in SQL and in memory over rows with nulls, case and trailing-space differences, unsupported filters refused, span test.

## Verification (MariaDB 11.8 from `.env.local`, value not printed)
- `go build ./...`, `go vet ./...`: pass. `gofmt -l cmd internal`: empty.
- `OTEL_EXPORTER_OTLP_ENDPOINT= go test -count=1 ./...`: all packages pass (server 136 s).
- `make generate` twice: `git diff -- gen web/src/gen` hash identical both times.
- `CI=true pnpm --dir web run typecheck`, `test` (72 passed), `build`: pass.
- `make restart`: `/healthz` returns `{"status":"ok","database":"Connected"}`; `internal/webui/dist` rebuilt; the desk shows no database warning.
- Headless Chrome pass (`/tmp/p7/drive.mjs`, log `/tmp/p7/browser.log`, screenshots in `/tmp/p7`), light and dark, 13 checks each, 26 pass, 0 fail: formula shown and locked, Show when hides and shows, Required when marks and blocks save, validate rejects, create lists the value, filter empty, match and bad-filter toast, formula visible in field settings. The temporary `p7_order` type was deleted afterwards (its Audit Log rows stay).

## Files changed
README.md, cmd/athena/{expressions.go,kernel.go,kernel_test.go,main.go}, docs/{README.md,architecture/overview.md,decisions/decision-log.md,decisions/adr-0013-expression-language.md}, gen/athena/v1/* and web/src/gen/* (generated), go.mod, go.sum, internal/expr/{expr.go,funcs.go,value.go,sql.go,expr_test.go}, internal/server/{expressions.go,expressions_rpc.go,expressions_test.go,migrations.go,record_types.go,records.go}, internal/store/{mariadb.go,store.go}, proto/athena/v1/{record_types,records}.proto, web/src/{api.ts,fieldRules.ts,fieldRules.test.ts,style.css,types.ts}, web/src/components/FieldSettingsDialog.vue, web/src/pages/RecordsPage.vue, internal/webui/dist (rebuilt, ignored).

## Open points for the owner
1. A child Record Type needs its parent saved first, so a parent's rollup formula is added in a second save. The save plan then recalculates.
2. Compiled expressions are not cached across requests.
3. Lookup fields (`fetch_from`), `hidden_if`, `default_from`, `user`/`setting()`/`ctx` scopes, `period_of` and `in` are not built; they belong to P8, P11 and later.
4. `money * number` rounds once and drops the difference; there is no `/` (use `div(a, b, places)`); `int == decimal` needs `dec(...)`.
5. The desk preview is a hint; the server decides on save.
6. PostgreSQL is untested; the four new dialect methods have only a MariaDB implementation.
7. The brief said `--project`; the repository standard is `--workspace`.
