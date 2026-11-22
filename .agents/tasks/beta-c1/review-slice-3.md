# Review: C1 slice 3, reversal mirrors + correction-period rules (6.6+6.7)

Branch: `feat/c1-posting` in `../athena-erp-c1`, commit `5bbc94e`
(reviewed diff `git diff 3dece91...HEAD`, verified clean tree at that
commit before any gate). Scope: slice 3 only (batch-2 G6.6+G6.7)
against batch-2 `design.md` G6, requirements R6.11–R6.13, ADR-0023,
and the still-binding Z2 rule.

Verdict: PASS

No blockers. Mirror semantics exact, replay sharing correct with the
retry-after-close path test-pinned, Z2 clean, full suite green.

## Checks run (all in the worktree, MariaDB enabled)

- `gofmt -l cmd internal` → empty.
- `go build ./...` → ok. `go vet ./...` → ok.
- `go test -count=1 -timeout 20m -v ./internal/posting/` with
  `ATHENA_TEST_DATABASE_URL` (throwaway `test_athena_*` only) → all
  27 tests pass unskipped in 347s, including the 8 new reversal tests
  and `[rapid] OK, passed 100 tests` for Properties 8, 9 and 10.
- Full `make test` → exit 0, zero FAIL lines, 55 web files / 563
  tests pass. (`node` via `~/.nvm/.../bin` on `PATH`; no repo change.)
- `make generate` twice → both exit 0, tree clean, no diff.
- No browser pass: no Desk in this slice, per brief.

## What was verified by reading

1. Mirror exactness (`reverse.go:213-241`, `write.go` `insertRows`
   + `rowFlags`): every stored amount swaps sides (base and txn),
   currency/quantity/UOM/dimensions carry over untouched, rows are
   flagged `is_reversal` with the stored row's id, under the cancel
   event. `assertMirror` pins flag, identity resolution, account,
   swapped sides, same currency, swapped txn sides, identical
   quantity/UOM and canonical-equal dimensions; `TestReverseHappyPath`
   drives it with a rich plan (control account, dimensions, txn,
   quantity) and pins cancel event, inherited date/period, per-account
   netting, both group rows, the `reversal_of` audit change, the
   `posting.reversed` outbox payload and the cancel log row. The
   refactor (`effectiveOperationID`, `checkReplay`, `commitPost`,
   `checkIdentity`, `rowPeriod`, `insertOutbox` extra payload) is a
   clean extraction — the post path is behavior-identical (same event
   type, nil extra, empty flags). Balances net to zero because
   `applyBalances` runs on the swapped mirror; periods don't affect
   netting since balances are per (company, book, account), so
   corrections in another period still net.
2. Inherited trust: reversal skips company/account/dimension/control
   rechecks and checks only the reversal date's period — as the doc
   comment claims. Pinned by `TestReverseInheritedTrust` (account,
   dimension and company disabled after posting; unwind succeeds).
   Dropped `SourceType/SourceID` on mirror lines is faithful, not a
   gap: `l_gl` stores no source columns, nothing downstream reads
   them, and no validation runs on the mirror.
3. Replay sharing: `checkReplay` runs before the period check, so a
   retry returns the first result even after the period closes —
   PINNED by the "retry replays after close" subtest (reverse, then
   soft+hard close, then same-id retry returns the result with
   `Replayed`). Same-id replay, different-op-id post-once-per-cancel
   (shared group key + B1-fixed winner lookup) and cross-voucher
   conflict are pinned by `TestReverseReplay`. Same op id for a
   voucher's own submit and cancel conflicts (correct per R6.8:
   different payload) — see follow-up 1. Cancel-of-cancel is
   impossible (`loadSubmitGroup` reads submit-event groups only);
   unposted voucher → `ErrInvalid`, both pinned.
4. Z2: hard-closed reversal period returns `ErrCorrectionWindowOpen`
   (`reverse.go`, via `codeFor` → rejected/correction_window_open,
   log row pinned); soft-closed and missing periods → `ErrInvalid`;
   no correction target is guessed — the date is caller-named or
   inherited, and only openness is checked. Grep over the diff finds
   no day counts, date arithmetic or book defaulting (empty book on
   reverse is a mechanical group-lookup miss, not policy). The
   explicit-date correction path is pinned by `TestReverseExplicitDate`.
5. Property 10 (`TestReversalMirrorsProperty`): genuine rapid
   coverage — drawn plans (the shared helper now also draws
   dimensions and quantities, which strengthens Properties 8 and 9
   too), post + reverse + replay per iteration, exact-mirror
   assertion, per-account net-zero over stored text, row counts.
6. Scope: 4 files, all in `internal/posting/` (`reverse.go` new);
   no migration, no ADR, no edits elsewhere. Lock order unchanged
   from the slice-2 reasoning (reads, then groups → operations →
   rows → sorted balances → audit → outbox → log); no new deadlock
   risk.

## Follow-ups (non-blocking)

1. Same op id for a voucher's submit and its cancel conflicts via the
   shared `checkReplay` (correct per R6.8) but no test pins it —
   one-line addition to `TestReverseReplay`.
2. No concurrent-reversal test: the shared `commitPost` race paths
   were pinned in slice 2 and the reversal-specific work is
   read-only + deterministic (`ORDER BY id` mirror), but racing
   reversals in Property 10 would close the loop.
3. `assertMirror` doesn't pin bijectivity (each original reversed
   exactly once) — implied by unique row ids, could assert explicitly.
4. `checkIdentity` says "a post needs its actor/source" on the
   reversal path too — cosmetic wording.
5. Still open from earlier rounds, unchanged by this slice: slice-2
   F2/F3 (unreachable `replayOperationResult` match branch,
   commit-failure logging corner — both shared shapes Reverse reuses),
   slice-2 F4 (actor/request-id lengths, applies to reversal log rows
   too), slice-1 F1–F3 (`fitsLedger` integer width, non-calendar
   dates — now also reachable via `ReverseOptions.PostingDate` —
   multi-company coverage, now including reversals).
