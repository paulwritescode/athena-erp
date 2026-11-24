# Review: C7 slice-5 FIX2 (pin same-bin returns)

Verdict: PASS

Stylus ruling (a): counter-proposal ACCEPTED — same-bin returns are the
chosen design (return elsewhere is return+transfer); fix2's refusal pin +
moved-away guard is the right test. HOLD lifted, verdict delivered.

Tree: `../athena-erp-c7-s5-r`, detached at `1b1ce5c`, clean.
Fix2 delta (`1b1ce5c^..1b1ce5c`): `internal/stock/tracking_test.go` only,
+31, test-only. In scope.

## Pushback verification: the worker is right, my scenario is unpostable

I verified the enforcement claim in the code myself, not on the worker's word:

- `planReturn` (`internal/stock/stock.go:458`, doc comment at :444-448):
  `if orig.Bin != p.Bin || orig.ItemType != m.ItemType` → `ErrInvalid`
  ("return bin differs from the issue's bin"). `binKey` is company, book,
  owner, item, location, lot, serial — so a return at another location,
  owner, lot, or serial than its issue is refused at plan time.
- Provenance (blame): slice-2 commit `1f6ae9d` ("stock ledger core with
  weighted average"), predating slice 5. The enforcement is not new; my
  demanded third-location return was unpostable in the code I reviewed.
- No bypass: the issue check (`:455`) runs first, so `ReversesRowID` cannot
  point at a non-issue; cancel mirrors cannot smuggle a cross-bin return
  because `ReturnSerials` requires `Reversal == false`, so only
  `planReturn`-planned rows (which passed the same-bin check) ever qualify.

Correction to my fix1 review: given the same-bin invariant, return-row
owner/location always equal the issue's, so the pre-fix1
`ReverseReturnSerials` was already exact and fix1's location restoration is
behavior-preserving defense-in-depth. Fix1's moved-away guard remains the
real protection (transfer-after-return IS postable: `ApplyTransfer` moves an
`on_hand` unit, and fix1's test part 2 proves the cancel then blocks). My
finding-2 premise was wrong; the worker's pushback stands.

## Fix2 test: real

`TestReturnRequiresIssueBin` posts a receipt, issues 10, then plans two
returns through `PlanMovement` — one at third location `locC`, one to owner
`consignment` — and asserts `ErrInvalid` for both. It exercises the
production seam and pins the invariant fix1's exactness rests on. (It would
fail if the `:458` check were ever removed.)

## Gates

- `gofmt -l` empty, `go vet ./internal/stock/` clean (this tree).
- `affected` (stock scope): GREEN via w-c7's gate run on `bf8aaf1`
  (branch tip: fix2 + slices 6–7 after a rebase; my `1b1ce5c` ticket was
  stopped per the dedup order and the branch moved on). Suite record
  `1124-193546-w-c7.log`: `ok athena-erp/internal/stock 299.093s`, exit 0,
  clean tree. All four fix files (`reserve.go`, `tracking.go` + tests) are
  byte-identical between reviewed `1b1ce5c` and gate head `bf8aaf1`; the
  stock delta between them is purely additive (`check.go`, `check_test.go`,
  +237 in `stock_test.go`, zero deletions), and the pin test has no skip
  path — so the green package run proves `TestReturnRequiresIssueBin`
  passes and fix2 breaks nothing. Same-commit rerun would only re-prove
  this after a ~2h queue (w-c7 waited 7539s).
