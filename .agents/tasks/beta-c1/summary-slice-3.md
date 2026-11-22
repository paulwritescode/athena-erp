# Summary: C1 slice 3 — reversal mirrors + correction-period rules (6.6+6.7)

Merged 2026-11-22 as 4334fc3 (built at 5bbc94e, rebased twice with a
manual insertOutbox adaptation: 981ee78 onto 722e183, ccaf482 onto
1e25ab9, 4334fc3 onto dac61fe; rebases 2 and 3 byte-identical).

- `internal/posting/reverse.go` (new): Reverse unwinds a posted
  submit group by mirroring stored rows with swapped sides under the
  cancel event, flagged is_reversal with the reversed row identity.
  The mirror inherits the submit's validity so an unwind succeeds
  after master changes; only the reversal date's period must be
  open (default: the submit date, overridable per call).
- Reversals share the operation/group/log machinery: replays,
  conflicts and post-once-per-cancel hold; a retry replays even
  after the period closes (test-pinned).
- Property 10: submit+cancel net to zero with exact mirrors
  (8 new reversal tests + rapid Properties 8, 9, 10).
- Outbox: insertOutbox generalized with (eventType, extra) and an
  outboxPayload helper; the rebase adaptation routes it through
  the C2 events.Emit bus (workspace + opts) with a payload
  byte-equal to the base's inline map plus reversal_of_group_id
  on the reverse path.
- Z2 unchanged: hard-closed reversal period returns
  ErrCorrectionWindowOpen; no correction target guessed.
- Review: full slice PASS at 5bbc94e (review-slice-3.md) plus a
  delta re-review PASS of the rebase adaptation at ccaf482
  (review-slice-3-delta.md), after the master found the candidate
  differed from the reviewed tree. Follow-ups recorded in tasks.md.
- Merge-time verification on 2837a38 (identical patch to 4334fc3):
  gofmt empty, go build/vet clean, make test exit 0 (quiet run;
  the clock-out red is attributed to parallel-suite contention),
  make generate twice with no diff, web typecheck + build clean.
- Open: slices 4–6 (hooks, RPCs/CLI, voucher view).
