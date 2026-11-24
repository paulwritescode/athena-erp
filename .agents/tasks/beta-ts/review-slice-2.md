# Review: TS slice 2 — sweep + stamped names + pool docs (r-ts2)

Verdict: PASS

Tree: `/Users/iampaulking/projects/duchess/athena-erp-ts-s2-r`, detached at
da178bb (68b9a97 code + da178bb numbers), verified clean before and after;
temp naming probes added and removed by the reviewer, tree clean at send.

Scope (diff af1f891..da178bb): exactly the 7 briefed files —
`internal/testdb/testdb.go`, `sweep_test.go`, `shape_test.go`,
`internal/auth/shared_test.go`, `internal/server/shared_test.go`,
`README.md` (pool sentence), `.agents/tasks/beta-ts/measure.md` (numbers).
No Makefile change (2a dropped per Stylus ruling in measure.md), no
migrate-once (slice 3/4), no production file touched (`internal/testdb` is
imported only by `*_test.go` files — verified by repo-wide grep).

## Brief criteria

1. Naming — PASS. `throwawayName()` builds
   `test_athena_<pkg>_<unixts>_<rand12>`; `callerPackage()` walks the stack
   skipping `internal/testdb.` frames, trims `_test`, lowercases to
   `[a-z0-9]` (max 16, fallback `x`), zero call-site changes (all callers
   still compile: `go build ./...` + `go vet` clean). Walk verified live for
   3 packages: `testdb` (shape_test), `posting` and `ledger` (temp reviewer
   probes, since removed:
   `test_athena_posting_1795530272_8bac217256ef`,
   `test_athena_ledger_1795530272_6ef7bc50cddc`). The `testdb_test`→`testdb`
   trim plus the `posting_test`/`ledger_test` trims prove the
   `auth_test`→`auth` path; `server` (internal package) yields `server`
   directly. Repo-wide grep: nothing parses DB names besides
   `throwawayBirth` itself.
2. Sweep conservative — PASS. `SweepStale(1h)` drops only: stamped names with
   `time.Since(birth) > maxAge`, or pre-timestamp names whose
   `MAX(CREATE_TIME)` is older than maxAge (newest-old ⇒ all-old). Keeps:
   fresh stamps, tableless pre-timestamp names, unreadable DBs (error ⇒
   keep + warn), future stamps, over-64-char names. Live concurrent runs are
   safe: fresh-stamp DBs (new binaries), fresh-table DBs (old binaries),
   tableless just-created DBs, and browser-harness `test_athena_browser_<port>`
   DBs (fresh tables; the harness drops/recreates at start anyway) all fail
   closed. `DROP` is guarded by `validThrowawayName` (prefix + `[a-z0-9_]`
   + ≤64) and the `LIKE` prefix is escaped. TestMain wiring is warn-only.
3. Tests real — PASS. `sweep_test.go` has no sleeps, fixed 2h offsets,
   extreme ages scoped to a pid-prefixed `sweepUnder` range, and backstop
   cleanups; skip-safe via `openAdmin` + pure table tests. Ran green 3×
   (subset, full package, subset re-run — no flakes):
   `TestSweepStaleDropsOnlyProvablyOldDatabases`,
   `TestSweepStaleHonorsMaxAgeBothWays`,
   `TestSweepStaleAgesPreTimestampDatabasesByTheirTables`,
   `TestSweepStaleWithoutDatabaseURLIsNothing`, `TestThrowawayBirth`,
   `TestValidThrowawayName`, `TestThrowawayNameRecordsPackageAndTime`.
4. Wiring — PASS. Both TestMains (auth + server — the only two in the repo)
   call `SweepStale(time.Hour)` before `OpenShared`, warn-only on error.
   Exercised live: `-run TestZZNoMatchXYZ` on both packages starts TestMain
   (migrate + sweep) and exits 0 (auth 15.9s, server 15.6s).
5. Gates — PASS. `gofmt -l cmd internal` empty, `go build ./...` OK,
   `go vet` on testdb/auth/server OK. `affected` (main-repo path) on clean
   da178bb: 7 pkgs, exit 0 —
   cmd/athena 266s, auth cached (pass from same-commit run), ledger 268s,
   posting 637s, server 585s, stock 122s, testdb 0.7s.
   An earlier affected attempt on the same clean commit went red with bogus
   `[build failed]` (`could not import fmt/os/...` — shared go-build cache
   entries vanished mid-run; disk 95% full); `go build`+`go vet` passed
   immediately after, and the retry went green, so the red was
   environmental, not code. Recorded in suite.jsonl (1124-192950 exit 1).
6. Scope exclusions — PASS. No Makefile `-p` change, no migrate-once.

## Stylus's 3 orders

- (a) Posting comparison: measure.md records affected posting 451s (worker
  run) vs the ~1200s baseline, plus the F7s1 full-gate rank table. My retry
  saw posting 637s (load variance) — still ~½ baseline.
- (b) TestMain >1h cleanup: implemented + wired + tested as above.
- (c) 512M pool in docs, not code: README tests section carries the
  `innodb_buffer_pool_size=512M` recommendation. Nothing in code.

## measure.md numbers

Verified against suite logs: focused testdb `ok 7.618s` (1124-155131) and
affected 7 pkgs (1124-170036: cmd 139s, auth 657s, ledger 152s, posting
451s, server 435s, stock 202s, testdb 1s) match the file exactly. Note both
worker runs record `clean:false` (dirty tree — almost surely the measure.md
bookkeeping); my clean-commit affected retry above is the authoritative
green for da178bb.

## Notes (non-blocking)

- No test deleted or weakened: the delta is purely additive (TestMain sweep
  calls, new tests, docs); the name-format change alters no asserted
  behavior.
- Pre-timestamp live DBs rely on table CREATE_TIME; a hypothetical browser
  pass idle >1h with old tables could be swept, but passes run minutes and
  the harness recreates its DB at start. Acceptable per the ordered design.
