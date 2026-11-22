# Review brief: C1 slice 2, Operation_ID contract + idempotent replay (6.4+6.5)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1, branch feat/c1-posting
(commit 17e10ab, on top of merged slice 1). Brief: ../athena-erp/.agents/
tasks/beta-c1/brief.md (slice 2 only: batch-2 G6.4+G6.5). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.
Design reference: .kiro/specs/athena-batch-2/design.md G6; requirements
R6.7–R6.10 (batch-2 requirements.md); ADR-0023 (OPEN backdating/book
policy — Z2 rule still binding).

Review the slice-2 diff (git diff 35061c0...HEAD on this branch — slice 1
is already in main, do not re-review it):

1. Operation_ID contract: canonical payload hash (Decimals normalized
   before hashing — review follow-up 4 from slice 1), replay returns the
   stored result, conflicting reuse rejected, uniqueness per voucher
   revision/event/book. Groups/log rows as designed.
2. Idempotent-replay property test (Property 9): sequential and concurrent
   replays, conflicts, second-operation replay, racing fresh posts — real
   rapid-style property coverage with the race actually exercised.
3. Duplicate-key checks degrade without a Dialect instead of panicking
   (follow-up 5); balance rows lock in one global order (no deadlock
   across concurrent posts — reason about the order).
4. Z2 still clean: no backdating window or book policy invented; the two
   failed_precondition sentinels still pinned.
5. No new migration/ADR (none expected); confirm no schema changes and no
   edits outside internal/posting/.
6. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test with MariaDB enabled (throwaway test_athena_* databases
   only, never the dev database in .env.local) — incl. the new property
   tests unskipped, and make generate twice with no diff. No browser pass
   (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong replay semantics,
lost/conflicting writes, deadlock, invented Z2 policy); the rest are
follow-ups. Write findings to ../athena-erp/.agents/tasks/beta-c1/
review-slice-2.md with a verdict line `Verdict: PASS` or `Verdict:
NEEDS_CHANGES`, blockers first with file:line. Then report: hc send master
"C1 SLICE 2 REVIEW: PASS or NEEDS_CHANGES <one line>". Do not edit product
code, commit, merge, push or start agents. Permission policy: stay inside
athena-erp and athena-erp-* folders, nothing destructive.
