# Review: C4 slice 4 — Property 25 (13.4)

Verdict: PASS

Commit: f615096 (review tree `athena-erp-c4-r`, detached, clean on
arrival and at every gate; no edits made by the reviewer). Delta
`3175005...HEAD` is exactly the briefed 2 files, tests only:
NEW `internal/ledger/ledgers_test.go` (+161) and an append to
`internal/types/types_test.go` (+52).

## Blockers

None.

## Follow-ups (do not block)

1. Two-leg `ConvertQuantity` bound still unpinned (carries slice-3
   follow-up 4). The only `ConvertQuantity` call in the delta is the
   cross-family rejection (`internal/ledger/ledgers_test.go:157`); no
   same-family A-to-B conversion pins the composed two-rounding
   bound. Per this brief's item 2 this blocks only with
   clean-values-only tests, and these tests are genuinely
   randomized — each leg's rounding IS pinned (randomized row
   scales 0–6 and both modes through the same `ToBase`/`FromBase`
   code the two-leg path uses), so the residual gap is just the
   composed formula. Suggested pin for a later slice, with row
   scales sA (unit A) and sB (unit B):
   `|out − q·fA/fB| ≤ 0.5·10^−sB + 0.5·10^−sA/fB`.
2. Comment precision (`internal/ledger/ledgers_test.go:24-25`):
   "Periods are the ledger's effective-dated rows; the same
   ... contract binds effective-dated Record Types (R13.2)" is
   true only at the contract-shape level (reject overlap +
   single as-of row). Fiscal periods are inclusive `[start,end]`
   with both dates required — pre-existing design
   (`internal/ledger/periods.go:229-231`, `:323`, untouched by
   C4) — while slice-2/ADR-0029 effective ranges are
   end-exclusive and NULL-open. Say that in the comment so no
   reader takes the period property for a proof of record
   effective-dating. Determination end-exclusive boundaries and
   open ranges stay example-tested from slice 3, not
   property-tested; accepted, since this brief's item 1 blesses
   period cases.
3. Nit (`internal/ledger/ledgers_test.go:43`):
   `for j := 0; j < rapid.IntRange(1, 6).Draw(t, "periods"); j++`
   redraws the bound on every condition check. Harmless
   (terminates, yields 1–6 periods) but draws extra values and
   obscures shrinking; hoist the draw above the loop if touched.

## Checked per the brief

1. Real properties, not example lists. All three tests use
   `rapid.Check` (100 iterations each, observed in the rerun
   logs; automatic shrinking) with meaningful generators:
   period starts 0–60, lengths 0–20, queries −5–85 over fresh
   companies; factor unscaled 1–999999 at scale 0–6, entered
   ±99999999 at scale 0–8, row scale 0–6, both rounding modes.
   Feature tags present (`ledgers_test.go:27,96`,
   `types_test.go:320`). Contents match the brief: period
   range-reject + as-of (`TestLedgerEffectiveRangesProperty`),
   DB-backed UoM round trip (`TestUoMRoundTripProperty`), pure
   `Convert`/`FromBase` bound + exactness
   (`TestQuantityConversionRoundTripProperty`).
   The asserted bound `f·|back−entered| ≤ 0.5·10^−s·(1+f)` is
   the exact composed half-ulp bound: `Convert` is one exact
   `Mul` plus `Round(s)` (`quantity.go:62-74`), `FromBase`
   is one `Div(f,s)` (`quantity.go:80-92`), so
   `back−q = e2 + e1/f` with each `|e| ≤ 0.5·10^−s` under
   both modes (`decimal.go:233-249`). The exactness guard
   (`q·f` rescales to `s` and `q.Scale() ≤ s` implies
   `back == q`) is sound: both roundings are then exact and
   `Equal` is scale-insensitive (`decimal.go:180-181`).
2. Two-leg bound: missing (see follow-up 1); not a blocker per
   the item-2 condition and beta design §5 (a tests-only delta
   missing one extra pin breaks no behavior).
3. Range semantics: the oracle matches the period
   implementation exactly (`share` ≡ `Overlaps`,
   `periods.go:229-230`; as-of ≡ `PeriodFor`, `periods.go:323`;
   "overlaps" ≡ `periods.go:221`; gap ≡ `ErrNotFound`,
   `periods.go:327`). End-exclusive/NULL-open cannot hold for
   periods (both dates required, `periods.go:158-160`); the
   tested touching-bounds direction (adjacent days disjoint)
   holds and is exercised (sim below). Slice-2 semantics
   themselves are untouched.
4. Strength, judged by simulation plus implementation match. A
   uniform-draw simulation of the generators (conservative:
   rapid's small-bias clusters more) over 20k iterations gives
   per-iteration rates: overlap pair 60%, adjacent pair 5.5%,
   same-day touch 13%, zero-length single-day period 15%,
   as-of query exactly on a boundary 5%. Over 100 iterations
   each boundary direction is hit several times, so an
   off-by-one in `Overlaps` or `PeriodFor` in either direction
   fails loudly. Rounding: the bound is tight half-ulp over
   randomized scales/modes (truncation or a wrong scale
   violates it), and the exactness case fires ~21% of pure
   iterations (200k sim; rounding occurs in ~79%), so
   spurious rounding is pinned too. Cross-family leak: the
   `ErrCrossFamily` assertion runs every DB iteration over
   fresh families (`ledgers_test.go:157`). All three tests
   reran green fresh (below).
5. Confirmed tests-only: `git diff --name-only 3175005...HEAD`
   lists only the two `_test.go` files. `types_test.go` has
   two hunks — the append plus a 2-line purely additive
   `rapid` import (`@@ -9,6 +9,8 @@`, no existing line
   touched; unavoidable since the file previously imported
   only `testing/quick`). No proto/gate/migration/web/cmd
   changes; Z2/Z4 untouched (no ADR/log/posting changes).
   Slice-3b's reservation (`ledgers_test.go` owned by slice 4)
   respected.
6. Gates, tree clean throughout: `gofmt -l cmd internal`
   empty; `go build ./...` ok; `go vet ./...` ok (full);
   `affected` (absolute main-repo path) reused the worker's
   green run on f615096 — same command, clean tree, 7/7 `ok`,
   no FAIL/panic/SKIP (`suite-logs/1124-023302-w-c4.log`;
   ledger 224s + server 365s ran fresh, rest go-cache valid);
   fresh direct reruns with `-count=1`: pure types property
   PASS (100 rapid tests, 0.46s), both ledger properties PASS
   (100 rapid tests each, 62.7s, MariaDB, not skipped);
   `make generate` twice with no diff, tree clean after. No
   browser pass (no Desk in slice).
