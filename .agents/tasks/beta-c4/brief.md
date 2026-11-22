# Brief: C4 trees, effective dating, determination specificity, UoM (G13) (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c4, branch feat/c4-trees (from latest main).
Read: tasks.md C4; .kiro/specs/athena-batch-2/tasks.md G13.1–G13.7 (this task; NOTE the printed migration/ADR numbers there are stale — use the numbers below); batch-2 "Conventions"; .kiro/specs/athena-batch-2/design.md G13; requirements R13 (batch-2 requirements.md); internal/ledger (determination, masters); internal/server attributes + records + record_types + migrations; internal/types/quantity.go; internal/store/store.go.

Numbers assigned by the master: migration **18** `trees_effective_uom` (reserved in .agents/stylus/migrations.md); ADR-**0029** "Trees, effective dating and UoM" (Proposed). No other migration/ADR in this task. Never pick a number yourself.

Slices, each merging on its own (report after each with say master "C4 SLICE n: PASS or BLOCKED <one line>"; one worker, sequential; rebase on main before reporting PASS and say if the rebase changed anything):
1. 13.1: migration 18 + ADR-0029 (Proposed) + decision-log/README rows.
2. 13.2: tree + effective-date attributes (`internal/server/attributes.go`, `records.go`, `record_types.go`, `migrations.go`, `internal/store/store.go`: path, depth, cycle, range checks) + `proto/athena/v1/record_types.proto`; `make generate` twice with no diff.
3. 13.3: UoM families + conversions (`internal/types/quantity.go`, new `internal/ledger/uom.go`: factors, overrides, cross-family rejection) + `internal/ledger/determination.go` uses effective ranges.
4. 13.4: Property 25 (effective ranges + UoM round trip) in `internal/ledger/ledgers_test.go` + `internal/types/types_test.go`.
5. 13.5: CLI (`cmd/athena/ledger.go`: tree, uom, determination subcommands) + Desk (`web/src/pages/RecordTypesPage.vue`, `web/src/components/FieldSettingsDialog.vue`, `web/src/fieldRules.ts` with `internal/server/testdata/field_rules.json` in step). Desk last by design.
6. 13.6+13.7: docs (`docs/architecture/record-type-and-pack-model.md`, decision log) + checkpoint (generate, test, typecheck, build, browser pass of the touched screens in both themes).

Files allowed: the files above, their tests, the ADR + docs. NOT allowed: gate.go (append-only, master resolves), App.vue (ask master), any other lane's files (another lane owns other Desk pages right now — stay inside your listed files).

Rules: batch-2 conventions (generate twice no diff, property tests, spans without values, no Record values/credentials in spans). Telemetry stays off unless the endpoint is set. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server (never the dev database). Full suites and browser passes only through .agents/bin/suite (it queues; never bypass the lock). Package tests may run directly. Commit on your own branch feat/c4-trees (restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents. If the task has more than about three independent slices left or one slice runs over about 90 minutes, ask master about splitting (AGENT_RULES.md) instead of grinding on.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test through suite, make generate stable; web typecheck/test/build for slice 5.
