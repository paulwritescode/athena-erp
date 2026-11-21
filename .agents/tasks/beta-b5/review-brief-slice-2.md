# Review brief: B5 slice 2, Workspace identity Desk

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b5, branch feat/b5-identity
(commit 2e3f5b7, on top of merged slice 1). Brief: ../athena-erp/.agents/
tasks/beta-b5/brief.md (slice 2 only). Follow .kiro/specs/athena-beta/
design.md section 5 reviewer workflow, docs/design/athena-design-system.md
(section 10 brand + logo, plus §7, §13, §14), AGENTS.md and
.kiro/steering/athena-design.md.

Review the slice-2 diff (git diff 4698f96...HEAD on this branch):

1. Header, sign-in, tab title and favicon use the Workspace name and logo
   (light/dark); the Athena mark is the fallback (black on light, white on
   dark); "Powered by Athena" appears in the user menu footer once a logo is
   set. §10 sizes and clear space (header 24 px, sign-in 48 px, never
   stretched, name beside/below).
2. Settings → Workspace identity section: name, light and dark logo upload
   with preview, remove; validation errors (type/size) inline; works with no
   logo set (mark fallback everywhere).
3. Root logo copies (`athena-black-logo`, `athena-white-logo`) handled per
   brief (renamed under web/src/assets/brand or removed if B2's copies are
   reused — no duplication, no stray root files left by this slice).
4. No accent/token violations (token guard green), favicon + theme-color
   correct in both themes, manifest name consistent.
5. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace at 375, 768, 1280, 1440 in both
   themes (set name + logos, reload, check header/sign-in/tab/favicon,
   remove logos, check fallback). Never the dev database in .env.local.
   Note: only one Athena server runs per $HOME; the user server holds the
   real-HOME lock, so run any throwaway server with a fake HOME and never
   stop another agent's server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-b5/review-slice-2.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "B5 SLICE 2 REVIEW: PASS
or NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
