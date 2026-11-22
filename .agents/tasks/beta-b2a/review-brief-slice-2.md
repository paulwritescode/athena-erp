# Review brief: B2a slice 2, Access + Rules + Files on DataTable

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b2a, branch feat/b2a-tables
(commit ed600ca, on top of merged slice 1). Brief: ../athena-erp/.agents/
tasks/beta-b2a/brief.md (slice 2 only). Follow .kiro/specs/athena-beta/
design.md section 5 reviewer workflow, docs/design/athena-design-system.md
(section 21 tables; also §7, §13, §14), AGENTS.md and
.kiro/steering/athena-design.md.

Review the slice-2 diff (git diff e23de85...HEAD on this branch — slice 1
is already in main, do not re-review it):

1. Access users, roles and sessions; Rules list (Type/Status filters) and
   Rule Runs table (Outcome/Event filters); Files list (Folder filter) plus
   the mapping table — all on the shared DataTable with the §21 frame,
   toolbar, selection, footer and states. Row menus reuse each sheet's
   actions with explicit targets; versions/history tables, permissions
   matrix and simulate table intentionally stay plain Tables.
2. Confirm the six slice-1 follow-ups are fixed: search input padding +
   480 px cap, dead entries/files-tree CSS removed, glossary status words,
   no double list request on filter clear, pressTimer cleanup.
3. Scope: table conversions only; no behavior changes beyond the listed
   follow-ups; token guard green.
4. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace at 375, 768 and 1440 in light and
   dark (each converted table: paging, selection, filters, row actions,
   empty/loading/error states). Never the dev database in .env.local. Note:
   only one Athena server runs per $HOME; the user server holds the
   real-HOME lock, so run any throwaway server with a fake HOME and never
   stop another agent's server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups
(record them; the worker also reported API gaps — runs 50-cap/total, no
bulk APIs — confirm and carry them as follow-ups, not blockers). Write
findings to ../athena-erp/.agents/tasks/beta-b2a/review-slice-2.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: hc send master "B2A SLICE 2 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push or
start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
