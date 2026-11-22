# Review brief: C2 slice 2, event bus + emit hooks + Property 13

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c2, branch
feat/c2-events-jobs (commit bb3cedd). Brief:
../athena-erp/.agents/tasks/beta-c2/brief.md (slice 2 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md. Design reference: .kiro/specs/athena-batch-2/design.md G8;
ADR-0028.

Review the slice-2 diff (git diff 2e7b156...HEAD — 14 files, all Go):

1. `internal/events/` (new): envelope, producer Emit in the caller
   transaction, inbox Claim with effect keys, Pack-declared
   subscriptions, dispatcher with lease claim, backoff and legacy
   outbox-row recovery. Envelope/lease/occurrence semantics must
   match migration 17 + R8.2/R8.3/R8.4.
2. Emit hooks: posting writes the envelope beside its outbox row;
   lifecycle submit/cancel emit through registered hooks
   (`internal/server/durable_events.go`, kernel/lifecycle/records/
   server wiring).
3. Property 13 inbox dedup (concurrent claims + redelivery): rerun
   the property (worker notes the Go suite needs -timeout 20m;
   default 10m kills posting tests — confirm the suite passes at 20m
   and fails only on timeout below that, not on logic).
4. Confirm NO web/ or proto/ changes in the diff (worker claims
   none; it could not run web tests — run the FULL make test
   yourself with node via nvm, do not skip web).
5. Check the committed rapid `.fail` seed
   (`internal/events/testdata/rapid/.../TestInboxDeduplicationProperty-*.fail`):
   intentional regression corpus or accidental commit of a failing
   seed? If the property currently fails from that seed, that is a
   blocker; if it passes and the seed only pins a found case, say so.
6. Z4: no secrets/key-custody/retention policy invented; 8.8/8.9/
   8.10 still deferred. Z2 untouched.
7. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test with MariaDB enabled (throwaway test_athena_* only,
   -timeout 20m for the Go suite), make generate twice with no diff.
   No browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong bus
semantics, broken hooks, failing property, red suite); the rest are
follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c2/review-slice-2.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: hc send master "C2 SLICE 2 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge,
push or start agents. Permission policy: stay inside athena-erp and
athena-erp-* folders, nothing destructive.
