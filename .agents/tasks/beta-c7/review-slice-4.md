# Review: C7 slice 4 Property 21 (11.4)

Verdict: PASS

Tree: athena-erp-c7-s4-r detached at f68cf1d, clean (`git status` empty).
Delta reviewed: 44f5ed5..f68cf1d only = 1 new file, `internal/stock/stock_test.go`
(+302/-0, `create mode`). No product-code changes, no proto, no other lane's files.

## Item 1 — Property REAL, distributions cover the brief

Generator (per rapid iteration, fresh item/voucher/operation ids, one shared DB):
- Histories of 1-8 movements: receipt 45% (forced when onHand==0), issue 30%
  (qty uniform 1..onHand), return 25% against an open issue with qty capped by
  its unreturned balance (fallback receipt when no issue is open). Matches the
  `stock_return`/over-return semantics pinned in slice-2 tests.
- Backdating: movement and posting dates drawn independently from all 12 months
  of 2026, so P(months differ)=11/12 per movement, both directions, plus
  out-of-order arrival across movements; delta posting dates drawn independently
  of receipt economic dates.
- Returns: reachable in most multi-movement runs (30% issue x 25% return
  branches over up to 8 movements).
- Fully-sold: fully-consumed receipt layers are common (issues span layers
  oldest-first, so any cumulative issue past an early receipt's qty consumes
  it); the all-sold split is also pinned deterministically by
  `TestDeltaFullySoldSkipsStockLeg`.
- Two delta batches x 0-3 deltas on uniformly random receipts, totals
  -2000..+5000c (negatives ~29%, zero handled: Plan skips legs, Apply claims
  its own group — confirmed in `delta.go`). ~94% of runs post >=1 delta.

Assertions, exactly the briefed three:
- Every historical `l_stock` row byte-identical after each batch. I verified the
  snapshot SELECT covers all 26 data columns of migration-21 `l_stock`
  (id..created_at incl. both dates, dimensions, group, reversal fields) — no
  blind spot where a rewrite could hide.
- Bin value moves exactly by the summed planned on-hand portions.
- Voucher-scoped GL stock (`l_gl` account `stock` over exactly this iteration's
  vouchers) moves exactly by the same sum. Fresh UUIDs per iteration plus
  item/voucher-scoped reads rule out cross-iteration contamination.
- Runs meaningful: 100 rapid iterations of real MariaDB txns
  (1-8 movements + up to 6 deltas each), not example lists.

## Item 2 — Tests green, verified myself

- Direct: `go test ./internal/stock/ -run TestAppendOnlyCostCorrectionProperty
  -count=1 -v` -> `[rapid] OK, passed 100 tests (6.16s)`, PASS in 14.63s.
  Worker's 100/100 claim confirmed.
- `affected` dedup-reused the green run on clean f68cf1d
  (1124-101210-w-c7.log, `ok athena-erp/internal/stock 308.623s`); I read the
  log: zero FAILs.
- `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet` on stock OK.
  No proto change, so `make generate` trivially stable; no web files, so no
  typecheck/build needed. design.md section 5 checks satisfied for this slice.

## Item 3 — Scope: clean

Only the new `internal/stock/stock_test.go`. Helpers (`setup`,
`seedStockBasics`, `testMovement`, `issueOf`, `testDelta`) all come from
prior-slice files, untouched here. 11.4 asked to create exactly this file.

## Follow-ups for master (not slice-4 fails)

1. ~6% of runs draw zero deltas in both batches and assert trivially; harmless
   at 100 iterations, could force >=1 delta total if ever tightened.
2. `equalRows` failure prints only row counts, not the differing rows — a
   debugging nit (rapid shrinks/replays anyway).
3. The sold portion is pinned only indirectly (exact bin/GL-stock equality +
   engine balance validation); a direct scoped-COGS assertion would strengthen
   the "new cost appears only in appended delta rows" half. P22 (slice 8)
   covers GL-side equality anyway.
