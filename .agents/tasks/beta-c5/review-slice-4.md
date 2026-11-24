# Review: C5 slice 4 — realised FX + settlement reversal (7.4)

Verdict: PASS

Tree: athena-erp-c5-s4-r, detached at 5c7b7bf, clean. Slice diff
(6bcf848..5c7b7bf): `internal/posting/settlement.go` + `internal/posting/settlement_test.go` only.

## 1. FX — PASS

- Gains/losses post as one balanced plan through the G6 engine
  (`postFX` → `Engine.PostInTx`, `settlement.go:930-969`), under the
  `fx_settlement` voucher identity so group keys never collide with the
  payment's own post. No direct `l_gl` writes.
- Directions: gain → Dr control / Cr gain account; loss → Dr loss account /
  Cr control account. Per-application legs, no netting. Sign flips for
  credit-side items (owing less in base terms is the gain), and the same leg
  structure stays correct on both sides.
- payment==item kept (both amount columns use the payment currency);
  payment/base free (base at the settlement rate, invoice rate at item
  `created_at`, half-up to ledger scale).
- Tests: `TestApplyPostsRealisedGain` (Dr receivable 200 / Cr fx_gain 200 on
  the 130→132 fixture), `TestApplyPostsRealisedLoss` (mirror),
  `TestApplySkipsZeroFX` (no group, base still priced),
  `TestApplyRefusesForeignWithoutFX`.

## 2. Reversals mirror + land current — PASS

- Mirrors keep the same lines with flipped sides (`mirrorPartyRow`,
  `mirrorUnappliedCash`: base and txn amounts swap), flag
  `is_reversal` + `reverses_application_id` / `reversed_row_id`, restore
  residuals (guarded against exceeding original), mirror unapplied cash, and
  unwind each revision's FX post through `Engine.ReverseInTx`.
- All mirrors date at the cancel `PostingDate`, and `strictOpen` requires
  that date's period to be open — never edits, never backdated. Submit rows
  are only read, never updated.
- Tests: `TestReverseMirrorsSettlement` (link, swapped Dr 100, cash mirror,
  `ErrAlreadyReversed` on repeat, `ErrInvalid` on unknown),
  `TestReverseUnwindsFX` (submit+cancel nets to zero over 4 rows),
  `TestReverseRefusesClosedDate`, `TestReverseAcrossRevisions` (2 revisions,
  2 FX cancels). A reviewer live-date probe was queued but stood down as
  redundant: mirror dating at the cancel date is unconditional in code and
  the closed-date refusal is tested.

## 3. Closes + deferred Z2 edges — PASS

- Soft-closed: refuses naming `ledger:close` + reason unless `CloseAuthority`
  allows; hard-closed refuses everything even with authority.
  Settlement-level gate runs in both `ApplyInTx` and `ReverseInTx`;
  unknown books and uncovered dates refuse. Covered by
  `TestApplyRespectsCloses` (6 subtests, incl. refused-FX-writes-nothing).
- The 4 deferred edges are genuine 7.6/engine scope, none a slice-4 gap:
  1. `ledger:close` permission + gate + adjustment audit → slice 6 owns
     `gate.go`/server; settlement takes the verified `CloseAuthority`.
  2. Statutory-book naming → needs a books master; no migration allowed.
  3. FX-into-soft → engine takes no adjustment authority (`PostOptions`
     has none); worker refuses conservatively with decided wording.
  4. Control-source existence check → needs an engine hook; `posting.go`
     is out of slice scope. FX control legs already carry
     `SourceType: "party_item"` + item id, satisfying the engine's check.

## 4. No sentinels — PASS

- New code raises only `ErrInvalid` wraps with Z2-policy wording; no
  `ErrCorrectionWindowOpen`/`failed_precondition`-style behavior.
  `postFX` preempts the engine's stale soft-close wording. The engine's own
  sentinels are untouched (engine out of scope) and unreachable from
  settlement (FX and reversals only enter open periods). Unnamed books
  refused (book is a required field); defined-book check in place.

## 5. Gates — PASS

- `affected` green, all 8 packages (cmd/athena, expr, ledger, posting,
  rules, server, stock, types). `gofmt -l cmd internal` empty,
  `go build ./...` + `go vet` clean. No `.proto` changes, so `make
  generate` has nothing to regenerate (stable).

## 6. Scope — PASS

- Slice diff is `settlement.go` + `settlement_test.go` only. No engine,
  server, proto, gate, CLI or Desk changes.

## Nits (non-blocking, for the worker)

- `settlement.go:1012`: comment "Reversal mirrors arrive in 7.4" is now
  stale — they arrived in this slice.
- `settlement.go:815`: "The three amounts agree in 7.2" — `priceFX` now
  reprices `BaseAmount`; reword when nearby.
