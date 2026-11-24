# Review: C5 slice 5 — Property 12 party ledger equals control (7.5)

Verdict: PASS

Tree: athena-erp-c5-s5-r, detached at 8468821 (on 43ded7c), clean.
Slice diff (43ded7c..8468821): `internal/posting/settlement_test.go`
(+417) + one rapid seed
`internal/posting/testdata/rapid/TestPartyLedgerEqualsControl/...fail`
(+25) only.

## 1. Property REAL — PASS

- Two genuine rapid properties, `TestPartyLedgerEqualsControl` (base)
  and `TestPartyLedgerEqualsControlFX` (multi-currency), default 100
  iterations each, not examples. Draws: receivable/payable side, 1–2
  invoices, totals, split cut, nonempty settlement subsets, payment
  size, 0–2 settlements, optional reversal; FX adds booked-at and
  settle-date draws over a 120/130/125 rate curve.
- Cutoffs vary: the equation is asserted after EVERY completed
  operation (each invoice, each settlement, the reversal), so each
  iteration checks 1–5 distinct cutoffs depending on its draws —
  exactly what design.md P12 ("at a cutoff") and R7.8 ("FOR ALL
  cutoffs") ask.
- Control figure independently derived: `controlBalance` reads
  `SUM(debit_base)-SUM(credit_base)` from `l_gl` by raw SQL, and
  residuals come from raw SQL over `athena_party_items` — neither
  side goes through the service under test. Invoice legs are valued
  from test draws; payment legs post applied-base-only per the R7.7
  contract, so the equation cross-checks the GL store against the
  party store (it pins report/store consistency; per-item sizing is
  P11's job, and the USD 100 golden fixture lands in slice 7).
- Reversals are real coverage: `equationReverse` unwinds through
  `svc.Reverse` + `engine.Reverse` (incl. FX unwind) and re-asserts.

## 2. Isolation — PASS

- Per-iteration uuid companies (`c12`+tag with own year + full-year
  period); both asserts filter by company. Fresh `testdb` per test
  function; the FX rate curve is written once and read-only after.
- Ran the property filter twice in this worktree: both green
  (`TestPartyLedgerEqualsControl` + `FX` PASS, ~15s each). The
  committed `.fail` seed replays inside those runs and passes; it
  matches repo convention (events, ledger, rules all commit seeds).

## 3. FX exactness — PASS

- Read `priceFX` (`settlement.go:840-899`): products round half-up to
  ledger scale (6dp). Amounts are 2dp and the curve rates (120, 130,
  125) are integral, so every conversion is exact and the Round(6)
  is a no-op — the claim holds. Invoice rate at item `created_at`
  (which the test backdates, then reads back through the same
  `GetRate`), settlement rate at end of posting date; signs verified
  for gain and loss on both sides.
- Fractional-rate dust genuinely out of scope: `athena_party_items`
  (migration 20, `sysmigrations.go:705-728`) has no booked-base
  column, so with fractional rates residual×rate vs control can
  differ by rounding dust that no stored figure pins. This task may
  add no other migration, so a booked-base column cannot land in C5;
  the worker's follow-up note (comms 17:48) is the right handling.

## 4. Gates — PASS

- `affected` green via the suite lock on 8468821 clean tree (reused
  log `1124-173200-w-c1.log`, read, not rerun): `cmd/athena`,
  `internal/posting`, `internal/server`, `internal/stock` all ok.
- `gofmt -l cmd internal` empty, `go build ./...` + `go vet ./...`
  clean. No `.proto` change, so `make generate` has nothing to
  regenerate. No web changes. `dist/.gitkeep` untouched (2-file diff).

## 5. Z2 — PASS

- Delta greps clean for sentinel/correction-window/book-policy
  markers. No reversals, windows, sentinels or books invented: the
  test drives the existing `svc.Reverse`/`engine.Reverse` on the
  named book `main`, all dates inside the open full-year period, and
  reversal dates (08-20, 11-20) land after their settlements —
  never backdated. No new Z2 edge.

## 6. Scope — PASS

- Tests ONLY: `settlement_test.go` (appended at end + two offset
  accounts in `settleSetup`, needed as invoice offsets) and the
  rapid seed. Zero product-code changes.

## Nits (non-blocking, for the worker)

- Reversal always targets the LAST settled voucher; the API allows
  reversing any unreversed settlement (no ordering constraint in
  `ReverseInTx`, and the residual guard only binds in inconsistent
  states, so the equation holds for any order). Drawing the reversal
  target would widen the "any sequence" coverage by one line.
