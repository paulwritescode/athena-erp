# Review: C7 slice 2 stock core (11.2) — internal/stock seam

- Tree: `/Users/iampaulking/projects/duchess/athena-erp-c7-s2-r`, detached at `a69d8b3`, clean (`git status` empty).
- Delta reviewed: `b3e9ca4..a69d8b3` ONLY (6 new files, all `internal/stock/**`, +2351). Slice 1 out of scope.
- Reviewer: r-c7s2. Date: 2026-11-24.

Verdict: NEEDS_CHANGES (first review at `a69d8b3`; re-review of fix tip
`325cca0` below — see "Re-review" section for the final verdict).

Three slice-scoped defects need fixes (empty cancel-edge kinds, concurrent-return
over-return race, untested cancel/claim paths); the rest is sound and the tests
that exist are real asserts through the G6 engine.

## 1. Seam: Plan/Apply around G6 — mostly correct, one ordering hazard

- One plan→apply contract holds: `PlanMovement` validates, locks the bin,
  converts UoM read-only via `ledger.ToBaseQuantity`, costs, builds balanced
  legs and validates the fragment with `posting.ValidatePlan`; the caller posts
  with `PostInTx`; `ApplyMovement` writes under the group id. Tx boundaries are
  honest (everything takes the caller's `*sql.Tx`, no hidden commits).
- `claimGroup` is byte-identical to the engine's `insertGroup` column list and
  shares the post-once unique key, so quantity-only lines reuse a G6-posted
  group when the engine posts first. `TestQuantityOnlyMoveClaimsGroup` proves
  one group, zero GL rows. Sound.
- FINDING F4 (doc fix required, slice-11 watch): order is fragile for mixed
  vouchers. If a quantity-only `Apply` claims the slot BEFORE the G6 post of
  the same key, the later engine post hits `replayGroupResult`, finds no
  `athena_operations` row for the claimed random operation id, and fails with
  an internal `no operation row ... points at group` error
  (`internal/posting/operations.go`). So for mixed vouchers the G6 post MUST
  precede quantity-only applies. Require: state this ordering contract in the
  seam doc comment (`stock.go` package comment); slice 11 must wire plan-all →
  post-GL → apply-all in that order. (Reverse direction is safe: claiming
  reuses an engine-posted group; `ReverseInTx` on a rowless claimed group
  fails loudly with `posted group holds no rows`, no panic — verified in
  `internal/posting/reverse.go`.)
- Quantity-only moves skip ALL period/book validation (no legs → no fragment
  validation, and `checkMasters` only checks company/currency/accounts). Noted,
  not demanded: whether quantity-only moves need an open period is undecided
  (Z2-adjacent). Slice 11 should carry the question, not this slice.

## 2. Costing

- Weighted average behind `CostMethod` is correct: receipt reblend
  `(binValue + qty*cost)/(binQty + qty)`, issue at `binValue/binQty`, HalfUp to
  6dp. `average_test.go` (5 tests) pins blend, first-receipt, issue, empty-bin
  error, rounding. Real asserts.
- Layers + applications (submit path) correct: oldest-first consume keyed on
  UUIDv7 receipt order, last-take dust absorption so takes sum exactly,
  full-return exact restore, partial-return proration with dust on the last
  edge. `TestReceiptsBlendAndIssueTakesAverage` (spanning issue, 2 edges,
  residuals) and `TestReturnTakesIssueCost` (incl. `ErrOverReturn` and
  return-of-receipt refusal) are true asserts.
- FINDING F1 (defect): cancel application edges are stored with `kind = ''`.
  `planReceiptCancel`, `planIssueCancel`, `planReturnCancel` (`stock.go`)
  never set `layerOp.Kind`, and `applyCancelLayers` (`layers.go:351`) inserts
  `op.Kind` verbatim. Grep confirms `Kind` is assigned only in `coverLayers`
  (`"issue"`) and `prorateRestore` (`"return"`). ADR-0032's taxonomy is
  `kind='issue'` / `kind='return'` with "reversal mirrors", and the
  `application` comment says "Kind with Reversal gives the direction" — with
  `Kind=''` the direction is unknowable. Nothing reads reversal edges by kind
  yet, so tests pass, but slice 7 (`check.go`) / slice 8 (Property 22)
  aggregations over applications will inherit meaningless rows. Fix: carry the
  mirrored kind on cancel ops (issue-cancel restore → `"issue"`,
  return-cancel consume → `"return"`, receipt-cancel consume → `"issue"`; all
  with `is_reversal=1` as today) and add a test asserting stored kinds.
- Negative refusal valued-only: confirmed per Z3's letter ("refused for valued
  items"). `planIssue` returns early for unvalued, the `ApplyMovement`
  recheck is `if m.Valued`, `PlanCancel`'s running check is `if
  mirror.valued`, and `TestNegativeStockRefusedForValuedOnly` asserts both the
  `ErrNegativeStock` refusal and the unvalued bin at `-1.000000`. Yes, exempt.
- FINDING F2 (defect): concurrent returns against one issue can over-return.
  `planReturn` reads the issue row (`loadStockRow`) and prior returns
  (`sumPriorReturns`) as plain snapshot SELECTs with no lock on the reversed
  issue row, and `ApplyMovement` does not recheck. Two transactions can both
  plan against `prior=0` and both apply — the race is isolation-independent
  (no lock is ever held on the issue row, so interleaved plan/plan then
  apply/apply breaks `ErrOverReturn` under REPEATABLE READ and READ COMMITTED
  alike). Every other refusal decision in this slice uses `FOR UPDATE`; this
  one is the gap. Fix: lock the reversed issue row `FOR UPDATE` in
  `planReturn` (held across the caller's plan→apply→commit) and make the two
  `sumPriorReturns` reads locking so a waiter sees the winner's rows.
- Cancel mirrors (valued issue path) are exact: negated qty/value, same grain,
  accounts, UoM, currency, dimensions, source line, `event=cancel`,
  `is_reversal=1`, `reversed_row_id` set; layers restored along the issue's
  edges; `TestCancelMirrorsSubmit` asserts bin, layer, flagged mirror count,
  GL balance `1000.000000`, and cancel replay. `ErrCancelBlocked` on consumed
  receipts and `ErrNoSubmit` proven. Real asserts.
- Bins + rebuild: `applyBinDelta` duplicate-insert race handled via
  dialect-classified reselect; `RebuildBins` restores a corrupted bin (test
  asserts qty `60` + value `600`) including cancel mirrors, value deltas and
  active reservations. Sound for this slice.
- Slice-5 watch (note, do not fix here): layer grain is
  (company, book, item, location, lot) while bin grain adds owner + serial, and
  `openLayers` filters neither owner nor serial. Fine for slice 2's untracked
  single-owner flows, but serial/owner layer attribution (rentals, quarantine
  as ownership grain) must be settled when slice 5 lands, or issues will
  consume another serial's/owner's layers while costing at their own bin's
  average.

## 3. Tests: 15 tests, real — with coverage gaps

- Count verified: 5 in `average_test.go` + 10 in `movements_test.go` =
  `grep -c "^func Test"` 5/10. All DB tests drive the production seam
  (plan → real `PostInTx`/`ReverseInTx` → apply, one tx) against migrated
  `test_athena_*` DBs. GL-balance matches (`1080.000000` after issues,
  `1000.000000` after cancel), replay (post-twice returns first row id, bin
  unchanged; second cancel replays row ids), and cancel-block are value
  asserts, not shapes. Box→each conversion, validation table (14 cases), group
  claim, rebuild — all concrete.
- FINDING F3 (test gap): three paths have zero coverage —
  (a) quantity-only cancel (`ApplyCancel` with `cancelGroupID=""`, the
  event=cancel `claimGroup` branch): `cancelVoucher` is only ever called with
  `withGL=true`, no test cancels a quantity-only voucher;
  (b) receipt-cancel success (unconsumed receipt mirror + layer consume-back);
  (c) return-cancel success (`planReturnCancel`/`applyCancelLayers` restore
  path). Require at least (a) and one of (b)/(c) with stored-kind asserts
  (covers F1's fix).
- `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet ./...` OK
  (all run in this review worktree at `a69d8b3`).
- No proto change: delta contains no `.proto`/generated/gate files, so
  `make generate` is trivially stable — verified via diff stat.
- affected (reviewer's own run at `a69d8b3`, through the suite lock): GREEN —
  `ok cmd/athena 247s`, `ok internal/server 423s`, `ok internal/stock 117s`,
  exit 0. Matches the worker's pre-rebase greens on content-identical slice
  files; the rebased tip introduces no test regression.

## 4. Scope: clean

- Delta is `internal/stock/**` + its tests only. `posting`, `quantity.go`,
  `uom.go`, ledger masters untouched (read-only use confirmed: `ToBaseQuantity`
  item-arg is the per-item conversion override, `ValidatePlan`/`PostInTx`/
  `ReverseInTx` called, never modified). Lifecycle wiring absent as expected —
  slice 11's job; noted, not demanded. One stale comment to fix with F4's doc
  edit: the package comment says wiring "is not scheduled in any C7 slice
  yet" — it is now slice 11; name it.

## 5. Worker evidence: verified

- `suite.jsonl`: worker's affected at pre-rebase `a396319` exited 0, clean,
  431s; log `1124-063434-w-c7.log` has 0 `FAIL` lines and
  `ok cmd/athena 251s`, `ok internal/server 428s`, `ok internal/stock 114s` —
  matches the brief's 251s/428s/114s exactly.
- Rebase `b0510f1→a6dec5c` content check: `git diff a396319 a69d8b3 --
  internal/stock` is EMPTY (0 lines); the only `a396319..a69d8b3` changes are
  the 49 main-line base files (RecordsPage.vue, brand.go, tooling, agent
  docs). Both picks replayed byte-identical: slice-2 pick content unchanged
  across the rebase. (Post-rebase the worker ran only narrow
  `internal/stock`, 289s — hence this reviewer's own affected run.)

## Nits (fix opportunistically, not verdict-blocking)

- `PlanMovement` replay comment says it "skips costing and locking" but
  `findSubmitRow` takes `FOR UPDATE` — it locks. Reword.
- `stock.Service.Dialect` nil panics in `claimGroup`/`applyBinDelta`
  (`s.Dialect.Classify`); the G6 engine explicitly nil-guards its dialect
  ("never panic"). Document Dialect as required or mirror the guard.
- `checkBinMatch` does not compare `ItemType` (same `item_id`, different
  type would share a bin). UUIDs make this caller-bug-only; a 3-line guard
  would close it.
- `validDate` is shape-only (`2026-13-99` passes, then fails at the DATE
  column with a raw driver error, not `ErrInvalid`). Acceptable; could
  classify.
- Mixed `Valued` flags within one bin are not refused: an unvalued issue
  relieves quantity without touching value, silently corrupting the bin's
  value-per-unit for later valued issues. No guided slice-2 flow mixes flags;
  slice 5 (quarantine/rental quantity-only flows) should decide the guard.

## Required changes (for PASS)

1. F1: set `Kind` on all cancel mirror ops (`"issue"` for receipt-cancel
   consume and issue-cancel restore, `"return"` for return-cancel consume)
   and assert stored kinds in tests. No more `kind=''`.
2. F2: lock the reversed issue row (`FOR UPDATE`) in `planReturn` and make
   `sumPriorReturns` reads locking, so concurrent returns serialize and the
   waiter observes the winner. Add a note or test sketch if a deterministic
   concurrency test is impractical here.
3. F3: add tests for quantity-only cancel (exercises the cancel `claimGroup`
   branch) and receipt-cancel / return-cancel success with kind asserts.
4. F4: document the mixed-voucher ordering contract (G6 post before
   quantity-only apply) in the seam comment; fix the stale "not scheduled in
   any C7 slice yet" line to name slice 11.

## Re-review (fix tip `325cca0`, stack `fe222f2` s2 + `325cca0` fix on `6c871c9`)

Re-review verdict: PASS.

Verdict: PASS

- Base check: `git diff a69d8b3 fe222f2 -- internal/stock` is EMPTY — the
  restacked slice-2 base is content-identical to what the first review
  passed over; only the fix commit below was judged fresh. Fix scope is
  `internal/stock/{stock,bins,movements_test}.go` only (+220/−17).
- F1 DONE: the three cancel planners now set `Kind` exactly as prescribed
  (`"issue"` receipt-cancel consume, `"issue"` issue-cancel restore,
  `"return"` return-cancel consume). Stored-kind asserts added:
  `cancelEdgeKinds` helper; issue-cancel `[issue]` (existing test),
  receipt-cancel `[issue]`, return-cancel `[return]`. No `kind=''` path
  remains (grep: every `layerOp` literal now carries `Kind`).
- F2 DONE: `loadStockRow` and both `sumPriorReturns` reads are `FOR UPDATE`
  (verified in diff). Lock analysis: the first return's plan holds the issue
  row lock across plan→apply→commit in the caller's tx, so a concurrent
  second return blocks instead of deciding; once the winner commits, the
  waiter's locking reads bypass the snapshot and see the winner's rows, so
  `ErrOverReturn` holds. `TestConcurrentReturnsSerializeOnLocks` proves a
  second in-flight plan gets lock-wait 1205, not a stale plan decision.
  Characterization note (not blocking): the 1205 in that test fires at the
  pre-existing bin lock, so the test guards in-flight serialization rather
  than discriminating the new issue-row lock; the post-commit stale-snapshot
  case is closed by the locking reads, verified in code. A stronger
  deterministic test (establish txB's snapshot, commit txA's return, txB must
  refuse) is possible and left as an optional follow-up.
- F3 DONE: `TestQuantityOnlyCancelClaimsGroup` exercises the cancel
  `claimGroup` branch (`withGL=false`: 1 cancel group, 0 GL rows, bin
  zeroed); `TestReceiptCancelSuccess` (bin/layer/`consumed`/kind/GL asserts)
  and `TestReturnCancelSuccess` (bin 60/600, residual 60, kind `[return]`,
  GL 600) cover the remaining success paths. Suite is now 19 test funcs
  (5 average + 14 movements).
- F4 DONE: ordering contract in the package comment ("plan all lines, post
  once, then apply all", naming the no-operation-row breakage), and the
  wiring line now names slice 11.
- Nits: replay-locking comment, nil-`Dialect` `duplicate()` guard (both call
  sites), and bin `ItemType` check all done. Skipped nits match the review's
  call exactly per the worker's report: `validDate` strictness (acceptable)
  and mixed-`Valued` guard (slice 5).
- `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet ./...` OK at
  `325cca0` (run in the re-review worktree).
- Reviewer's affected at `325cca0`: GREEN — `ok internal/stock 343s`,
  exit 0, stock-only plan (fix touches `internal/stock` only). First queue
  attempt was stopped cleanly by a suite slot change (`gave-up`, no result);
  fresh re-queue ran to green.
