# Review brief: C2 slice 3 ROUND 2 — review-fix verification (B1, F1–F5)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c2-r (detached; you start at
pre-rebase e2c2bf2 for READING ONLY — do not gate it). Brief:
../athena-erp/.agents/tasks/beta-c2/brief.md (slice 3 only). Round-1 verdict:
../athena-erp/.agents/tasks/beta-c2/review-slice-3.md (NEEDS_CHANGES: blocker
B1, follow-ups F1–F5). Follow .kiro/specs/athena-beta/design.md section 5
reviewer workflow and AGENTS.md. Design: batch-2 design.md G8; R8.4/R8.5;
ADR-0028.

PRE-START (now, while the worker rebases and gates): read the brief slice 3,
round-1 review-slice-3.md in full, and the fix diff
(git diff 3a9a3a1...HEAD in your tree: B1 restart-recovery fix, F1/F2/F4/F5
fix, F3 migration-19 occurrence_key widening). Reads only — no suite-lock
gates on this stale commit (gofmt/go build/go vet narrowly if useful).

ON MY ORDER (I will send the final rebased commit): move your tree to it
(git checkout --detach <commit>), verify clean, then review ONLY the fix
delta 3a9a3a1...<commit> plus regressions:

1. B1: restart recovery is row-first
   (attempts >= CASE WHEN max_attempts > 0 THEN max_attempts ELSE ? END),
   with the new test (MaxAttempts 10, attempts 7, expired lease => released).
   Verify the existing recover tests still pass.
2. F1: Runner.Errors removed or used. F2: DispatchDue copies the payload map
   before adding identity. F4: Recover release counts an attempt like reclaim
   (or a documented reason it does not). F5: claim NULL-next_run_at ordering
   comment.
3. F3: migration 19 widens athena_jobs.occurrence_key to 700 (reserved in
   .agents/stylus/migrations.md); dispatch of a ~600-char key works; the
   migration applies cleanly on a throwaway database.
4. Regression scan: the fix delta touches nothing outside the reviewed files
   except migration 19 + its test; slice-3 semantics from round 1 unchanged.
   Z4 still deferred (no secrets/custody/retention), Z2 untouched.
5. Gates: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   .agents/bin/affected by the ABSOLUTE main-repo path
   /Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected (same
   lock; reuse of the worker's green run on the same commit is fine — read
   that log), make generate twice with no diff. Tree clean before any gate.
   No browser pass (no Desk in this slice).

Merge-small rule: only blockers stop the merge (B1 not fixed, F3 migration
broken, red gates); the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c2/review-slice-3-round2.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: say master "C2 SLICE 3 ROUND 2 REVIEW: PASS or
NEEDS_CHANGES <one line>". Then wait for instructions. Do not edit product
code, commit, merge, push or start agents. Send UPDATE every 10 minutes.
