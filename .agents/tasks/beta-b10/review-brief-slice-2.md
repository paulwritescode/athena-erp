# Review brief: B10 slice 2, empty-state figures

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b10, branch feat/b10-art
(commit 7a0a7de, on top of merged slice 1). Brief: ../athena-erp/.agents/
tasks/beta-b10/brief.md (slice 2 only). Follow .kiro/specs/athena-beta/
design.md section 5 reviewer workflow, docs/design/athena-design-system.md
(section 9 is this task; also §7, §13, §14), AGENTS.md and
.kiro/steering/athena-design.md.

Review the slice-2 diff (git diff 9fdad29...HEAD on this branch — slice 1
is already in main, do not re-review it):

1. Empty-state figures per the §9 meaning table (e.g. Sieve for no
   results, Drawer for empty Files, Stack for empty Records) through the
   registry only — EmptyState + DataTable empty slots, at most one figure
   per screen, 160–240 px (120 mobile), line color `--app-text-subtle`,
   theme auto, intensity ≤0.3, play off, aria-hidden or labelled, reduced
   motion rested.
2. Records list carries NO art (B9 follow-up 2.3 owns list-image work) —
   confirm no Records empty-state figure was added.
3. Registry extensions (new placement keys) keep the slice-1 invariants:
   unknown keys return nothing, lazy loading intact (spot-check the chunk
   did not balloon unreasonably past the known 120 KB issue — do not block
   on that pre-existing follow-up).
4. Scope: empty states only; no behavior, copy (beyond art labels), layout
   or token changes; token guard green.
5. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace at 375 and 1440 in both themes
   (several empty states + one filtered-empty: figure, size, tone, motion).
   Never the dev database in .env.local. Note: only one Athena server runs
   per $HOME; the user server holds the real-HOME lock, so run any
   throwaway server with a fake HOME and never stop another agent's server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
This is the LAST B10 slice: also confirm the B10 task text is fully
covered. Write findings to ../athena-erp/.agents/tasks/beta-b10/
review-slice-2.md with a verdict line `Verdict: PASS` or `Verdict:
NEEDS_CHANGES`, blockers first with file:line. Then report: hc send master
"B10 SLICE 2 REVIEW: PASS or NEEDS_CHANGES <one line>". Do not edit product
code, commit, merge, push or start agents. Permission policy: stay inside
athena-erp and athena-erp-* folders, nothing destructive.
