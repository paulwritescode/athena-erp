# Review brief: B9 form sub-slice 2, layout builder

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b9form, branch
feat/b9-form-builder (commit 698f20f, on top of merged sub-slice 1).
Brief: ../athena-erp/.agents/tasks/beta-b9/form-brief.md (sub-slice 2
only). Follow .kiro/specs/athena-beta/design.md section 5 reviewer
workflow, docs/design/athena-design-system.md (§4, §7, §13, §14, §19 Form
tab), AGENTS.md and .kiro/steering/athena-design.md.

Review the sub-slice-2 diff (git diff 4dca9c0...HEAD on this branch —
sub-slice 1 is already in main, do not re-review it):

1. Builder in the editor Form tab: sections add/rename/reorder, columns 1|2,
   drag-and-drop field ordering with a keyboard alternative (move up/down,
   full keyboard operability), field settings in a right sheet (width, help,
   visibility CEL with inline type-check errors), static text/image blocks
   (image via picker over images/ files), optional art-header picker (keys
   as free text, placeholder preview — B10 registry must NOT be built here),
   live preview at phone (375) and desktop width. Move buttons instead of
   drag on phones; bottom drawers and 44 px targets on touch.
2. Saves are layout-only through the existing save path (no DDL, no
   confirm_data_loss) and show in History. Layout round-trips through the
   builder unchanged.
3. Scope: builder + editor Form tab + forms/ only; no records-list edits, no
   B2a-owned files; token/type/motion rules respected; one primary action
   per view.
4. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace at 375, 768, 1280, 1440 in both
   themes (build a 2-section layout by drag AND by keyboard-only, set
   widths/help/visibility incl. an invalid CEL, add text/image blocks, save,
   reload, confirm the View form matches). Never the dev database in
   .env.local. Note: only one Athena server runs per $HOME; the user server
   holds the real-HOME lock, so run any throwaway server with a fake HOME
   and never stop another agent's server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-b9/review-form-2.md with
a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: hc send master "B9 FORM 2 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push or
start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
