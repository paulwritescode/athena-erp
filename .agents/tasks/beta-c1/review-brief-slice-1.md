# Review brief: C1 slice 1, posting validation + writer (6.2+6.3)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1, branch feat/c1-posting
(commits 9d87fbf + a9e1d49). Brief: ../athena-erp/.agents/tasks/beta-c1/
brief.md (slice 1 only: batch-2 G6.2+G6.3). Follow .kiro/specs/athena-beta/
design.md section 5 reviewer workflow and AGENTS.md. Design reference:
.kiro/specs/athena-batch-2/design.md G6; requirements R6 (batch-2
requirements.md); ADR-0023 (OPEN backdating/book policy — see Z2 rule).

Review the branch diff against main (git diff main...HEAD — new
internal/posting/ package only):

1. Plan validation: balance by company and book, leaf accounts, periods,
   dimensions, control accounts. Single-transaction writer + balance
   deltas. Reuses internal/types money/allocate and internal/audit.
2. Property test for balanced postings (Property 8) in posting_test.go —
   real rapid-style property coverage, not a table test in disguise.
3. Z2 RULE (binding): backdating window + book policy are UNDECIDED. The
   worker reports empty-book and hard-closed-period cases return
   failed_precondition sentinels pinned by tests. Verify: no invented
   window/policy anywhere (grep for day counts, date arithmetic, book
   names as policy); every Z2 point fails plainly and is test-pinned.
4. No new migration/ADR (none expected — migration-14 tables reused);
   confirm no schema changes and no edits outside internal/posting/.
5. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test with MariaDB enabled (throwaway test_athena_* databases
   only, never the dev database in .env.local) — incl. the new property
   tests unskipped, and make generate twice with no diff. No browser pass
   (no Desk in this slice).

Merge-small rule: only blockers stop the merge (wrong balances, lost
writes, broken atomicity, invented Z2 policy); the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-c1/review-slice-1.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "C1 SLICE 1 REVIEW: PASS
or NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
