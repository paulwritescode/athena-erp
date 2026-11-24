# Review: C5 slice 2 settlement applications (7.2)

Verdict: PASS

Tree: athena-erp-c5-s2-r, detached at 9b91f8d (parent 2954bce), clean.
Diff scope: `internal/posting/settlement.go` (new) + `internal/posting/settlement_test.go` only. No other lane's files, no protos, no App.vue, no L3-owned UI.

## 1. Correctness

- Open items per instalment: `CreateItemsInTx` records one row per (source voucher type/id/revision, instalment); the unique source key maps to `ErrItemExists`, pinned by `TestCreateAndListOpenItems`. List is stable id order.
- Line-level applications: each row carries both endpoints (settlement voucher type/id/revision/event/line + item id), both currencies (payment + item columns), the base amount, and the reversal-link columns (NULL in 7.2; mirrors are slice 4). Columns verified against migration 20 in `sysmigrations.go` (17/17/21 args match).
- Locks on both residuals: `lockItems` takes every touched item row `FOR UPDATE` in sorted id order (deadlock-free) before any allocation. The payment-side residual is immutable request state, not shared state — one voucher+line+item applies once (unique key → `ErrAlreadyApplied`, pinned by `TestApplyReplaysSameLineOnce`) — so no second shared lock exists to take; the code documents this. No TOCTOU: every share is decided against running locked residuals before anything writes, all in one tx at READ COMMITTED; deadlock/InnoDB-1020 retries with a fresh tx (`Apply`, 3 attempts).
- Aggregate ceiling under the lock: explicit amounts must fit the running residual and sum within the line; auto shares are capped at the residual; per line applied + unapplied == line amount exactly (guarded by the ceiling-slip check). Cross-line same-item reuse flows through the shared running map.
- Unapplied cash separate: remainder rows go to `l_party` with NULL item/application id; nothing inserts them into `athena_party_items`. `TestApplyLeavesUnappliedRemainder` pins both counts (1 NULL-item party row, still 1 item).

## 2. allocate.go use and exactness

- Auto shares go through `remainder.Allocate(weights, 6)`; no reimplemented rounding/splitting. `weightUnits` only builds int64 proportionality weights from residuals. `fitsLedger` (≤6 fraction digits, enforced on items, lines and explicit amounts) guarantees `Allocate`/`Rescale(6)` never hit `ErrRounding` on validated input; zero-residual auto targets are double-guarded (early return + cap-and-skip).
- No floats anywhere in either file (grep clean). All math in `types.Decimal`.

## 3. Tests

- `TestConcurrentAppliesNeverOverApply` genuinely races: two goroutines Apply 80 each against a 100 residual with no sequencing; both must succeed (cap, not fail) and the test pins residual exactly 0 and total applied exactly 100. Single-row `FOR UPDATE` serializes, so no deadlock flake; `Apply` retries anyway.
- Ceiling pinned by the concurrency totals plus the explicit over-application refusal in `TestApplyExplicitAmounts`. Exact-sum pinned per result by `sumApplied` in two tests. Order-independence pinned across two databases in `TestApplyOrderIndependent`.
- Checks on 9b91f8d: `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet ./internal/posting/` OK, `affected` green via suite: `ok cmd/athena 325s`, `ok internal/posting 739s`, `ok internal/server 705s`, exit 0.
- Note: the worker's cited green run (suite `1124-070617-w-c1.log`) is at pre-slice head bafe51d, whose tree has no `settlement.go` — it could not have run the new tests. The green `affected` run on 9b91f8d above (this review's queued run) is the covering evidence.

## 4. Z2

No reversals implemented (`is_reversal=false`, `reverses_application_id`/`reversed_row_id` NULL, mirrors deferred to 7.4 in comments only), no window, day count, book policy or default book, sentinels untouched (zero matches for window/policy/sentinel identifiers). Slice 4 owns it. PASS.

## 5. Scope

Two files only, both slice-2-owned. PASS.

Review worktree `athena-erp-c5-s2-r` left in place for master to remove.
