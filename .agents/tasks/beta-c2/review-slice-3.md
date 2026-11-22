# Review: C2 slice 3 — job runner + occurrences + worker startup (8.4+8.5)

Verdict: NEEDS_CHANGES

Scope: worktree athena-erp-c2, branch feat/c2-events-jobs, commit 3a9a3a1
(clean tree before and after every gate). Diff `0ac599e...HEAD` is the
expected 11 files, all Go (no web/, no proto/ — verified by diff stat).
References: brief slice 3, batch-2 design G8, requirements R8.4/R8.5/R8.9/
R8.10/R8.19, ADR-0028, migration 17, design.md section 5.

## Blockers

- B1. `internal/jobs/runner.go:310-315`: `Recover` parks expired leases
  dead with `attempts >= LEAST(max_attempts, ?)`, where `?` is the runner
  default (5). Every other path is row-first (`maxAttempts`,
  runner.go:339 — "caps attempts per job when the row does not say
  otherwise"; `exhausted`, runner.go:335; the reclaim check,
  runner.go:168). A job with row `max_attempts = 10`, `attempts = 7` and
  an expired lease is parked dead by a restart, while the live reclaim
  path would run it (8 < 10). Restart recovery contradicts the runner's
  own documented MaxAttempts contract and drops work before its
  configured attempts are used (R8.4). Note the runner maintains
  queued/leased => attempts < max_attempts (fail/reclaim/parkDead all
  enforce it), so with a row-first threshold the dead branch only fires
  for hand-edited rows — as a safety net should. Suggested fix: mirror
  `maxAttempts` in SQL —
  `attempts >= CASE WHEN max_attempts > 0 THEN max_attempts ELSE ? END` —
  plus a test (MaxAttempts 10, attempts 7, expired lease => released,
  not dead). The existing recover tests still pass under this fix
  (row max 3 <= runner default in both).

## What was verified

1. Leases/fencing/backoff (`runner.go`, R8.4): token-based claim over
   queued+due rows, then expired-lease reclaim with an attempt counted
   (anti-poison, documented); `finish`/`fail`-retry/`release` all carry
   `AND fencing = ?`; `parkDead` has no fencing predicate but converges
   (the reclaim path parks dead rather than running once attempts are
   exhausted, so no runner executes work another parks). Backoff
   2^attempts seconds capped at 5 min matches the outbox convention;
   unknown kinds sit queued, never dropped. Owner assignment is
   job-then-runner-default. One span carries ids/outcome only, no
   payload values.
2. Occurrences (`occurrences.go`, `jobs.go:227-258`, R8.9): unique key
   over Workspace/schedule/scope/period enforced by the table PK with
   duplicate-key mapped to already-stored; timezone validated by
   `LoadLocation` and kept on the row; catch-up skip/run with string
   comparison over ISO-8601 periods (documented); `DispatchDue` leases
   the earliest pending occurrence and enqueues its job in one
   transaction under `SELECT ... FOR UPDATE`, so one occurrence runs
   exactly one job; completion/death propagate back with watermark and
   result; `RecoverOccurrences` heals leased rows (done/dead/refreshed).
   All column names match migration 17.
3. Property 15 (`TestOccurrenceUniquenessProperty`): rerun with
   `-count=1` and the database → PASS (16/16 jobs tests pass, 66.8s),
   covering concurrent-ensures-store-one plus distinct-periods-distinct.
4. Server startup/shutdown (`workers.go`, `server.go:128`): recovery
   runs synchronously at boot, the drain loop exits on ctx cancel
   (Serve already shuts down on ctx.Done), events only tick with
   registered handlers, purge enqueues on the first tick via
   `EnqueueIfAbsent`. `startWorkers` is called only from `Serve` and
   its own test — no test starts workers implicitly. `workers_test.go`
   drains a real job and a purge end to end → PASS.
5. Purge (`purge.go`, R8.19): deletes expired-or-consumed Challenges
   and sessions past absolute or idle expiry; the session predicate is
   the exact negation of the live-session checks (`service.go:1313`,
   `security.go:268`); keeps one minimal `expired_purged` row with a
   fixed detail and no secret; counts go to the job result, not the
   log. Purge test → PASS (only dead rows go, one ok row logged).
6. Slice-2 F1/F2 (`dispatch.go`): both fixed as comment-only changes
   (F1 now honestly states row-level exhaustion with per-subscription
   behavior not honored yet; F2 documents the retry-first order). No
   behavior change — verified in the diff. Slice-2 F3/F4 carry over.
7. Gates: `gofmt -l cmd internal` empty; `go build ./...`, `go vet
   ./...` ok; FULL `make test` through `.agents/bin/suite` → exit 0
   (884s; 17 Go packages ok, zero FAIL/panic lines; web 55 files /
   563 tests passed); `make generate` twice with no diff; tree clean
   at 3a9a3a1 afterwards, no new `.fail` seeds. No browser pass (no
   Desk in this slice, per brief).
8. Z4/Z2/deferrals: diff holds no secret/custody/retention/disposal/
   encryption or backdating/book-policy terms (one grep hit is the
   pre-existing "not secrets" comment); 8.8/8.9/8.10 still deferred,
   Z2 untouched. R8.10's recheck-under-lock is a documented handler
   contract (`runner.go:15-22`), with no scheduled-expiry handler in
   this slice — correctly nothing to enforce yet.

## Follow-ups (non-blocking, for the worker or a later slice)

- F1. `runner.go:27-30`: `Runner.Errors` is set (`workers.go:49`) but
  never read. Remove it or use it.
- F2. `DispatchDue` (`occurrences.go:169-173`) writes the occurrence
  identity into the caller's payload map (`nonNilMap` returns a
  non-nil map as-is). Copy before adding.
- F3. `athena_jobs.occurrence_key` is VARCHAR(280) but `OccurrenceKey`
  can reach ~600 chars (140+140+255+64+separators). Long Workspace/
  schedule/scope names fail dispatch with error 1406. Widen the column
  or bound the parts.
- F4. `Recover` (`runner.go:322-324`) releases expired leases without
  counting an attempt, while the live reclaim path counts one. A job
  that crashes the process on every run never reaches dead across
  restarts. Consider `attempts = attempts + 1` on release to match
  reclaim accounting (leniency direction today: at-least-once holds).
- F5. `claim` (`runner.go:136-139`) sorts NULL `next_run_at` (fresh
  jobs) after retried jobs — same shape as the dispatch.go F2 the
  worker just commented. One-line comment for symmetry.

## Database safety

Everything ran in throwaway `test_athena_*` databases. The dev
database was never migrated, served or written by this review (tests
plus the suite run only).
