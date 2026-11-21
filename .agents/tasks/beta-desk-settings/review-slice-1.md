Quick slice review: Settings shell + Appearance, read-only, in /Users/iampaulking/projects/duchess/athena-erp-desk-settings (uncommitted). Merge-small rule: only BLOCKERs (broken behavior, data loss, security, unusable screen) stop the merge; everything else is FOLLOW-UP.
Check:
1. Appearance end to end on a throwaway Workspace: pick ink, red, custom; save; reload keeps it for another signed-in user; reset; a read-only user cannot save.
2. The master saw "Appearance saved." toast next to a contrast-failure suggestion (ds-appearance-suggested.png). Confirm the toast only shows when the server accepted the save; a toast saying saved after a refused save is a BLOCKER.
3. web/public/paint.js: runs before Vue, reads only localStorage, cannot inject CSS from untrusted input (values come from the server-validated OKLCH, but check the cache cannot be poisoned into arbitrary CSS through another path; if it can, is it only self-XSS). CSP still script-src 'self'.
4. Inert code (web/src/stepUp.ts unimported, interceptor step-up branch with no asker) does not change behavior now.
5. Run CI=true pnpm --dir web run typecheck, test, build.
Reply: hc send t6 "DESK SLICE 1 REVIEW: PASS or FAIL" with BLOCKERs and FOLLOW-UPs, file:line. Do not edit, commit or start agents.
