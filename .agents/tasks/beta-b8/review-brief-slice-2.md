# Review brief: B8 slice 2, root presentation + Build/View

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b8, branch
feat/b8-root-buildview (commit 3489f84, on top of merged slice 1). Brief:
../athena-erp/.agents/tasks/beta-b8/brief.md (slice 2 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow,
docs/design/athena-design-system.md (§7 mode switch, §14 actions, §15
sidebar, §16 detail), AGENTS.md and .kiro/steering/athena-design.md.

Review the slice-2 diff (git diff 0afe93c...HEAD on this branch — slice 1
is already in main, do not re-review it):

1. Root Desk presentation: quiet "Root administrator" caption, `lock` badge
   on root's own user row with the reason, root row actions disabled with
   the reason (consistent with the slice-1 server refusals).
2. The `?mode=build` fresh-load bug is fixed (no longer lands in View;
   regression tests pin it — the worker notes B2 already fixed the
   underlying watch, so verify the tests pin the behavior, not dead code).
3. Mode switch per §7 (segmented control, words + icons, shown only to
   builders); Build shows a persistent "Building" caption + hairline-
   separated sidebar group; View hides ALL Build navigation (no Build
   routes reachable, no Build items in nav/menus/palette).
4. Read-only "Preview as" control in Build renders View as a chosen Role
   (read-only: no writes possible through the preview; verify a write
   attempt is refused or impossible, not just hidden).
5. Scope: presentation + mode only; no permission, gate or auth-logic
   changes; token/type/motion rules respected.
6. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace at 375, 768, 1280, 1440 in both
   themes (root sign-in caption/badge/disabled actions; fresh ?mode=build
   load; mode switch both ways; View has zero Build nav; Preview-as each
   of 2+ roles incl. a write attempt). Never the dev database in .env.local.
   Note: only one Athena server runs per $HOME; the user server holds the
   real-HOME lock, so run any throwaway server with a fake HOME and never
   stop another agent's server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
This is the LAST B8 slice: also confirm the B8 task text is fully covered.
Write findings to ../athena-erp/.agents/tasks/beta-b8/review-slice-2.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "B8 SLICE 2 REVIEW: PASS
or NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
