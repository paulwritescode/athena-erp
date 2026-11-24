# Review brief: F1 records-table rhythm (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f1-r, detached at 6f1b1e3 (3 commits on a6dec5c: 507ffb5 rhythm, 6b29ed1 left cells, 6f1b1e3 select symmetry). Verify clean + diff touches ONLY DataTable.vue, dataTableModel.ts, style.css (.datatable-select-cell), recordColumns.test.ts, dataTableModel.test.ts. DISJOINT is load-bearing: fail the review if RecordsPage.vue, RecordsPage.test.ts, PageSwitch.vue or pageSwitchBuild.test.ts changed.
Read: .agents/tasks/beta-f1/brief.md; docs/design/athena-design-system.md (tables); the worker PASS (night log).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Rhythm: footer always counts on selecting tables; actions column narrow + right-aligned; select/rownum padding explicit; label headers; shared alignment incl. explicit left cells; select-cell symmetry vs checkbox pr-0; hairlines; no filler column; 48px thumbnails; .datatable-select-cell only in style.css.
2. Shots: eyeball /tmp/f1-pass/shots/table-{light,dark}-{375,768,1280,1440}.png (8) — rhythm holds at every width/theme, no breakage.
3. Tests: 26/26 slice vitest are REAL asserts (not shape-only); live-footer-0 + 0-of-5-shape claims verified in code (RecordsPage:55 keyset null); typecheck + vite build green; affected green (reuse-or-rerun through suite); generate stable.
4. Notes check: skills-unavailable/design-system-applied claim sane; dist gitkeep restored; tree clean.

Report: say master "F1 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f1/review.md with the verdict line.
