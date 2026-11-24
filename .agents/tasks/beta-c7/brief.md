# Brief: C7 stock ledger, reservations, serial/batch, valuation (G11) (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c7, branch feat/c7-stock (from latest main).
Read: tasks.md C7; .kiro/specs/athena-batch-2/tasks.md G11.1–G11.11 (this task; NOTE the printed migration/ADR numbers there are stale — use the numbers below); batch-2 "Conventions"; .kiro/specs/athena-batch-2/design.md G11 + Data Models row `stock_ledger`; Requirement 11 (batch-2 requirements.md); internal/posting (G6 engine — read what exists); internal/types/quantity.go; internal/ledger/uom.go (C4 merged — UoM masters for stock quantities); proto/athena/v1/ledger.proto (patterns only — you get your own file).

Numbers assigned by the master: migration **21** `stock_ledger` (reserved in .agents/stylus/migrations.md); ADR-**0032** "Stock valuation and reservations" (Proposed). No other migration/ADR in this task. Never pick a number yourself.

Z3 RULE (binding, decided 2026-11-24 — bake it in, do not redesign): valuation is **perpetual weighted (moving) average** for all stock, one formula for items of similar nature (IAS 2.25). Perishable ingredients carry batch + expiry and issue **first-expiry-first-out** (picking order, not a cost formula). Rental items (chairs, tables, sound) are NOT consumed: serial/quantity tracking with reservations per event, no cost consumption. **Negative stock is refused** for valued items.

Slices, each merging on its own (report after each with say master "C7 SLICE n: PASS or BLOCKED <one line>"; one worker, sequential; rebase on main before reporting PASS and say if the rebase changed anything):
1. 11.1: migration 21 + ADR-0032 (Proposed, with the Z3 decisions above) + decision-log/README rows. Tables per the design row: `l_stock`, layers, item applications, bins, reservations, serials, batches, integrity log.
2. 11.2: `internal/stock/` (new): ledger rows, layers, applications, bins, cost-method interface with weighted-average; negative-stock refusal for valued items. Use `internal/posting` + `quantity.go` + UoM masters (read-only use; do not modify them).
3. 11.3: `internal/stock/delta.go` (new): append-only cost deltas (economic + posting dates), freight split on-hand/sold, rebuild watermark, close block.
4. 11.4: Property 21 (append-only cost correction) in `internal/stock/stock_test.go` (new).
5. 11.5: `internal/stock/{reserve,tracking,count}.go` (new): reservations, serial/batch/FEFO, quarantine, counts; rentals tracked-not-consumed.
6. 11.6: Property 23 (availability) in stock_test.go.
7. 11.7: `internal/stock/check.go` (new): per-control-account stock-to-GL comparison + Integrity Log entries.
8. 11.8: Property 22 (stock value equals GL) in stock_test.go.
9. 11.9: `proto/athena/v1/stock.proto` (new service — yours alone); `make generate` twice with no diff; `internal/server/stock.go` (new) + `gate.go` (append entries at the end of the list); `cmd/athena/stock.go` (new); Desk `web/src/pages/StockCheckPage.vue` (new) + RecordsPage.vue touch. GATE: do NOT touch RecordsPage.vue until master says GO (C1 6.10 owns it now; F1/F2 queue after).
10. 11.10+11.11: docs (`docs/architecture/overview.md`, README, decision log) + checkpoint (generate, test, typecheck, build, browser pass of the stock check in both themes).

Files allowed: the files above + their tests + the ADR + docs. NOT allowed: any other lane's files (ledger.proto belongs to L1, events.proto to L2, record_types.proto to L3-C4 — do not touch), App.vue (ask master), RecordTypesPage/FieldSettingsDialog/fieldRules (L3-C4 owns these — do not touch), quantity.go/uom.go/posting internals (read-only use).

Rules: batch-2 conventions (gate policies for new procedures, generate twice no diff, property tests where the design demands them, spans without values, no Record values/credentials in spans). Telemetry stays off unless the endpoint is set. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server. Full suites and browser passes only through .agents/bin/suite (it queues across slots; never bypass the lock). Package tests may run directly. Commit on your own branch feat/c7-stock (restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents. This task is big (10 slices) — if the remaining work has more than about three independent slices or one slice runs over about 90 minutes, ask master about splitting (AGENT_RULES.md) instead of grinding on.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test through suite for slice 1+ (affected for the rest where it applies), make generate stable; web typecheck/test/build for slice 9.
