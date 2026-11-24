# Review: C7 slice-5 FIX (re-hold revive + cancel-location exactness)

Verdict: NEEDS_CHANGES

Tree: `../athena-erp-c7-s5-r`, detached at `5796837`, clean.
Fix delta (`5796837^..5796837`, 4 files, +210/−35): `reserve.go`,
`tracking.go` + their tests only. In scope; no migration, proto, gate,
server, CLI or Desk changes.

## Finding 1 (re-hold): FIXED and proven

- `resurrectHold` reactivates the terminal row for the same holder+grain
  (SELECT by the exact unique key `FOR UPDATE`; active row stays
  `ErrDuplicateHold`; terminal row flips to active with new uom/qty/expiry
  and a revision bump, same id). Race-safe: concurrent revives serialize on
  the row lock, loser sees active. Availability accounting is right: the
  terminal row is excluded from `sumActiveHolds`, and the bin increment runs
  on both the insert and revive paths. Serial compensation (`unholdSerial`)
  preserved on the error path.
- `TestReholdAfterReleaseReactivates` is real and discriminating: hold 10 →
  release → re-hold 15 with expiry asserts the same id revived, qty 15,
  reserved 15. Would fail on the old code (duplicate forever).
- Bonus: `ExpireReservations` now returns the flipped count (my note D). Good.

## Finding 2 (cancel location): code fixed, proof vacuous — one test amendment needed

- The implementation is correct by reading: `ReverseReturnSerials` restores
  the reversed *issue's* owner/location (via `returnSerials`, which now
  returns the issue row), `ReverseIssueSerials` loads the issue row and
  restores its owner/location too, and the new moved-away guard blocks a
  return-cancel when the unit no longer sits where the return put it instead
  of silently rewriting it. Sensible and exact.
- But `TestCrossLocationReturnCancelRestoresIssueBin` does not prove the
  restoration: the return posts at `locB`, the same location as the issue,
  so the old code (return row's location) yields the identical `locB` and the
  test passes on old and new code alike. Only the second half (moved-away
  unit → `ErrCancelBlocked`) discriminates, and it covers the new guard, not
  the location restoration the fix is about.
- Required: post the return at a third location (e.g. keep `back.Location`
  at the receipt's `wh_main` instead of setting `locB`), then assert the
  cancel restores `locB`. Two-line change; fails on old code, passes on new.

## Gates

- `affected` (main-repo path, suite lock): `ok athena-erp/internal/stock
  233.831s` on `5796837`, fresh run, exit 0.
- `gofmt -l` empty, `go build ./...`, `go vet ./internal/stock/` clean.

## Note (not a blocker)

- `resurrectHold` inherits the unique key's grain, which excludes owner: a
  repeat hold as a *different* owner revives the first row and keeps its
  stored owner. Pre-existing slice-1 schema grain (the second owner could not
  even insert before this fix), orthogonal to this finding; flagging for a
  future slice, not this one.
