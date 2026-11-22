# Summary: C1 slice 4 — lifecycle hooks + Posting Rule type (6.8)

Merged 2026-11-23 as 8fd7bef (built at a3691da on f28e953,
rebased twice cleanly — first onto 503a8f3, then onto 64bb3d2
after C4-1 landed; both rebases byte-identical).

- Lifecycle hooks: SubmitRecord posts via posting Rules and
  CancelRecord mirrors in-transition, in the same transaction
  before commit (R6.4/R6.5/R6.11). DEVIATION ACCEPTED:
  `internal/lifecycle/` untouched — it is storage-agnostic by
  design, so the hooks live in `internal/server/lifecycle.go`
  (transition calls postVoucher/reverseVoucher joining the
  caller's tx, never committing). Only Submit/Cancel reach
  transition(); Amend posts nothing.
- Posting Rule type (`internal/rules/`): post() entry returning
  a plan dict (shape judged, values left to the engine),
  fixtures() required at compile, RunFixtures on an isolated
  host.
- Activation gate (`rules_engine.go`, `server/rules.go`):
  generic engine skips posting Rules; postingPlans runs enabled
  ones in rule order with stable operation ids; gatePostingSave
  replays fixtures on activation, lands changed plans disabled
  with a capped diff, and same-source re-save with enabled is
  the named approval. Versions + audit persist without a new
  migration (athena_rule_versions + writeAudit in the
  caller's tx).
- Preview-hash check (R6.15): records.proto gains
  posting_preview_hash only (+5); no new procedure, no gate
  entry; stale hashes refused with FailedPrecondition.
- Small additions: posting HashPlans export (canonical-hash,
  scale-insensitive); find() parent_id filter for voucher-line
  reads (error branches untested — follow-up).
- Z2: sentinels VERIFIED byte-identical (posting.go/reverse.go/
  operations.go zero diff); nothing invented, nothing skipped.
- Four-eyes (review F4): same-user approval ACCEPTED for beta
  (Stylus); author (v2 actor) + approver (v3 actor) both kept
  and auditable; Z13 opened for the policy decision.
- Review: PASS, no blockers (review-slice-1.md). Reviewer ran
  gates in a pristine /tmp scratch worktree (branch carried
  stacked slice-2 WIP), reused the worker's green run via
  suite dedup (first live reuse), plus uncached reruns of the
  key posting/rules/server tests. F1–F3 recorded as follow-ups.
- Merge-time verification on 8fd7bef (identical patch): full
  make test exit 0 (second gate; the first green gate was lost
  to the C4-1 under-merge, which motivated the merge-train
  rule). Merged the slice-1 commit only; slice 2 stays stacked.
- Open: slices 5–6 (6.9 RPCs/CLI, 6.10 voucher view + docs).
