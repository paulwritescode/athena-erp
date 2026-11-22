# Review: C2 slice 2 — event bus + emit hooks + Property 13

Verdict: PASS

Scope: worktree athena-erp-c2, branch feat/c2-events-jobs, commit bb3cedd
(clean tree). Diff `2e7b156...HEAD` is the expected 14 files, all Go plus
one rapid seed (no web/, no proto/ changes — verified by diff stat).

## Blockers

None.

## What was verified

1. `internal/events/` bus semantics match migration 17 + R8.2/R8.3/R8.4:
   - `Emit` (`events.go:93`) validates type/producer/subject, defaults id
     (UUIDv7), correlation and schema version, and writes the envelope +
     pending outbox row in the caller's transaction — commits or rolls
     back with the business change (R8.1/R8.2). Rollback test included.
   - `Claim` (`events.go:168`) inserts the inbox row and reports
     duplicate-key as already-claimed (R8.3); `Claimed` read serves
     replays/checks only.
   - `Declare`/`Match` (`events.go:213,249`) store and fan out mode,
     action, effect key, exhaustion behaviour and declaring Pack (R8.6).
   - Dispatcher (`dispatch.go`): token-based lease claim over
     pending+due and expired-lease rows, per-consumer Claim+handler in
     one transaction with redelivery skip, retry with backoff
     (2^attempts seconds, 5 min cap), park-dead at max attempts
     (default 5), legacy (migration-14-shaped, event_id NULL) row
     recovery under the outbox row id. Crash-between-commit-and-dispatch
     recovers via pending rows (R8.5). No fencing on outbox leases is
     correct here: fencing lives on `athena_jobs` (slice 3); outbox
     delivery is idempotent via inbox claims.
   - Telemetry: one span with ids/counts/outcome only
     (`athena.event_id`, `athena.event_type`, `athena.outcome`,
     `athena.subscriptions`, `athena.failed_consumer`); no payload,
     subject or Record values. Tracer name `athena/events` matches the
     `athena/kernel`, `athena/store` convention.
2. Emit hooks: posting `insertOutbox` now emits through `events.Emit`
   with a byte-identical payload key set to the old direct INSERT plus
   the linked envelope (`write.go` diff); `OutboxEventType` is now the
   shared `events.TypePostingPosted` constant (same string). Lifecycle
   `Event` gains Actor/Workspace/Revision, set in `transition()`
   (`server/lifecycle.go`) with the post-transition revision; hooks
   registered for OnSubmit/OnCancel (in-tx steps) in both kernel and
   RPC paths with the Workspace slug threaded through
   (`kernel.go`, `server.go`, `records.go`, `durable_events.go`). A hook
   error fails the transition and rolls everything back. Posting
   `Engine.Workspace` has no production constructor yet (no caller
   outside tests — C1 integration still open), so the blank default is
   moot today; lifecycle envelopes carry the real slug.
3. Property 13 (`TestInboxDeduplicationProperty`): rerun with
   `-count=1` → PASS ("OK, passed 100 tests", 6.5s), covering
   concurrent-claims-admit-one plus dispatcher-redelivery-applies-once.
4. Suite at 20m vs below: `make test` (which sets `-timeout 20m`)
   → exit 0, Go all-pass plus web 55 files / 563 tests passed (node
   v26.7.0 via nvm; web covered despite the worker being unable to).
   A separate `go test -timeout 10m ./...` run fails with ONLY
   `panic: test timed out` in `internal/auth`, `internal/posting` and
   `internal/server` — zero `--- FAIL` lines, i.e. timeout-only, no
   logic failures. Worker's note confirmed and extended (not just
   posting). Uncached `-count=1 -timeout 20m` rerun of those three
   packages: all ok (auth 309s, posting 259s, server 256s).
5. Committed rapid seed
   (`testdata/rapid/TestInboxDeduplicationProperty/...-79698.fail`):
   genuine rapid artifact (draw labels match the test; header
   `v0.4.8#...` equals rapid v1.3.0's fail-file format version in
   `persist.go`, so it IS replayed, not ignored). It replays green
   inside the passing property run above — a regression pin, not a
   failing seed. Not a blocker.
6. Z4/Z2: diff contains no secret/retention/custody/disposal/encryption
   or backdating/book-policy terms; 8.8/8.9/8.10 still deferred, Z2
   untouched. Posting edits are minimal and additive per the brief.
7. Standard checks: `gofmt -l cmd internal` empty; `go build ./...`,
   `go vet ./...` ok; `make generate` twice with no diff; tree clean
   afterwards (my runs wrote no new `.fail` files). No browser pass
   (no Desk in this slice, per brief).
8. Database safety: everything ran in throwaway `test_athena_*`
   databases. Dev `athena_erp` sits at version 17 via the slice-1
   merge (master action), untouched by this review (tests + SELECTs
   only, never `migrate up` or `serve`).

## Follow-ups for later slices (non-blocking)

- F1. `dispatch.go:225-227`: the `fail` comment says "ExhaustSkip parks
  silently", but the code always records `last_error` and no path
  reads a subscription's `ExhaustBehavior` (row-level status cannot
  express per-consumer exhaustion anyway). Fix the comment or design
  per-subscription exhaustion handling in a later slice.
- F2. `claim` (`dispatch.go:137`) orders `available_at IS NULL` last,
  so retried rows sort ahead of fresh pending rows. Harmless (every
  due row is eventually claimed) but worth a comment if intentional.
- F3. `DispatchNext` marks a row `dispatched` even when none of its
  subscriptions has a registered handler (`dispatched_noop`). The
  "handlers are code, not state" rationale is documented; just confirm
  the master accepts that a deploy missing a handler drops (rather
  than parks) that consumer's events.
- F4. Suite wall-time: three packages each need 4–6 minutes of the
  20m budget on this machine. If later slices add more DB tests, the
  20m budget or package parallelism will need attention before E2.
