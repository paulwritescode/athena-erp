# Review: C5 slice 3 Property 11 allocation sums (7.3)

Verdict: PASS

Tree: athena-erp-c5-s3-r, detached at 6a879f4, clean (`git status` empty at start).
Base note: the brief names base 9b91f8d, but that commit is NOT an ancestor of 6a879f4 — slice 2 was rebased (9b91f8d → 167c5af) and the 9b91f8d..6a879f4 range now also spans unrelated main-line commits (F6 ASCII logo, night record). Reviewed the actual slice-3 commit 6a879f4 (`test(c5): Property 11 allocation sums`) instead: exactly `internal/types/types_test.go` (+100/−0) and `internal/posting/settlement_test.go` (+171/−0), 271 insertions, 0 deletions.

## 1. Property REAL (Property 11: Allocation sums)

Spec (batch-2 design.md): "For any payment amount and any instalment set or weight set, the allocated parts sum exactly to the amount and are identical under any display ordering of rows with stable IDs."

- Splitter (`TestAllocateProperty`, types_test.go): genuine rapid property, not examples. n 1..6, weights 0..100, scale 0..6, totals ±100000. Pins exact sum, repeat-call determinism, zero-weight-takes-zero, and lowest-index tie-break (even weights: spread ≤ 1 unit, head never smaller than any later share — catches a reversed tie-break). Verified the tie-break math against `AllocateUnits` (stable remainder sort, negate-after for negatives; the test's sign flip-back is sound). Zero-weight shares can never take a leftover unit (leftover count is always below the positive-remainder count), so that assertion holds by construction.
- Settlement (`TestSettlementAllocationProperty`, settlement_test.go): genuine rapid property over 1..4 instalments (residuals 1..2000 minor units), payment 1..8000, both sides (receivable/payable), random listing order via drawn shuffle keys. Pins applied + unapplied == payment exactly, per-instalment shares equal to the id-ordered model (`Allocate` at scale 6, capped at residual, zero parts dropped), stored residuals, and party-row debit/credit sides. Model checked line-by-line against `allocateLine`/`weightUnits` in settlement.go: id sort (line 588), weight = residual×10000 at scale 6, scale-6 split, cap + zero-skip all match.
- Runs: rapid default 100 checks each (confirmed in output: `[rapid] OK, passed 100 tests`); random seeds, built-in shrinking, no custom shrink code to judge. Ranges are small enough that proportional remainders and tie-breaks occur often, as the comment claims. The all-zero-weights early return is a negligible skip (probability ~101^-n).

## 2. APPEND-ONLY (types_test.go)

Single hunk `@@ -359,3 +359,103 @@` at end of file, 100 insertions, 0 deletions, no import-block touch. The appended code needs `rapid`, `strings`, `math/big`, `usd` — all already present (lines 5, 8, 13, 17). L3's share of the file is untouched. PASS.

## 3. Tests green

- `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet` on types + posting OK.
- Narrow direct run (allowed): `TestAllocateProperty` PASS, 100 rapid checks.
- `affected` green through the suite lock on 6a879f4: `ok cmd/athena 439s`, `ok internal/expr`, `ok internal/ledger 445s`, `ok internal/posting 850s`, `ok internal/rules`, `ok internal/server 820s`, `ok internal/types`, exit 0. Plan covered types + posting + all importers; no web changes.

## 4. Z2

Zero matches in the delta for revers*/cancel/window/sentinel/failed_precondition/codes.*book-policy/default-book. `Book: "main"` appears twice but only as required request-plumbing values, identical to the slice-2 baseline helper (`settleItem`, also `Book: "main"` twice) — no window, day count, policy or default-book invention. Slice 4 owns reversals. PASS.

## 5. Scope

Two test files only; no product-code, proto, gate, CLI or docs changes in 6a879f4. The settlement_test.go import additions (`sort`, `uuid`, `rapid`) are in C5's own file, not the L3-shared one. PASS.

Review worktree `athena-erp-c5-s3-r` left in place for master to remove.
