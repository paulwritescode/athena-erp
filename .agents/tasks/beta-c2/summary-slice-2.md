# Summary: C2 slice 2 — event bus + emit hooks + Property 13

Merged 2026-11-22 as 6be771e (built at bb3cedd, rebased onto 3dece91
with 3 posting-file conflicts resolved as union + the new
insertOutbox signature).

- `internal/events/` (new): envelope + producer Emit in the caller
  transaction (commits/rolls back with the business change),
  inbox Claim with effect keys, Pack-declared subscriptions
  (Declare/Match), dispatcher with lease claim, backoff
  (2^attempts, 5 min cap), park-dead at max attempts, legacy
  outbox-row recovery. Telemetry carries ids/counts/outcome only.
- Emit hooks: posting `insertOutbox` emits through `events.Emit`
  (byte-identical payload + linked envelope); lifecycle
  submit/cancel emit via registered hooks in kernel + RPC paths;
  hook error fails the transition and rolls back.
- Property 13 inbox dedup (concurrent claims + redelivery):
  PASS, rerun by the reviewer; the committed rapid `.fail` seed
  replays green (regression pin, format v0.4.8).
- Suite note (reviewer-confirmed): `make test` (20m budget) exit 0
  incl. 563 web tests; a 10m run fails timeout-only (zero FAIL
  lines) in auth/posting/server — logic is green, budget is tight.
- Z4/Z2 clean: no secrets/retention/custody/disposal or
  backdating/book-policy terms; 8.8/8.9/8.10 still deferred.
- Review: PASS, no blockers (review-slice-2.md). 4 follow-ups
  recorded (exhaust-comment drift, retry sort order, noop-dispatch
  acceptance, suite wall-time).
- Merge-time verification on 6be771e: gofmt empty, go build/vet
  clean, make test exit 0, make generate twice with no diff.
- Open: slices 3+ (jobs/outbox worker, workflows, secrets/lifetimes
  once Z4 is decided).
