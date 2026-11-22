# Review brief: C1 slice 3 DELTA re-review (rebase adaptation only)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1, branch
feat/c1-posting (commit ccaf482). Read AGENTS.md,
.kiro/specs/athena-beta/design.md section 5 reviewer workflow, the
slice brief ../athena-erp/.agents/tasks/beta-c1/brief.md (slice 3
only), and the full slice-3 review
../athena-erp/.agents/tasks/beta-c1/review-slice-3.md (Verdict PASS
for commit 5bbc94e — that review stands for the slice logic; do NOT
redo it).

## Why this delta review exists

The merge candidate ccaf482 is NOT the reviewed tree. Sequence:
5bbc94e (reviewed, base 3dece91) → 981ee78 (rebased onto 722e183,
which merged C2 slice 2's event bus) → ccaf482 (rebased onto
1e25ab9; 722e183..1e25ab9 is .agents docs only, so ccaf482 is
product-identical to 981ee78). The first rebase needed a MANUAL
adaptation: slice 3 generalizes insertOutbox with (eventType, extra)
params and an outboxPayload helper, but the new base had already
rewritten insertOutbox onto events.Emit with (workspace, opts).
Reproduce the delta yourself:

  git diff 3dece91 5bbc94e > /tmp/rev.patch
  git diff 722e183 981ee78 > /tmp/cand.patch
  diff /tmp/rev.patch /tmp/cand.patch

(all inside internal/posting: posting.go, posting_test.go,
reverse.go, write.go; context shifts plus the insertOutbox/outboxPayload
adaptation).

## Scope: the adaptation only

1. The candidate's insertOutbox(ctx, tx, workspace, eventType, extra,
   groupID, hash, plan, opts) routes through events.Emit with a
   parameterized EventType and Payload outboxPayload(extra, ...).
   Verify: (a) the posted-event call (extra nil) emits a payload
   equal to the base's inline map (same keys/values; key order
   irrelevant); (b) the reversed-event call carries
   reversal_of_group_id and the same base keys; (c) envelope fields
   (Workspace, Producer "posting", Subject*, Actor, CorrelationID)
   match the base behavior for posting events.
2. Verify the OutboxEventType / OutboxEventTypeReversed consts now
   alias the events package consts (no forked event-type strings),
   and that reverse.go call sites pass the right event type each.
3. Confirm nothing else semantic changed in the delta (context-line
   shifts only elsewhere) and Z2 handling is untouched
   (ErrCorrectionWindowOpen, no invented policy).
4. Rerun the posting package suite with MariaDB
   (ATHENA_TEST_DATABASE_URL, throwaway test_athena_* only, never
   the dev database; -timeout 20m; the posting package alone took
   ~6 min last time). Do NOT run the full make test — the master
   runs it once at merge time (suites run one at a time overnight).
5. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make generate twice with no diff. If node is missing on PATH,
   source nvm first. No browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong adapted
semantics, payload/envelope mismatch, broken build, red posting
package); the rest are follow-ups. Tree must be clean at ccaf482
before any gate (an untracked rapid .fail seed was already removed
by the master; if a new one appears, report its content — it means
a property test failed).

Write findings to
../athena-erp/.agents/tasks/beta-c1/review-slice-3-delta.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "C1 SLICE 3 DELTA
REVIEW: PASS or NEEDS_CHANGES <one line>". Then wait for
instructions. Do not edit product code, commit, merge, push or
start agents. Permission policy: stay inside athena-erp and
athena-erp-* folders, nothing destructive.
