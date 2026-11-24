# Summary: F1 records-table rhythm

Merged 2026-11-24 as b7fcb01/e3c381f/6ecf06d (cherry-picks of 507ffb5/6b29ed1/6f1b1e3 onto post-decisions main), 3 commits, 5 files.

Scope: footer always counts on selecting tables; actions column narrow + right-aligned; select/rownum padding explicit; explicit left cells; select-cell symmetry vs checkbox pr-0; label headers; shared alignment; hairlines; no filler column; 48px thumbnails. Files: DataTable.vue, dataTableModel.ts, style.css (.datatable-select-cell only) + recordColumns.test.ts, dataTableModel.test.ts. DISJOINT from C1 6.10 verified twice (zero shared files, zero rebase conflicts).

Evidence: typecheck + vite build green; 26/26 slice vitest (real asserts); browser 15/15 with DOM asserts + 8/8 shots inspected (375/768/1280/1440 x light/dark); final make test green pre-rebase; affected 66/636 post-rebase. Merge gate on 6f1b1e3: 424s, 17 Go ok (16 cached-from-identical-content, server re-ran fresh 411s DB-backed — webui/dist inputs changed), 636 web passed, zero FAIL. Live footer reads 0 selected (Records keyset total null, RecordsPage:55) with 0-of-5 shape unit-pinned.

Review: r-f1 PASS (review.md) — rhythm holds in code + shots, asserts real, DISJOINT held. Note: impeccable/emilkowalski-motion skills unavailable in this environment; design system applied as the authority (logged decided-without-skills).
