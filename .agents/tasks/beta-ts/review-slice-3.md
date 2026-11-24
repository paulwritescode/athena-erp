# Review: TS slice 3 posting shared-DB (r-ts3)

Reviewed tip: 690a1a9 (code b9b5f02 + numbers 690a1a9), rebased on main 9b1c070.
Review tree: /Users/iampaulking/projects/duchess/athena-erp-ts-s3-r, detached, clean.
Slice delta vs new base: `git diff 9b1c070..690a1a9 -- internal/posting/` =
posting_test.go + settlement_test.go + shared_test.go (conversion only);
plus measure.md bookkeeping. No slice-4 content in the commits
(slice 4 held uncommitted in the worker tree per master's order, confirmed
in comms 19:37:34 and by `git status` showing no posting dirt).

Verdict: PASS

## 1. Conversion real — PASS

- New `internal/posting/shared_test.go`: TestMain sweeps stale DBs (warn-only),
  `testdb.OpenShared`, one `store.Up`, `Freeze`; `sharedDatabase(t)` does
  `Reset()` per test and skips like `testdb.New` when MariaDB is away.
- All 5 `store.Up` sites converted: `setup()` + 3 rapid properties in
  posting_test.go (TestBalancedPostingsProperty, TestIdempotentReplayProperty,
  TestReversalMirrorsProperty — all outside `rapid.Check`, one shared handle
  per test exactly as before) + `settleSetup()` in settlement_test.go.
  Only remaining `store.Up` in the package is the TestMain one; zero
  `testdb.New` calls remain.
- Pattern matches `internal/server/shared_test.go` modulo package name,
  imports, dialect qualifier and the server-only auth cheap-hash lines.
- Zero keepers justified, checked each: no `t.Parallel` in the package; no
  DDL (CREATE/ALTER/DROP/TRUNCATE/RENAME) — new main tests use UPDATE-only
  DML; no migration-state tests; `canonical_test.go` (TestCanonicalDecimalText,
  TestHashPlans) is DB-free pure unit code, untouched. New main tests at the
  rebased tip (FX + Property 12, 11 added) all route through
  `settleSetup` → shared DB, isolate via uuid companies/groups, add no
  goroutines.

## 2. No weakening — PASS

- Test names before (da178bb) vs after: identical 35 in the two converted
  files; before-run JSON reconciles 37/37 with 0 fails (35 + 2 canonical).
- Diff hunks touch only the 5 setup preludes + dropped `server`/`testdb`
  imports; every assert, seed call and oracle line untouched.
- Concurrent tests still join: TestIdempotentReplayProperty receives all n
  then all m buffered outcomes before asserting; TestConcurrentAppliesNeverOverApply
  keeps balanced Add/Done/Wait (settlement_test.go:365-384 at new tip).

## 3. Numbers honest — PASS (29x: 396.2s → 13.7s)

- BEFORE: suite "TS slice 3 posting before", clean:true, head da178bb,
  exit 0, full-package `go test -json -count=1 ./internal/posting` (no -run
  filter). `/tmp/ts-posting-before.json` exists (332 lines, 0 fails, final
  Elapsed 396.202s). Top tests match measure.md: TestPostValidation 171.7s,
  TestReverseClosedPeriods 29.5s, ~6s/test migrate floor.
- AFTER: suite log 1124-193523, `go test -timeout 20m -count=1
  ./internal/posting/` (no -run filter — full package, not a subset),
  exit 0, `ok athena-erp/internal/posting 13.711s`, head e925a3c.
- The after-run's `clean:false` is explained and immaterial: the dirt was
  slice-4 test-only files held uncommitted in the worker tree (other
  packages' _test.go + measure.md, none under internal/posting/ or imported
  product code — verified in worker `git status`), which cannot enter the
  posting test binary.

## 4. Gates — PASS

- Reused master-priority after-run (exit 0, 13.7s). Ran myself at 690a1a9:
  `gofmt -l` clean, `go build ./internal/posting/` OK,
  `go vet ./internal/posting/` OK. No other packages affected.

## 5. Scope — PASS

- Slice commits touch posting _test.go + measure.md ONLY. No product code,
  no Makefile, no `-p`, no other packages' files.

## Corrections and notes for master (non-blocking)

1. NOT byte-for-byte vs e925a3c: `settlement_test.go` differs because the
   rebase pulled main's newer FX/settlement tests (C5 Property 12 etc.) into
   the file. The slice DELTA is intact and conversion-only against 9b1c070
   (verified hunk by hunk). "Content-identical" holds for the delta, not the
   file bytes.
2. Committed measure.md AFTER row still says PENDING; 13.7s verified in
   suite log 1124-193523 + worker's held measure.md. Recommend amending the
   AFTER row at merge so the doc isn't left stale.
3. The 13.7s run tested e925a3c's 37 tests; the rebased tip has 48 (11 new
   from main, all on shared DB). Merge gate covers them.
4. Minor: measure.md "27 tests" prose vs 37 actual — stale estimate, not a
   number defect.
5. FYI (not TS's): main carries a committed rapid `.fail` artifact from
   646cc6d (C5 slice 5). Flagging for Stylus hygiene.
