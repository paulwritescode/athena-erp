# TS measurements (running log; Stylus: write every number as it lands)

Worktree `/Users/iampaulking/projects/duchess/athena-erp-ts`, branch `feat/test-speed`.
Machine: 8 CPUs / 8 GB. Load at 11:17 EAT: 66 / 58 / 29 (heavily contended;
absolute seconds swing with ambient load — see caveats).

## Slice 0 (package campaign) — STOPPED by Stylus order ~13:00

Full per-package `-json` campaign replaced by slice 1 (Argon2 first). The
already-queued auth run was kept as the slice-1 before-run.

## Baselines from tonight (pre-pool-fix unless noted)

| Run | Conditions | Numbers |
| --- | --- | --- |
| Brief baseline (tonight's log) | parallel `go test ./...` | auth 1178s, posting 1158s, server 897s, ledger 694s, cmd/athena 690s; median full green 15 min |
| F3 gate (Stylus) | parallel, auth+posting+server ran together | auth 1179s / posting 1161s / server 1117s, finished together = starvation evidence |
| C7 train gate (Stylus) | auth cached, posting ran | posting 670s (vs 1161s starved) |
| C2s2 gate (Stylus) | cached | 32s, confirmed legitimate |
| w-f7 affected (suite 12:54–12:56) | serial `go test ./cmd/athena`, F7 branch | cmd/athena 123s (post-fix env; branch differs) |
| r-c5s3 affected (suite 12:39–12:54) | parallel 7 pkgs (cmd,expr,ledger,posting,rules,server,types), C5 branch | 860s total |

## Root causes (Stylus)

1. MariaDB: 100 abandoned `test_athena_*` DBs (3069 tables) from killed runs +
   128 MB InnoDB pool. Fixed at runtime: 87 DBs older than 2h dropped, pool
   raised to 512 MB. Orders: (a) posting-vs-~1200s comparison [this file];
   (b) TestMain cleanup of stale test DBs [slice 2]; (c) 512M pool in docs [slice 2].
2. Auth Argon2id starves posting+server in parallel runs (F3 vs C7 above).
   Fix: slice 1 (this file).

## Slice 1: cheap Argon2 params in the auth test binary (IN PROGRESS)

Design (test-only, zero production change; follows the existing server-TestMain
precedent at `internal/server/shared_test.go:25`):
- `internal/auth/shared_test.go` TestMain sets `auth.DefaultParams` to the cheap
  `{8 MiB, 1, 1}` every other test already assumes; no `t.Parallel` in the
  package, so the global is safe.
- `TestPasswordHashRoundTrip` pins production `{64 MiB, 3, 4}` explicitly
  (save/restore): same PHC-prefix/round-trip/wrong-password/salt oracles, still
  exercises production-cost hashing. This is the one test checking prod params.
- `TestWeakerHashMatchesAndAsksForRehash` pins production explicitly: same
  cheap-hash-vs-prod-default rehash oracle.
- No other auth test asserts on hash cost (verified by grep); `dummyHash`
  package-init cost unchanged (production behavior untouched).

| Run | Command | Result |
| --- | --- | --- |
| BEFORE | dedicated run SKIPPED by Stylus order (~13:05; queued 11:17 waiter cancelled via `suite stop w-ts`, never started) — baseline is tonight's log: auth 1178s in a full parallel gate | 1178s (log baseline) |
| AFTER | `go test ./internal/auth` run by MASTER as master-priority ticket post-commit | PENDING (master runs) |

Slice-1 edit (committed on `feat/test-speed`, test files only, 14+/3-):
`internal/auth/shared_test.go` TestMain sets cheap `{8 MiB, 1, 1}`;
`internal/auth/auth_test.go` `TestPasswordHashRoundTrip` and
`TestWeakerHashMatchesAndAsksForRehash` pin production `{64 MiB, 3, 4}` with
save/restore. `gofmt -l` clean, `go vet ./internal/auth` OK. No production
file touched; no assert added/removed/weakened; no `t.Parallel` in package.

## Slice 1 result (merged 1c757a0, pushed)

Auth 188s vs 1178s log baseline (master's master-priority run). Fix is live.

## Slice 2: stale-DB sweep + pool docs (IN PROGRESS)

- `internal/testdb`: throwaway names are now
  `test_athena_<pkg>_<unixts>_<rand12>` (pkg from a runtime caller walk, no
  call-site changes); `SweepStale(1h)` drops provably-stale DBs (old name
  timestamp, or pre-timestamp name with all tables older than 1h) and keeps
  everything doubtful; wired into the auth + server TestMains (warn-only on
  error). Live browser-harness DBs and old-binary live DBs are safe by the
  fail-closed rule.
- Maintained tests: `internal/testdb/sweep_test.go` (drop/keep/maxAge/table-
  fallback/env-unset/pure parsing tables) + `internal/testdb/shape_test.go`
  (name shape via `NewWithConfig`). Deterministic, no sleeps, skip-safe.
- Docs: `innodb_buffer_pool_size=512M` recommendation in README tests section.

| Run | Result |
| --- | --- |
| Focused `./internal/testdb` (master-priority, suite log 1124-155131) | GREEN `ok testdb 7.618s`, exit 0, 8s slot time |
| `affected` 7 pkgs (suite log 1124-170036, `-p 2` default), commit 68b9a97 | GREEN exit 0, 1103s slot: cmd/athena 139s, auth 657s, ledger 152s, posting 451s, server 435s, stock 202s, testdb 1s |

Slice-2 commits on `feat/test-speed`: 68b9a97 (code+docs+tests) + measure
bookkeeping. Full-suite confirmation rides master's merge gate.

## Stylus waiting-not-CPU data (F7s1 gate, carries slice 1)

auth+posting still ~18 min inside full `./...` (auth 188s alone); server.test
at 1.9% CPU with MariaDB idle. MariaDB: 39,795 row-lock waits, 1,625s total
lock wait since start, `innodb_lock_wait_timeout` default 50s.

## Stylus DDL-fsync finding (F7s2 -p 4 gate; REPRIORITISED)

Test procs under 3% CPU while MariaDB runs nothing but CREATE/ALTER/RENAME
from per-test migrations, disk ~11k transfers/s — every DDL is an fsync.
Migrate-once-per-package (clone or TRUNCATE per test) outranks clocks and
lock waits. `innodb_flush_log_at_trx_commit=2` is user-gated (same MariaDB
holds real Workspace data) — TS will not set it.

## Slice plan (current)

- Slice 2 (in flight): stale-DB sweep + pool docs.
- Slice 2a DROPPED (Stylus): after slices 2+3 land, measure `-p 2/4/8` on
  one full `make test` each, then put the winner in the Makefile as
  `go test -p $(GO_TEST_P)` with `GO_TEST_P ?= winner` (suite/dev override).
  Stylus ruling: migrate-once (slice 3, posting first) outranks everything
  else in L7; its gate goes master-priority like slice 1.
- Slice 3 (ORDERED): posting migrate-once — `setup()` = testdb.New +
  full migrations, 27 tests incl settlement_test.go (largest). Same
  TestMain + sharedDatabase pattern as server/auth. Posting-alone
  before/after runs. Master-priority gate.

## Slice 3: posting shared-DB (IN PROGRESS)

Conversion (uncommitted): new `internal/posting/shared_test.go` (TestMain:
sweep + OpenShared + migrate + Freeze; `sharedDatabase` helper);
`setup()` + `settleSetup()` + 3 rapid properties converted (all 5
`store.Up` sites, all outside `rapid.Check`); `server`+`testdb` imports
dropped from both test files. Zero keepers: no `t.Parallel`, no DDL, no
migration-state tests; concurrent tests join workers (`wg.Wait`, channel
receives). `gofmt` clean, `go vet` OK.

| Run | Result |
| --- | --- |
| BEFORE posting-alone `-json` (scratch tree da178bb, suite, load 7→14) | GREEN 396.2s PASS. Top: TestPostValidation 171.7s (per-subtest setup), TestReverseClosedPeriods 29.5s, then ~6s/test migrate floor across 27 tests + subtests (81 nodes). Rapid bodies 6–10s. Full top-20 in `/tmp/ts-posting-before.json`. |
| AFTER posting-alone (master-priority gate, suite log 1124-193523) | GREEN 13.711s PASS, exit 0, 20s slot (~29x vs 396.2s before; shared migrated DB, zero keepers). |

## Slice 4: ledger/jobs/events/stock + server shared-DB (IN PROGRESS)

One commit: 4 new TestMains + `service()`/`setup()` conversions (zero
keepers in the 4 packages — no parallel/DDL/migration-state tests;
concurrent tests join; jobs disposal CREATE TABLE is Reset-dropped);
server converts brand/shop/public/identity-x2, keeps `dead` handle, kinds
legacy migration, sysmigrations x8, vocabulary rename (all with
why-comments) + newIsolatedServices (already documented). gofmt+vet clean.

| Run | Result |
| --- | --- |
| `affected` 8 pkgs (suite log 1124-225111, `-p 2` default), commit a0741da | GREEN exit 0, 607s slot: events 8.1s (was 655s), jobs 7.4s (was 653s), ledger 9.9s (was 857s), stock 32.3s (was 202-408s), posting 25.8s, server 228.2s (was 435-551s), auth 327.0s (was 639-657s — canary responding as the storm clears), cmd/athena 245.7s |

Stylus train-vs-gate data (both `-p 2`): posting 853s -> 33s in full gate.

## Slice 5 sketch: auth-waits + remaining DDL (ORDERED next)

Auth has ZERO testdb.New (4 shared fixtures + OpenShared): its
parallel-vs-alone gap is pure victim waiting. Remaining per-test DDL after
slice 4: files 1 fixture, secrets 1 fixture (both hazard-free; secrets
needs a Stylus order — in-flight lane), store migrate keepers,
cmd/athena ~25 NewWithConfig sites (no TestMain; migrate_test is a
migration-state keeper, rest need per-test shareability reads). Then the
`-p 2` vs `-p 4` shootout on one full run each.

## Slice 5a: auth profile + redundant-Up removal

BEFORE auth-alone `-json` (suite, load ~11): GREEN 81.1s PASS.
TestLastOwnerProperty 70.6s (87%), TestChallengeSingleUseProperty 2.2s,
production-hash round-trip 0.2s, rest ~0.1s. Full top-20 in
`/tmp/ts-auth-profile.json`.
Findings: ZERO testdb.New in auth; ZERO timers/sleeps in package (expiry
and lockout driven by SQL rows and fixed times; lockout tests loop 5
attempts without waiting); `auth.Service.Now` clock seam already exists
and challenge tests use it — clock injection has NO target.
Fix: drop redundant per-fixture/per-iteration `store.Up` on the already
migrated shared DB (ownerDB x100 race iters, challenge x2). Oracle
untouched; alone-time cut only (gate waits are external DDL-tax).

| Run | Result |
| --- | --- |
| AFTER auth-alone `-json` (suite) | GREEN 79.8s PASS (was 81.1s; owner property 70.6->68.7s — no-op Ups were ~13ms each; GET_LOCK churn per iteration gone). Alone-time cut only, as predicted. |

## Slice 5b: cmd shared-DSN conversion

TestMain (sweep + cheap hashing + shared migrate) + `sharedDSNEnv`
helper + additive `Shared.Config()` accessor; ~20 class-A sites
converted; keepers migrate/doctor-fresh/ledger-connector/5 workspace
tests with why-comments. gofmt+vet+build clean.

| Run | Result |
| --- | --- |
| AFTER cmd-alone `-json` (suite) | GREEN 44.2s PASS (61 tests). Top 7 are the keepers at ~5s each (5 workspace provisions + fresh-migrate connector + migrate-walk); every converted test <=0.36s. No cmd-alone before was run; before-story is gate history (844s @-p8 -> 246s @-p2). Full list in `/tmp/ts-cmd-after.json`. |

## Slice 5c: -p 4 full retry (suite log 1125-031450)

`env GOFLAGS=-p=4 make test CACHE_DIR=<shared>` (explicit `-p=` wins
over the suite `-p 2` default): GREEN exit 0, **513s wall** (8.55 min)
vs the -p 2 TSs4 gate 836s (13.9 min) — **-p 4 wins by 39%**.
Per-package: auth 464.2s, server 271.8s, files 137.9s, cmd/athena 123.4s,
secrets 98.9s, posting 58.3s, stock 38.7s, events 36.3s, ledger 22.1s,
jobs 19.9s, store 3.5s, testdb 1.0s, rest <1s; web 74 files / 717 tests.
7-min target NOT yet met: auth (464s) remains the pole; remaining DDL
storm is files/secrets per-test migrates + cmd workspace keepers —
next-slice candidates. Makefile `GO_TEST_P ?= 4` needs a Stylus order.

## Slice 5 read-only prep (no commits until slice 4 merges)

Perf probe `/tmp/ts-perf-probe.sh` (`once`|`watch`, validated 2026-11-24):
read-only processlist + lock-counter samples every 5s into
`/tmp/ts-perf/<gate>/`, DSN from the tree's `.env.local`, never printed.
INNODB_TRX/metadata_locks/waits/INNODB STATUS need PROCESS (1227, no
admin path here) — processlist states + counters carry the attribution;
user can GRANT PROCESS if deeper data is ever needed. Baseline counters
23:04: row_lock_waits 60022, row_lock_time 2001494ms, table_locks_waited
2001. Watcher armed for the next Go run; rerun `watch` after each catch.

First catch (w-c7 stock-only affected, 25 samples/2 min, `/tmp/ts-perf/1124-230522-*`):
pure per-test migration DDL on fresh stamped DBs (CREATE TABLEs, ALTER/
RENAME incl. the vocabulary_rename migration, DROP DATABASE churn), +527k
queries, row-lock waits +2, no MDL waits (single package, no contention
partner). Auth-waits attribution needs a gate with auth running — still
to catch.

files survey: mixed `package files` (pure) + `package files_test`
(newFixture: New+migrate+files.Open on t.TempDir) — clean TestMain
conversion, no parallel/DDL/goroutines.

cmd/athena survey (all `package main`, 19 files, no TestMain): each test
does NewWithConfig + t.Setenv DSN + t.Chdir(temp) and runs CLI commands
in-process, migrating via `migrate up` itself (no store.Up in prod code).
Shareable shape: shared pre-migrated DB + per-test Reset + shared-DSN env,
needs a `Shared` DSN/cfg accessor (additive test-helper method).
Keepers/flags: migrate_test (partial ups, fresh-status asserts); doctor
likely asserts pending-migration warnings (verify); new_test +
identity create whole workspaces with no visible DROP cleanup — read
fully at implementation (server pollution + share-blocker risk);
events/jobs "pending" hits are status words, not migration state.
- Slice 4 (ORDERED): ledger/jobs/events/stock in ONE commit, plus the 17
  testdb.New calls left in server (brand/identity/etc.). testdb.New kept
  ONLY for tests that change migration state, with a why-comment.
  Survey: ledger/jobs/events/stock each have ONE setup fixture (same
  mechanical conversion x4); server stragglers in brand/helpers/identity/
  kinds/public_shop/public + sysmigrations x8 + vocabulary_rename (the
  migration files stay keepers with why-comments; helpers/identity need
  per-test reads). Starts after slice-3 merge.
  Rank from F7s1 full gate (-p 8, CONTAINS slice 1, verified by ancestry):
  posting FAIL 1201s (timeout), auth 1212s, server 917s, ledger 857s,
  cmd/athena 844s, events 655s, jobs 653s, stock 582s, files 414s,
  secrets 364s, store 8.6s, rest <1s. Auth at 1212s WITH cheap hashing
  (188s alone) proves the wait is the shared DDL storm, not auth itself:
  auth is the canary, not a target (already shared+cheap).
- Later: lock-wait investigation (GET_LOCK 10s schema/owner locks,
  per-session timeout precedent in `stock/movements_test.go:718`, workers
  5s poll), sleep/clock injection, `-p 4` before/after numbers from gate
  history.
- Slice 5+: fixes from slice 3, by package.

## Ranked win list (preliminary; serial per-package numbers still pending)

1. Argon2 test params (slice 1) — kills the parallel-run starvation; ~490s
   posting delta (C7 670s vs F3 1161s) is the prize, plus auth itself.
2. Stale-DB sweep + 512M pool — already applied at runtime by Stylus; slice 2
   persists (TestMain cleanup + docs).
3. Shared-DB TestMain for posting/ledger/cmd (auth+server already have it) —
   unmeasured; needs slice-0-style per-package numbers before slicing.
4. `t.Parallel` / rapid-iteration DB reuse — unmeasured; isolation must be proven.
