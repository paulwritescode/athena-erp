# Summary: B2a slice 1, DataTable + Records + Gallery (merged afc70a6)

## What shipped

One DataTable for the Desk (§21): bordered frame, select column with
indeterminate page checkbox, row numbers across pages, primary column with
icon tile + second line, toolbar (search, filter popovers with chips, Clear
filters, per-user Columns), selection bar with bulk actions, footer (range,
per-user page size, Page x of y, Previous/Next with disabled reasons),
loading/empty/filtered-empty/error states in-frame, server keyset mode,
resizable columns (existing width keys kept), stacked phone rows with long-
press select and a Filters drawer. Records uses it with server keyset paging
(token stack for Previous), debounced CEL search, Status filter where valid,
bulk delete with confirm, and row Open/Delete with disabled reasons. The
Gallery demos everything in every state (client + keyset-server).

## History

- Worker w-b2a committed 7699b5a (16 files; RecordsTable.vue deleted).
- Review round 1 PASS: §21 checklist met, typecheck/test (440)/build green,
  browser pass on 65 customers + 6 invoices at 1440/768/375 in dark+light
  (paging, cross-page selection, bulk delete, search, filters, Columns,
  persistence, row menu, editor create), 6 non-blocking follow-ups.
- Rebased cleanly onto main f7f4b5d (incl. B4b+B9b); master reran all checks
  green; merged --ff-only, pushed, recorded slice line (task stays open for
  slices 2–3).

## Follow-ups (in tasks.md)

Search-icon text overlap, dead entries CSS, hardcoded status words, double
list request on filter clear, pressTimer leak, 448 vs 480 px search width;
server API gaps: totals, free-text search, sort params, bulk-delete RPC.

## Try it

View → any Record Type with Records: search, filter, select across pages,
bulk delete; Build → Gallery for the full showcase.
