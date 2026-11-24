# Review: C7 slice 6 — Property 23 availability (r-c7s6)

Tree: `/Users/iampaulking/projects/duchess/athena-erp-c7-s6-r`, detached at `e5007d4`, verified clean before and after.
Scope reviewed: ONLY `6871fc8..e5007d4` (the Property 23 addition; slice-5 fix below it is r-c7s5's).

Verdict: PASS

## 1. Property REAL — yes

`TestAvailabilityProperty` (`internal/stock/stock_test.go`, +237 lines) is a genuine rapid
property, not examples: funding (50–100), attempt count, per-attempt location/holder/qty
(1–60), lifecycle-op choice, partial qty, and expiry (none/past/future) are all drawn.
The model is independent int64 arithmetic — `onHand` plus a per-location holds map, with
`reserved` recomputed by local summation — and asserts `Available` (on-hand, reserved,
available) after every take, release, consume and expiry sweep. The model never calls
stock code to build expectations. My direct run: `[rapid] OK, passed 100 tests`.

## 2. Edges — all pinned by real assertions, verified by reading

- Over-hold refused: expects `ErrInsufficientAvailable`. Precedence matches the
  implementation — `Reserve` checks availability (`reserve.go:113`) BEFORE duplicate
  detection (INSERT → `resurrectHold`, `:130–165`), so the test's over-first switch is
  correct, not flaky.
- Duplicates rejected: expects `ErrDuplicateHold` when the holder is active in the model;
  model/DB active sets stay in sync (full close and expiry remove from both; resurrect
  only fires for terminal rows, i.e. model-absent holders).
- Expiry statuses pinned: per-row `SELECT status` asserts `expired` iff past-expiry,
  `active` otherwise; model expiry predicate (`!= "" && < cutoff`) matches the sweep
  query (`expires_at IS NOT NULL AND expires_at < ?`). The freed count is deliberately
  NOT asserted (workspace-wide sweep), with a comment saying why.
- Final DB invariant: raw-SQL `SUM(active holds) <= on_hand` per location — independent
  of the `Available` code path.
- Consume/release model fidelity: verified `closeReservation` only drops `reserved_qty`
  (on-hand untouched; "the caller posts the issue movement") — the model's constant
  on-hand mirrors the documented seam contract (wiring is slice 11's job), not a vacuous
  pass.

## 3. Carry claim — verified, plus my own 100/100

- `git diff d32eb9f..e5007d4 -- internal/stock/` is EMPTY: byte-identical rebase, as claimed.
- The 12 files that do differ are main-side only (F7 `cmd/athena/doctor*`, README/tasks
  docs, `.agents` logs) — none imported by `internal/stock`, no `go.mod`/proto change.
- Worker's gate record exists in `suite.jsonl`: affected `go test ./internal/stock` on
  clean `d32eb9f`, exit 0, 196s. Reuse is sound; no re-gate needed.
- My own direct run in the review tree: `TestAvailabilityProperty` PASS, 100/100
  (`[rapid] OK, passed 100 tests`, 21.8s).

## 4. Gates

- `gofmt -l cmd internal`: empty. `go build ./...` + `go vet ./internal/stock/`: clean.
- No proto change (delta is `stock_test.go` only, so no `make generate` concern).
- Affected reuses the worker's green run on identical content per §3.

## 5. Scope

Delta is `internal/stock/stock_test.go` +237, zero product-code changes. Nothing else in scope.

Skipped: nothing. No section-Z questions touched.
