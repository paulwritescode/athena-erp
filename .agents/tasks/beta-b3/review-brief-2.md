Re-review B3 and B3a, loop 2, read-only, in /Users/iampaulking/projects/duchess/athena-erp-b3-experience (uncommitted).
Previous review (Codex) and the fixes asked: .agents/tasks/beta-b3/review-loop-1.md. Read it first, including the three master decisions at the top.
1. Reproduce each of the 8 blockers and confirm it is fixed: nested drawer taps at 375 and 768; hard close and end session ask first; migration backfill kept after Esc; optional select can be cleared, required cannot; stale responses cannot overwrite another selection (Access, Rules, Files, Ledger); nothing overflows at 320; File, Rule Run and Ledger master sheets; toasts never off screen or over actions at 768, 1023, 1280 with the 640 px sheet.
2. Confirm the 5 follow-ups.
3. Look for regressions from this loop in Records, Record Types, Access, Ledger, Rules, Files, sign-in.
4. Run CI=true pnpm --dir web run typecheck, test, build (temp outDir); sample .cache/screens/b3l1-*.
Use a throwaway Workspace only (athena new in a temp home), never the dev database in .env.local, if you need a live server.
BLOCKER or FOLLOW-UP; PASS if no BLOCKER. Do not edit, commit or start agents. Reply: hc send t6 "B3 RE-REVIEW: PASS or FAIL" with file:line.
