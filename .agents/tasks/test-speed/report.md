# Report: test-speed (make the MariaDB suite fast enough for make test)

**Status: Final.**

Worktree: `/Users/iampaulking/projects/duchess/athena-erp-test-speed`, branch `fix/test-speed`,
from main `19c0652`. Nothing committed, merged or pushed, no agents started. The B4B12 worktree
was left untouched. No product code changed — test infrastructure and test helpers only.

## Verdict: PASS

Both package targets met with margin and full `make test` is green.

## What was slow

Only one test created a database per rapid iteration: `TestLastOwnerProperty`'s race phase
(`internal/auth/ownership_test.go` via `ownerDB` → `testdb.NewScoped` + full migrate + seed +
drop, 100 iterations). All other rapid tests (`auth`, `challenge`, `ledger`, `rules`, `store`)
are pure logic or already share one database across iterations. Measured: fresh+migrate+seed
costs ~2.5 s, so the race phase alone spent ~250 s on database churn.

The rest of the bulk was the same shape at package scale: every test in `internal/auth` and
`internal/server` created and migrated its own database (~140 server tests × ~3 s migrate).

## What changed

- `internal/testdb/testdb.go`: new `Shared` handle. `OpenShared()` (for `TestMain`; nil when
  `ATHENA_TEST_DATABASE_URL` is unset so pure tests still run), `Freeze()` (snapshots the
  migrated table list plus the migrations' seed rows), `Reset()` (drops post-freeze tables such
  as Record Type tables, truncates the rest with counters restarted, restores seed rows, keeps
  the migrations ledger; foreign keys off on the reset connection only), `Close()`
  (nil-safe). Existing `New`/`NewWithConfig`/`NewScoped` behavior unchanged (small internal
  refactor to share creation code).
- `internal/server/shared_test.go` (new): `TestMain` opens, migrates once and freezes the shared
  database; `sharedDatabase(t)` resets and returns it, skipping when MariaDB is unavailable.
- `internal/auth/shared_test.go` (new, package `auth_test`): same shape.
- Converted fixtures to `sharedDatabase(t)`: `newAccessFixtureAt`, `newServices`,
  `newRuleFixture`, 3 direct sites in server `ledger_test.go`, `challengeService`,
  `TestChallengeSingleUseProperty`, `ownerDB` (no longer returns a cleanup func; serial phase
  still accumulates across iterations, race phase still resets per iteration).
- Deliberately NOT converted (need their own migration/schema state): all of
  `sysmigrations_test.go`, `vocabulary_rename_test.go`,
  `TestMigrationGivesLegacyRecordTypesTheMasterKind`, and
  `TestRecordChangeAndHistoryShareATransaction` (drops `athena_audit_log` on purpose; uses new
  `newIsolatedServices` helper with a private database).
- `Reset` empties tables with `DELETE` plus counter-restore instead of `TRUNCATE` (identical
  results for tests; no triggers exist in the schema): measured 164 ms vs 865 ms for the same
  42 tables while a competing suite ran, because it avoids dictionary locks. `Reset` also
  fails fast naming the culprit if a test drops a platform table.
- Server `TestMain` additionally lowers `auth.DefaultParams` to the cheap params the property
  tests already use (server tests never assert on hashing cost; verified by grep). The auth
  `TestMain` does NOT: its binary also runs package auth's password tests, which pin the
  production Argon2id cost (verified — a first attempt broke them and was reverted).
- Two real isolation bugs found by the suite during conversion and fixed: (1) migrations seed
  rows (audit chain head) that emptying wiped — `Freeze` now snapshots and `Reset` restores
  them, including empty-string values that must not become NULL; (2) covered by the
  isolated-services helper above.
- Property iterations unchanged (100) and every assertion untouched.

## Before / after (`go test -count=1`, this machine)

| Package | Before | After | Target |
| --- | --- | --- | --- |
| `internal/auth` | 704.1 s PASS (`TestLastOwnerProperty` 627.9 s) | **104.5 s PASS** | < 180 s ✓ |
| `internal/server` | 502.4 s PASS (clean main `19c0652`) | **97.7 s PASS** | < 360 s ✓ |
| `TestLastOwnerProperty` alone | 535.2 s (main `ee66539`, earlier task) | 101.9 s PASS | — |

Before/after pairs were run back-to-back under the same load (16:19–16:32, load 5–9 from the
operator's own desktop builds; no competing suites). Structural, load-independent win measured
directly: per-fixture migrate+seed ~2.5 s → reset+seed ~90 ms (28×); per-race-iteration body
~162 ms; server hashing override on top. 100 iterations and all assertions kept.

Caveat: absolute times on this shared machine swing with ambient load (a competing full suite
inflated runs up to 6× during this task, including one 20-minute auth timeout mid-task that
passed at 104.5 s once the competitor finished). The DELETE-based reset was chosen partly for
this: 164 ms vs 865 ms for the same 42 tables while a competitor ran.

## Checks

- `gofmt -l internal cmd`: my files clean; two pre-existing unformatted files on main untouched
  (`internal/ledger/determination_test.go`, `cmd/athena/ledger.go`).
- `go build ./...` → exit 0. `go vet ./...` → exit 0.
- `make test` → exit 0: `cmd/athena` 132.3 s, `internal/auth` 291.9 s, `internal/files` 95.2 s,
  `internal/ledger` 128.1 s, `internal/server` 209.4 s, `internal/store` 6.2 s, rest < 2 s or
  no test files; web 248/248 passed. (Parallel run, so per-package times exceed the sequential
  before/after pairs above; every package is far inside the 20-minute timeout now.)
- Makefile timeout untouched (still 20 m).
