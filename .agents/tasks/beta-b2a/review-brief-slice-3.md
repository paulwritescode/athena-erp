# Review brief: B2a slice 3, Ledger masters + Record Types on DataTable

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b2a, branch feat/b2a-tables
(commit fcb4c66, on top of merged slices 1–2). Brief: ../athena-erp/.agents/
tasks/beta-b2a/brief.md (slice 3 only; notifications SKIPPED per Stylus —
no notifications page exists and this slice must not invent one). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow,
docs/design/athena-design-system.md (section 21 tables; also §7, §13, §14),
AGENTS.md and .kiro/steering/athena-design.md.

Review the slice-3 diff (git diff 0431689...HEAD on this branch — slices
1–2 are already in main, do not re-review them):

1. Ledger masters on DataTable (companies, branches, departments, cost
   centres, dimensions, fiscal years, periods with Status filter,
   currencies, account types, accounts, posting rules) with search, sorting,
   client paging, per-user columns; New actions live in the table toolbars;
   the readiness checklist intentionally stays a plain Table.
2. Record Types list on DataTable: search, multi-select Kind/Pack filters
   in the toolbar, View-records row menu, empty state with create action.
3. DataTable gains optional filter hints (phone drawer); PrimaryCell gains
   a mono title for ledger codes. No other shared-component behavior
   changes; token guard green.
4. Confirm the worker-reported gaps (follow-ups, NOT blockers): workspace
   CLI lacks parseTime so period reads fail; no ledger bulk/delete APIs
   (tables correctly show no select/bulk where the API lacks them).
5. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace at 375, 768 and 1440 in light and
   dark (each converted table: paging, sorting, selection, filters incl.
   multi-select Kind/Pack, row actions incl. View-records, empty/loading/
   error states). Never the dev database in .env.local. Note: only one
   Athena server runs per $HOME; the user server holds the real-HOME lock,
   so run any throwaway server with a fake HOME and never stop another
   agent's server.

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
This is the LAST B2a slice: also confirm every table the B2a task lists is
now converted except the explicitly skipped notifications page. Write
findings to ../athena-erp/.agents/tasks/beta-b2a/review-slice-3.md with a
verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: hc send master "B2A SLICE 3 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push or
start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
