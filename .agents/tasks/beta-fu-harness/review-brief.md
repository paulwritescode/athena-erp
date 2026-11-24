# Review brief: FU-H shared browser harness (tool review)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-fu-h-r (detached at
the commit in my ORDER; verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-fu-harness/brief.md. Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow
(adapted: this reviews a developer TOOL, not product UI) and
AGENTS.md.

Review the FU-H diff (git diff main...HEAD — expect ONLY
.agents/bin/browser + example + README/docs):

1. Contract compliance, line by line against the brief:
   free-port probe-and-skip (never fixed colliding ports); own
   test_athena_* DB + fake HOME (never dev anything, never
   another lane's); first-user creation + sign-in; stable-
   selector waits (bounded, with diagnostics — never bare
   sleeps); theme set + VERIFY applied; route visits; shots
   to the given folder; console/page error collection.
2. Sign-in redesign (worker's disclosed call): owner lacks
   build + denied reads 403, so root via CLI reset challenge
   on the real form. Verify the evidence (exit-1 owner log
   quoted in the report) supports the call; judge whether
   root-based harness auth is acceptable for a test tool
   (consider: throwaway DBs only — confirm no path to prod
   or dev data). This echoes the B11 Instance-Owner question
   — note it as a follow-up, do not expand scope.
3. Exit codes: 0 pass / 1 app failure / 3 harness failure —
   verify each path returns the right code (read the code;
   the worker's failure drills claimed both — check the
   drill logs exist and show the right exits).
4. Always-exit + no orphans: try/finally coverage (Ctrl+C,
   timeouts, assertion throws, CDP drops); verify no Chrome/
   node processes from the worker's runs remain (`ps`
   check is fair game); the exec-detach + trap fix from the
   stuck-slot incident must be present.
5. Reproduce: run the example script yourself through
   `.agents/bin/suite` (same lock) on a free port pair of
   your choosing + own test DB — it must go green WITHOUT
   modifying it. One run suffices (the worker proved two).
6. Docs: README section accurate (args, exit codes,
   port/DB rules, example); a new lane could adopt it
   without asking questions.
7. Confirm: tool files ONLY (no product, proto, migration,
   gate, web, server changes).

Merge-small rule: only blockers stop the merge (contract
gap, unsafe data path (dev/prod reachable), wrong exit
codes, orphans left, example not green on your run); the
rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-fu-harness/review.md with
a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master
"FU-H REVIEW: PASS or NEEDS_CHANGES <one line>". Then wait
for instructions. Do not edit product code, commit, merge,
push or start agents. Send UPDATE every 10 minutes.
