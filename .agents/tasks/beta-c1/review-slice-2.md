# Review: C1 slice 2, Operation_ID contract + idempotent replay (6.4+6.5)

Branch: `feat/c1-posting` in `../athena-erp-c1`, commit `17e10ab` on top of
merged slice 1 (reviewed diff: `git diff 35061c0...HEAD`).
Scope: slice 2 only (batch-2 G6.4+G6.5) against batch-2 `design.md` G6,
requirements R6.7–R6.10, ADR-0023, and the still-binding Z2 rule.

Verdict (round 1): NEEDS_CHANGES

One blocker: a cross-actor operation-id reuse returns the wrong voucher's
result nondeterministically (wrong replay semantics). Everything else in
the slice passes, including both rapid properties unskipped, the deadlock
reasoning, Z2, and all gates.

## Blocker

**B1. Group-collision winner lookup ignores the actor namespace, so a
replay can return another voucher's result.**
`internal/posting/operations.go:137` in `replayGroupResult`:

```go
SELECT actor, payload_hash, result_json FROM athena_operations
WHERE command = ? AND operation_id = ? LIMIT 1
```

The operations key is `(actor, command, operation_id)` — the schema and
the `CommandPost` comment both treat the id as namespaced per actor, so
two actors may legitimately hold rows under one id string. But the group
row carries only `operation_id` (no actor), and this lookup filters only
`(command, operation_id)` with a bare `LIMIT 1` and no `ORDER BY`. When
two such rows exist, the returned row is whichever InnoDB scans first.

Concrete trigger: alice posts voucher V rev 1 with op id `"op-1"`;
carol posts voucher W rev 1 with op id `"op-1"` (a different key —
allowed); bob, who never used `"op-1"`, posts V rev 1 with `"op-1"`.
Bob's step-4 lookup misses, his group insert dups, and the winner
lookup may return carol's row. Then `winnerActor (carol) != bob`
skips the conflict check and bob gets carol's result for W — wrong
`PostingGroupID`/`RowIDs` for his post, nondeterministically. The
ledgers stay correct (post-once holds); the returned result is wrong,
which is the "wrong replay semantics" blocker class. (Two actors alone
cannot trigger it: the second actor's own row is either found at step 4
→ correct conflict/replay — or absent with only the winner's row
present. It takes a third same-string row, reachable as soon as any
caller uses human-readable ids; nothing enforces uuids.)

Suggested fix (code-only, no migration): in `replayGroupResult`, also
`SELECT` the group row's `id`, fetch all op rows for
`(command, winnerID)`, and return the one whose stored
`result_json.posting_group_id` equals that group id. Add a regression
test with three actors sharing one op-id string asserting bob gets
alice's result exactly. (Pre-fix that test may pass by scan luck;
post-fix it passes deterministically.)

## Checks run (all in the worktree, MariaDB enabled)

- `gofmt -l cmd internal` → empty.
- `go build ./...` → ok. `go vet ./...` → ok.
- `go test -count=1 -timeout 20m ./internal/posting/` with
  `ATHENA_TEST_DATABASE_URL` (throwaway `test_athena_*` only) → all
  pass unskipped in 226s. Note: my first run used `-race` with the
  default 10m timeout and hit the timeout at 601s (pure slowness —
  no failure, no data race, no hang). I then ran the two properties
  plus the canonical test with `-race -timeout 20m -v`: all pass,
  `[rapid] OK, passed 100 tests` for both Property 8 and Property 9,
  zero `DATA RACE` reports.
- Full `make test` → exit 0, no FAIL lines, 537/537 web tests pass.
  (Same environment note as slice 1: `node` came from
  `~/.nvm/versions/node/v26.7.0/bin` on `PATH`; no repo change needed.)
- `make generate` twice → both exit 0, tree clean, no diff.
- No browser pass: no Desk in this slice, per brief.

## What was verified by reading

1. Contract flow (`write.go` `PostInTx` + `operations.go`): empty op id
   is generated (no replay cover, documented); known ids replay before
   validating, so a retry survives later invalidation (R6.7);
   same-id/different-payload conflicts at step 4 and on both collision
   paths (R6.8); the group unique key plus `replayGroupResult` posts
   once across operation ids and returns the first result verbatim
   (R6.10, pinned by `TestDifferentOperationSameVoucher`); per-revision
   repost works and id reuse across revisions conflicts (R6.4,
   `TestRevisionPostsAgain`). Insert column/arg counts rechecked
   against the migration-14 DDL: groups 7/7, operations 7/7 (nullable
   workspace), log 13/13, outcomes/codes fit their `VARCHAR` widths.
2. Canonical hash (slice-1 follow-up 4): `canonicalDecimalText`
   strips trailing fraction zeros; `Decimal.String` emits only plain
   digits with an optional dot and `-` (no exponents, no `-0`, no `+`),
   so equal values hash alike at any scale. Pinned by
   `canonical_test.go` and `TestOperationScaleInsensitive`.
3. Property 9 (`posting_test.go`, `TestIdempotentReplayProperty`):
   genuine rapid coverage — drawn plans, 2–6 concurrent replays behind
   a start barrier, balance snapshots before/after, voucher row counts,
   a conflict case, a second-operation case, and 2–4 racing fresh
   posts asserting exactly one winner with losers matching. The race
   genuinely exercises the group-collision path across 100 iterations.
4. Deadlock reasoning (brief item 3): every post takes locks in one
   global order — groups → operations → `l_gl` → balances (sorted by
   company, book, account) → audit head → outbox → log — and replay
   paths add only lock-free fresh-connection reads plus one log
   insert. Unique-check waits are single-key (the waiter unblocks with
   duplicate iff the winner committed, else proceeds), so no wait
   cycle exists. Empirically: the concurrent suite passes, no hangs.
5. Nil Dialect (slice-1 follow-up 5): `isDuplicate`
   (`operations.go:92`) returns false on nil instead of panicking,
   pinned by `TestNilDialectDegradesGracefully`; the `Engine` doc now
   states the Dialect requirement. (Caveat: follow-up F1.)
6. Log coverage: posted / replayed(ok) / conflict /
   replayed(duplicate_group) / rejected(invalid, book_policy_open,
   correction_window_open) / error(internal) are each pinned
   (`TestPostingLogOutcomes`, `TestHardClosedPeriodLogsCorrectionWindow`,
   `TestNilDialectDegradesGracefully`); failures log via a separate
   connection so rows survive rollback (R6.6).
7. Z2: no window/policy invented in this diff (grep confirms); both
   sentinels still pinned by the untouched slice-1 validation cases
   plus the new log-code tests; `codeFor` maps them to distinct
   rejected codes.
8. Scope: 5 files, all in `internal/posting/` (`operations.go` and
   `canonical_test.go` new); no migration, no ADR, no edits elsewhere.

## Follow-ups (non-blocking)

1. A nil-Dialect engine on a *fresh* post still panics in
   `audit.Append` (`d.ForUpdate()` on nil) — `isDuplicate` fixed only
   the classification path. The doc now says Dialect is required, so
   this is contract discipline, but a constructor or an explicit guard
   would be safer than a nil-interface panic.
2. `replayOperationResult`'s match branch is unreachable short of a
   SHA-256 collision (same hash implies same group key, which would
   have dup'd at the group insert first); its conflict branch is
   reachable only in a narrow same-id/different-voucher race window
   and is untested. Suggest a comment plus a concurrent test racing
   one op id across two vouchers to pin the conflict branch.
3. `Post`'s outer `tx.Commit()` failure returns raw with no log row
   (the in-tx row rolls back) — a corner in the "exactly one row per
   attempt" claim. `logFailure` on commit error would close it; note
   the commit-may-have-succeeded ambiguity either way.
4. `RequestID` (CHAR(36)) and `Actor` (VARCHAR(255)) are unvalidated
   and untruncated in log/op inserts — overlong values fail late with
   raw DB errors. Validate in `checkOptions` or truncate like the op
   id and message.
5. Slice-1 follow-ups 1–3 remain open and untouched by this slice:
   `fitsLedger` 22-integer-digit tightening, calendar-date validation,
   multi-company balance coverage.

## Re-review (fix loop 1, commit 33a5447)

Verdict (round 2): PASS

Scope: B1 fix only (F1–F5 skipped per master instruction, not blockers).

The fix implements exactly the suggested approach: `replayGroupResult`
now `SELECT`s the collided group row's `id` and returns the operation
row whose stored `result_json.posting_group_id` points back at it,
instead of the bare `LIMIT 1`. Determinism argument: group ids are
fresh uuids minted per post and replays write no operation rows, so at
most one op row can point at a group — the match is unique. Corrupt-JSON
rows are skipped rather than fatal (an improvement: a corrupt
non-winner row no longer breaks replay of the true winner); zero
matches yields a clear logged internal error. The `fail` closure
preserves the old log-and-return-raw-err behavior on every error path,
and the conflict check now compares against the true winner's
actor/hash. No other files changed (2 files: `operations.go`,
`posting_test.go`).

Regression proof, scratch worktree at `17e10ab` (pre-fix) with only the
new test transplanted via `git apply` (worker tree untouched, worktree
removed after):

- Pre-fix: `TestGroupCollisionAcrossActorsFindsWinner` FAILS 3/3 —
  bob gets carol's group/rows (`PostingGroupID:01a20141-458d…` vs
  alice's `01a20141-4599…`), confirming the test catches B1 and fails
  deterministically in practice (carol-first setup + PK-ordered scan).
- Post-fix (`33a5447`): the test PASSES; `gofmt -l cmd internal`
  empty; `go vet` clean; full posting suite passes unskipped on
  MariaDB (`ok … 211.917s`); both rapid properties pass under `-race`
  with `[rapid] OK, passed 100 tests` each and no data races.

B1 is closed. No new blockers introduced.
