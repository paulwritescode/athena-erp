# Review brief: B2a slice 1, DataTable + Records + Gallery

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b2a, branch feat/b2a-tables.
Brief: ../athena-erp/.agents/tasks/beta-b2a/brief.md (slice 1 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow,
docs/design/athena-design-system.md (section 21 is this task; also 7, 13,
14), AGENTS.md and .kiro/steering/athena-design.md.

Review the branch diff against main (git diff main...HEAD):

1. DataTable per design-system §21: bordered full-width frame with proper
   padding; select column (page checkbox + indeterminate, only when bulk
   actions exist); row numbers continuing across pages; primary column with
   icon tile + second line; actions menu; toolbar (search, filter popovers
   with chips, Clear filters, Columns saved per user); selection bar with
   bulk actions; footer (range text, rows-per-page saved per user, Page x
   of y, Previous/Next with disabled reasons); loading, empty,
   filtered-empty and error states inside the frame; server keyset mode
   ("Page N" without total); resizable columns keeping the
   athena-entry-columns: width key; stacked phone rows with #N prefix and
   long-press select.
2. Records uses it with server keyset paging (Previous works via the token
   stack), debounced search, Status filter where valid, bulk delete with
   confirm, row Open/Delete menu with disabled reasons; editor untouched.
3. Gallery shows every feature in every state (client + server-paging demos).
4. No edits outside the table itself in files other workers own
   (RecordTypesPage, record-type components); tablePrefs/dataTableModel
   helpers tested; token guard green (no hex/rgb/px outside tokens).
5. Run: CI=true pnpm --dir web run typecheck, test (exit 0), build; plus a
   browser pass on a throwaway Workspace with 60+ Records at 375, 768 and
   1440 in light and dark (paging both ways, cross-page selection, bulk
   delete, filters, column chooser). Never the dev database in .env.local.
   Note: only one Athena server runs per $HOME (install lock); other agents
   hold the real-HOME lock, so run any throwaway server with a fake HOME
   (e.g. HOME=/tmp/athena-b2a-fakehome) and never stop another agent's server.

Merge-small rule: only blockers stop the merge (breaks behavior, loses
data, opens a security hole, makes a screen unusable); everything else is a
follow-up recorded in the review. Write findings to
../athena-erp/.agents/tasks/beta-b2a/review-slice-1.md with a verdict line
`Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with file:line.
Then report: hc send master "B2A SLICE 1 REVIEW: PASS or NEEDS_CHANGES
<one line>". Do not edit product code, commit, merge, push or start agents.
Permission policy: stay inside athena-erp and athena-erp-* folders, nothing
destructive.
