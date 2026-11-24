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

## Ranked win list (preliminary; serial per-package numbers still pending)

1. Argon2 test params (slice 1) — kills the parallel-run starvation; ~490s
   posting delta (C7 670s vs F3 1161s) is the prize, plus auth itself.
2. Stale-DB sweep + 512M pool — already applied at runtime by Stylus; slice 2
   persists (TestMain cleanup + docs).
3. Shared-DB TestMain for posting/ledger/cmd (auth+server already have it) —
   unmeasured; needs slice-0-style per-package numbers before slicing.
4. `t.Parallel` / rapid-iteration DB reuse — unmeasured; isolation must be proven.
