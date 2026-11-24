# Review: C7 slice 3 cost deltas (11.3)

Verdict: PASS

Tree: athena-erp-c7-s3-r detached at 110d9d8, clean. Delta reviewed: 325cca0..110d9d8 only
(4 files, all `internal/stock/`: `bins.go`, `delta.go` new, `delta_test.go` new, `stock.go`).
No proto change, no other lane's files. `bins.go`/`stock.go` edits are the briefed
rebuild-watermark and delta-cancel-mirror wiring, not scope creep.

## Item 1 — Deltas: correct

- Split: `splitDelta` divides by layer residual/original with exact edge cases
  (full residual -> all on hand; zero residual -> all sold; else round6 + remainder
  to sold so onHand+sold == total exactly). I checked rounding cannot flip a
  portion's sign (round error <= 0.5e-6 < any nonzero total step), so legs keep
  their sides.
- Legs: on-hand debits stock control, sold debits COGS, offset takes the other
  side; negative totals flip every side; zero portions post no leg. Every
  non-zero plan goes through `posting.ValidatePlan`, so balance/periods/dimensions
  are engine-checked, not just eyeballed.
- Rows: `ApplyCostDelta` writes `l_stock_value` with the receipt's movement date
  as economic date and the validated posting date; replay-safe via
  `findDeltaRow` in both Plan and Apply.
- Layers UNTOUCHED: delta paths only call `layerByReceipt` (read). No layer
  insert/update/restore in `delta.go`, `insertDeltaMirror`, or the ApplyCancel
  delta loop. Layer-cost assertions in tests pin this.
- Delta cancel: mirrors negate both splits, carry receipt economic date + cancel
  posting date, `is_reversal=1`, unwind the receipt's bin by negated on-hand,
  layers untouched; GL reverses via the seam's `ReverseInTx` (confirmed in test:
  GL stock returns to 1000).

## Item 2 — Rebuild: correct, crash-safe

- `RebuildBins` walks `l_stock` id order in 500-row chunks, each chunk committing
  bin rewrites + watermark in one tx; resume restarts from the committed
  watermark. Each grain is recomputed from its FULL row history (overwrite, not
  accumulate), so resume/overlap is idempotent: no double-apply, no skip.
  In-tx callers run single-shot, documented.
- `PendingRebuild` (any `running` row) is a sane close predicate; close-path
  wiring is slice 11's per the brief — noted, not demanded.
- Done-window (writer commits between last empty chunk and finish) is benign:
  writers maintain their own bins in-tx.

## Item 3 — Tests real, green

- 7 tests, all with concrete asserts: 180/120 split + dates + GL triple,
  fully-sold 2-leg, credit flip, delta cancel mirror + GL, watermark resume from
  midpoint (pre-watermark bin untouched at 999, resumed bin fixed, state
  completed, pending cleared), empty pending/rebuild, 8-case validation table.
- `affected` reuses worker's green `internal/stock` run on clean 110d9d8
  (1124-085015-w-c7.log, `ok ... 132s` — real DB, not skipped); I additionally
  ran the 7 delta tests verbosely myself: 7/7 PASS (~30s each, MariaDB).
- `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet` on stock OK.

## Item 4 — Scope: clean

Only `internal/stock/{bins,delta,delta_test,stock}.go`. No proto, gate, CLI, or
Desk files.

## Follow-ups for master (not slice-3 fails)

1. Void guards (recommend briefing before slice 11 wires cancel): voiding a
   delta voucher AFTER the receipt layer moved strands value (bin goes
   (0,-180)-shaped; COGS stays overstated because the intervening issues priced
   off the delta-inflated average and only the delta voucher's legs reverse),
   and cancelling a receipt while deltas reference it strands the on-hand
   portion in a zero-qty bin, polluting the next receipt's average via
   `ReceiptAverage`. Slice 2 blocks receipt-void on downstream consumption;
   delta paths have no analogous guard. Suggested shape: block delta cancel
   unless the split recomputed from the current layer equals the stored split
   (no schema change); decide receipt-with-deltas (block vs unwind).
   No briefed property is violated today (P22 holds on both sides; P10 holds for
   immediate cancel), so this is a design decision, not a defect in the briefed
   behavior.
2. Rebuild comment overclaims: "writers interleave safely because each chunk
   locks the bins it rewrites" — under REPEATABLE READ a writer committing
   between the chunk's grain snapshot and its bin overwrite would be dropped
   from that bin until the next rebuild (narrow race; slice-7 check would flag
   it). Consider locking reads in `aggregateGrain`, READ COMMITTED chunk txs,
   or documenting no-concurrent-writers during rebuild.
3. Nits: `aggregateGrain` reservation join lacks the COALESCE the `l_stock`
   side uses (moot until slice 5 writes reservations); credit-flip test pins
   the stock side but not the clearing debit; resume test assumes UUIDv7
   time-order across two posts (same-ms inversion theoretically flaky);
   zero-total deltas skip `ValidatePlan` (posting-date/dimensions format-only).
