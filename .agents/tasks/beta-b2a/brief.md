# Brief: B2a table system (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b2a, branch feat/b2a-tables. Scope: web/src (DataTable and its pages). Other workers are active in RecordTypesPage and the Record Type components (B9b) and in copy (B6): keep your edits in those files to the table itself and rebase often.
Read: docs/design/athena-design-system.md section 21 (this task, from the user's reference screenshots: a full-bordered table with padding, a select column, a row number column, a toolbar with search, filters and Columns, and a footer with "N of M selected" and Previous and Next), sections 7, 13 and 14; tasks.md B2a.
Slices, each merging on its own (report after each):
1. DataTable itself: frame, padding, select column, row numbers across pages, primary-column icon tile and second line, actions menu, toolbar (search, filter popovers with chips, Clear filters, Columns saved per user), selection bar with bulk actions, footer (range, rows per page saved per user, Page x of y, Previous and Next), and loading, empty, filtered-empty and error states. Show all of it in the Gallery. Records uses it with server keyset paging (RecordService already pages) and bulk delete with confirm. Keep the athena-entry-columns: width key.
2. Access (users, roles, sessions), Rules and Rule Runs, Files.
3. Ledger masters and the notifications full list. Record Types list last, after B9b merges.
Server: if a list needs a total count or a server-side search the API lacks, report it. Do not change Go.
Checks per slice: typecheck, test (exit 0), build. Browser pass on a throwaway Workspace with more than 60 Records (seed them) at 375, 768 and 1440 in light and dark: paging forward and back, selection across pages, bulk delete, filters, column chooser. Screenshots in .cache/screens/b2a-*.
Do not commit, merge, push or start agents. Report hc send t6 "B2A SLICE n: PASS or BLOCKED".
