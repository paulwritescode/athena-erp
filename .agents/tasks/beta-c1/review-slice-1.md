# Review: C1 slice 1, posting validation + writer (6.2+6.3)

Branch: `feat/c1-posting` in `../athena-erp-c1` (commits `9d87fbf`, `a9e1d49`).
Scope: slice 1 only (batch-2 G6.2+G6.3) against batch-2 `design.md` G6,
requirements R6, ADR-0023, and the Z2 rule.

Verdict: PASS

No blockers. No wrong balances, lost writes, broken atomicity, or invented
Z2 policy found. The diff adds exactly three new files, all in
`internal/posting/` (`posting.go`, `write.go`, `posting_test.go`, +1369);
nothing outside that directory changed, no migration added or edited, no ADR
added — as expected (migration-14 tables reused).

## Checks run (all in the worktree, MariaDB enabled)

- `gofmt -l cmd internal` → empty.
- `go build ./...` → ok. `go vet ./...` → ok.
- `go test -count=1 ./internal/posting/` with `ATHENA_TEST_DATABASE_URL`
  (throwaway `test_athena_*` only) → all pass unskipped: happy path,
  35 validation subcases, control-account positive, `TestPostInTx...`
  rollback/commit, and `TestBalancedPostingsProperty`
  (`[rapid] OK, passed 100 tests`).
- Full `make test` → Go suite pass, then 49 web files / 537 tests pass.
  (Note: this shell had no `node` on `PATH`; I ran it with
  `~/.nvm/versions/node/v26.7.0/bin` prepended. No repo change needed.)
- `make generate` twice → both exit 0, `git status` clean, no diff.
- No browser pass: no Desk in this slice, per brief.

## What was verified by reading

1. Plan validation (`posting.go:122-188`): voucher/event/book/date
   presence, enabled company + base currency, leaf + enabled + in-scope
   account, one-sided non-negative base amounts, txn amounts on the same
   side, quantity/UOM pairing, dimension existence/membership/mandatory
   axes, control-account source, per-company open-period cover
   (`checkPeriods`, soft-closed → `ErrInvalid`), and balance per
   company+book in base plus per-currency txn nets (`sumPlan` /
   `checkBalance`, `posting.go:447-477`). Matches R6.1/6.3/6.17 and the
   mechanical half of 6.13.
2. Writer (`write.go:25-86`): `Post` opens one transaction, `PostInTx`
   joins the caller's; ledger rows (21 columns, verified against the
   migration-14 `l_gl` DDL — placeholders and args both count 21),
   balance deltas, `audit.Append` (no action allowlist, so `posted` is
   fine), and outbox row all inside the tx. Every validation-failure test
   asserts `l_gl`, `athena_balance_account`, journal audit rows and
   `athena_outbox` stay empty (`posting_test.go:325-337`); rollback and
   commit paths are pinned (`posting_test.go:487-519`). Matches R6.5 and
   the atomic half of R6.6.
3. Balance deltas (`write.go:184-241`): DB-side `SET x = x + ?` (no
   lost update), update-first with insert fallback and duplicate-key
   retry; the fallback row cannot disappear (no deletes), so the retry
   always lands. Race reasoning holds under InnoDB.
4. Property 8 (`posting_test.go:367-485`): genuine rapid coverage —
   draws line count, amounts, accounts, sides, txn flag; plugs the net
   to balance by construction; asserts stored rows balance per voucher
   and that a +1¢ perturbation writes nothing. Not a table test.
5. Reuse: `types.Decimal` (`Rescale` semantics confirm the 6-fraction
   check), `audit.Append`, `ledger.Service` (`GetCompany`,
   `GetCurrency`, `GetAccount`, `ListDimensions`, `PeriodFor`, period
   statuses — all exist with the used signatures). `types.Allocate` is
   not used, correctly: nothing in this slice allocates (G7's job).
6. Dangling group id is safe: `l_gl.posting_group_id` has only a KEY,
   no FK, so carrying the id without the group row until 6.4 cannot
   violate constraints. Package doc (`posting.go:11-12`) says so.

## Z2 rule (binding) — verified clean

Two Z2 points, both plain sentinels, both test-pinned:

- Empty book → `ErrBookPolicyOpen` (`posting.go:132-134`), pinned by
  `posting_test.go:208` ("no book needs the book policy").
- Hard-closed period → `ErrCorrectionWindowOpen`
  (`posting.go:420-421`), pinned by `posting_test.go:302-310`.

Grep over `internal/posting/` for day counts, date arithmetic
(`AddDate`/`Add`/`Sub`/`After`/`Before`/`time.Now`), default books and
book names as policy: no invented window or policy. `dimensionApplies`
string range checks are validity windows, not backdating policy;
`"main"` appears only as a fixture book in tests. No book is ever
defaulted — an empty book fails.

## Follow-ups (non-blocking, for later slices or a tidy-up)

1. `fitsLedger` (`posting.go:255-265`) allows 28 significant digits, but
   `DECIMAL(28,6)` allows only 22 integer digits. A value ≥ 10^22 passes
   validation and then fails at the DB with a raw error (safe rollback,
   unmapped error). Tighten to 22 integer digits → `ErrInvalid`.
2. `validDate` (`posting.go:479-491`) accepts non-calendar dates such as
   `2026-02-31`; inside a covering period's string range it reaches the
   `DATE` column and fails raw. Validate a real calendar date.
3. No multi-company test: per-company balance is only covered by
   inspection (`sumPlan` keys) plus single-company cases. Suggest one
   case (or a Property 8 extension) where lines balance globally but not
   per company and must fail.
4. `hashPlan` (`write.go:307-357`) hashes `Decimal.String()` text, so
   `100.00` vs `100` hash differently. Harmless now (stored/returned
   only); slice 2's Operation_ID conflict comparison must normalize
   before comparing.
5. Zero-value `Engine` (nil `Dialect`) would panic in the duplicate-key
   fallback (`write.go:232`), reachable only on a concurrent-insert
   race. Document `Dialect` as required or default it.
6. Expected later-slice gaps, already in the brief's plan, not defects:
   no `athena_posting_groups` / `athena_operations` / `athena_posting_log`
   rows yet (6.4), hence no post-once uniqueness; no Preview/status/log
   procedures or CLI (6.9); no lifecycle or Rule hooks (6.8); no
   party-item existence check for control sources (G7).
