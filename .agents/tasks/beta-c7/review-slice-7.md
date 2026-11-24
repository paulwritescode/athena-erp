# Review: C7 slice 7 stock-to-GL check (11.7)

Verdict: NEEDS_CHANGES

Tree: `athena-erp-c7-s7-r`, detached at `471107e` (on `9b1c070`), clean except the
review-required `.env.local` copy. Slice-7 delta (`8fc3f90..471107e`) is
`internal/stock/check.go` + `internal/stock/check_test.go` ONLY — scope item 5 holds.

## Per-brief findings

1. Comparison real — HOLD. Per-control-account, both sides cut on `posting_date <=
   cutoff`: GL (`debit_base - credit_base` on `l_gl`), stock (`value_base` on
   `l_stock`), deltas (`on_hand_delta` on `l_stock_value`). Verified against the
   engine: movement GL legs post at `m.PostingDate` (`stock.go` plan fragment) and
   `l_stock.posting_date` is that same date; delta GL legs post at `d.PostingDate`
   while `l_stock_value.economic_date` is the receipt's movement date (`delta.go`
   `ApplyCostDelta`) — so cutting deltas on posting date, not economic date, is what
   keeps each stock-side row in the same bucket as its GL leg. On-hand-only is the
   right adjustment: `deltaLegs` debits the on-hand portion to the stock control and
   the sold portion to COGS, so sold deltas must not enter observed. Symmetric cutoff
   also keeps submit/cancel mirrors netting on both sides.
2. Integrity rows — HOLD, each behavior pinned by a test: match writes nothing
   (`TestCheckMatchesBalancedFlow`, log count 0, observed `780.000000`); GL-heavy
   break writes expected/observed/owner/detail (`TestCheckFlagsGLHeavyBreak`,
   `880.000000`/`780.000000`, owner `controller`, status `open`, non-empty detail);
   stock-heavy break (`TestCheckFlagsStockHeavyBreak`, `1000.000000`/`1050.000000`);
   cutoff early-match/late-break with exact values (`TestCheckRespectsCutoff`);
   identical rerun reuses the open row id with count staying 1, changed values write a
   new row (`TestCheckSkipsIdenticalOpenRow`); per-account two-account match pattern
   plus company/cutoff/owner validation (`TestCheckPerAccountAndValidation`).
3. Gates — HOLD. `affected` green: `ok athena-erp/internal/stock 246.593s` through
   the suite lock on `471107e`. `gofmt -l cmd internal` empty, `go build ./...` ok,
   `go vet ./internal/stock/` ok. No proto change (2-file delta).
4. Z3 — HOLD. `check.go` reads `l_stock`/`l_stock_value`/`l_gl`/company masters and
   inserts only into `athena_integrity_log` (the specified 11.7 behavior). No
   valuation, picking, rental, or negative-stock logic touched; no stock state mutated.
5. Scope — HOLD. Delta is `check.go` + `check_test.go` only; no other stock file
   changed.

## Required change (1)

- `internal/stock/check.go` lines 22–24: the `AccountCheck.LogID` doc comment says
  LogID is `""` "when the account matched or an identical open row already exists",
  but the code sets `check.LogID = openID` on identical-open reuse (line 195) and
  `TestCheckSkipsIdenticalOpenRow` pins reuse (`first.LogID == second.LogID`). The
  comment states the opposite of the implemented and tested behavior, and slice 8/9
  consumers will read it as the contract. Fix the comment to say the reused open row's
  id is returned (keep the code and test as-is).

## Notes (no action)

- `stockControls` lists controls without a cutoff filter, so an account with only
  post-cutoff rows reports a 0/0 match. Harmless and arguably correct; not flagged.
- Skip-same compares only the newest open row for the account/cutoff; a changed break
  writes a new row rather than updating. Reasonable dedup semantics.
