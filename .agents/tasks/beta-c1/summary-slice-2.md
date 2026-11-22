# Summary: C1 slice 2 — Operation_ID contract + idempotent replay (6.4+6.5)

Merged 2026-11-22 as f94fe09+367a56c (built at 17e10ab+33a5447,
rebased cleanly onto 2e7b156).

- `internal/posting/operations.go` (new): idempotency-key contract —
  same (command, operation_id, actor) replays the stored result;
  group-collision winner disambiguated BY GROUP ID (fix loop 1: the
  first version's bare LIMIT 1 could replay the wrong voucher's
  result across actors sharing one op-id string).
- Concurrent-replay property + 3-actor regression test (fails 3/3
  pre-fix, passes post-fix, verified by the reviewer both ways).
- Posting-log outcome coverage (posted/replayed/conflict/rejected/
  error), failures logged via a separate connection so rows survive
  rollback (R6.6). Nil Dialect degrades gracefully on the
  classification path (slice-1 follow-up 5).
- Z2 unchanged: no window/policy invented, both failed_precondition
  sentinels still pinned, distinct rejected codes.
- Review: round 1 NEEDS_CHANGES (1 correctness blocker, the LIMIT 1
  bug); round 2 PASS after the fix (review-slice-2.md). F1–F5
  non-blocking, recorded as follow-ups.
- Merge-time verification on 367a56c: gofmt empty, go build/vet
  clean, make test exit 0, make generate twice with no diff.
- Open: slices 3–6 (reversal, posting hooks, RPCs/CLI, voucher view).
