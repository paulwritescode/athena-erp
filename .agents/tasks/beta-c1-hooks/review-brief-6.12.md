# Review brief: C1 6.12 — posting checkpoint (verification-only, no commit)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c1-r (detached at 919bab4 =
main; verify clean on arrival — the worker reports no commit, tree clean).
Task brief: ../athena-erp/.agents/tasks/beta-c1-hooks/brief.md (slice
6.11+6.12 checkpoint part). Follow .kiro/specs/athena-beta/design.md
section 5 reviewer workflow and AGENTS.md.

This slice has NO diff: your job is to verify the worker's checkpoint
evidence that posting works on current main:

1. Worker claims (from its PASS report): generate-x2 stable, make test
   green via the train merge-gate log on 919bab4 (0 FAIL, web 613), web
   typecheck + build green, browser pass 22/22 (light submit + double-click
   yields one group + refused re-submit; dark cancel reverses), 6 shots in
   /tmp/c12-pass/shots all eyeballed, CLI preview with plan+hash, no Z2
   skip, Desk preview dialog correctly NOT built (6.10 has no GO).
2. Verify: branch feat/c1-hooks is empty on 919bab4 (git log main..HEAD
   empty in ../athena-erp-c1, tree clean); the merge-gate reuse is valid
   (suite log for the train tip gate on 919bab4, exit 0); the 6.12 browser
   suite log (label "C1 6.12 browser pass retry") shows exit 0 with 22/22
   steps; exactly the claimed shots exist in /tmp/c12-pass/shots.
3. Eyeball the shots yourself (image-reading tool): do they show the
   claimed states (draft, submit dialog, submitted/duplicate-refused,
   cancel dialog, cancelled) in the claimed themes? Any shot contradicting
   the report is a blocker.
4. Spot-verify one CLI preview (run the documented command on a throwaway
   Workspace; plan + hash shape sane). Do NOT run a full suite (nothing
   changed; reuse is the evidence) — narrow reads only, no suite-lock
   gates.
5. Confirm: no code/docs changed (nothing to merge); Z2 untouched; 6.10
   untouched.

Merge-small rule: only blockers stop the sign-off (false green claim, shot
contradicting the report, reuse invalid, unreported skip); the rest are
follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c1-hooks/review-6.12.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first. Then
report: say master "C1 6.12 REVIEW: PASS or NEEDS_CHANGES <one line>".
Then wait for instructions. Do not edit product code, commit, merge, push
or start agents. Send UPDATE every 10 minutes.
