# Review: C7 slice 5 reservations + tracking + counts (11.5)

Verdict: NEEDS_CHANGES

Tree: `../athena-erp-c7-s5-r`, detached at `5153252` (parent `8e1cbe8` slice 4, base `8b0efdf`), clean.
Slice-5-only diff (`8e1cbe8..5153252`, 7 files, +2070/−7): new
`internal/stock/{reserve,tracking,count}.go` + `reserve_test.go`, `tracking_test.go`,
`count_test.go`, plus a 14-line `stock.go` refactor. (`stock_test.go` in the
wider diff belongs to slice 4, not this slice.)

## What passes

1. Reservations: locked takes are real. `Reserve` takes the bin lock, then
   `sumActiveHolds ... FOR UPDATE`, then the serial lock — the same
   reservation-before-serial order as `closeReservation`, so no lock-order
   inversion. `TestCompetingTakesSerializeOnBinLock` is a genuine two-connection
   test (second take gets InnoDB 1205, not an availability decision).
   Release (partial + full), consume-with-issue-in-tx, and expiry (one of two,
   cutoff-correct) are all exercised in `TestReleaseConsumeExpireLifecycle`;
   over-hold and duplicate-hold refusals are tested.
2. Tracking: serial lifecycle receipt→issue→return is tested end to end;
   `TestSerialCancelReversals` verifies the full status mirror
   (return-cancel→issued, issue-cancel→on_hand with link cleared,
   receipt-cancel→rows deleted). FEFO ordering is verified with a real test
   (L2 50 before L1 70, no-expiry L3 last, shortfall 50 on over-pick).
   Transfers preserve company value (GL stock unchanged at 1000, clearing nets
   zero on top of the receipt's −1000, quarantine bin 30 @ 300) and move serials.
3. Counts: cutoff-adjusted expected (100 on the 15th stays a no-op after a
   16th issue), shrinkage at average, gain at average, gain-into-empty at zero,
   zero-delta writes no `l_stock` row and claims no group. All tested.
4. Rentals tracked-not-consumed: `TestRentalTrackedNotConsumed` is a real e2e
   (unvalued receipt → register → event hold → reserved → on_rent with hold
   auto-consumed → return → on_hand), asserting bin value 0, zero `l_gl` rows
   and zero layers. Not a stub.
5. Z3: `average.go` untouched (weighted average intact); FEFO is picking order
   only; negative stock refusal (`ErrNegativeStock`) lives in slice-2
   `PlanMovement`, which this slice only routes through (transfers, counts,
   serial issues all plan through it). Over-hold refusal spot-checked via test.
6. Gates: `affected` green on `5153252` — reused worker run
   `1124-144319-w-c7.log` (`ok athena-erp/internal/stock 185.808s`, real DB run,
   per one-green-run-per-commit); `gofmt -l` empty, `go build ./...` and
   `go vet ./internal/stock/` clean; no proto change (no `proto/`, `gate.go`,
   server, CLI or Desk files in the diff; no migration edit).

## Required changes

1. Re-hold after release is impossible, contradicting the code's own error text.
   `uq_reservations_hold` (migration 21, merged — cannot change) has no status
   column, `closeReservation` never deletes rows (no `DELETE FROM
   athena_reservations` anywhere), and `Reserve` maps any duplicate to
   `ErrDuplicateHold`, whose text promises "release first, then hold again".
   So a cart/event that releases and re-adds the same bin grain gets
   `ErrDuplicateHold` forever. Fix code-only in `reserve.go`: on duplicate,
   revive the conflicting row when it is terminal (released/consumed/expired)
   back to active with the new qty/expiry, or delete-then-insert — and add a
   release-then-hold-again test. (No test covers re-hold today; the duplicate
   test only covers hold-then-hold.)
2. Cancel mirrors are not exact on owner/location.
   `ReverseReturnSerials` restores units with the *return* row's owner/location
   (`row.Bin`, tracking.go ~L298) instead of the *reversed issue's*, and
   `ReverseIssueSerials` never restores location at all — so after a full
   unwind at differing locations the registry disagrees with where the receipt
   put the units. Load the issue row (already loaded for attribution in
   `returnSerials`) and restore its owner/location. Add a cross-location
   return→cancel test (today every location in the cancel test is `wh_main`,
   so this cannot fail).

## Notes (not verdict drivers)

- `stock.go` (14 lines) re-signatures `checkMasters` from `Movement` to three
  strings so `count.go` reuses it; the single old caller is updated. Strictly
  outside the 3-new-files scope, but behavior-neutral, minimal, inside the
  worker's owned package, and the alternative is a duplicated validator.
  Recommend master accept; flagged for the record.
- `ExpireReservations` returns `len(ids)` but `continue`s past state errors;
  unreachable within one transaction (rows were just locked active). Trivial;
  may ignore or count only flipped rows.
