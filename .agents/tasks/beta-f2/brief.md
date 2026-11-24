# Brief: F2 type to filter (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-f2, branch feat/f2-filter from latest main. First: git status clean + git log main..HEAD empty.
Read: tasks.md F2 (beta tasks.md); B2a notes (API gaps: server lacks free-text search); the records list RPC + handler (find them); DataTable.vue search block; web/src/copy/glossary.ts; design-system tables/type sections.

Acceptance→slice mapping (every task-text line lands somewhere):
- "Search filters from the first character (~150 ms debounce" → slice 2 (Desk input + debounce; prove first-char with a test).
- "server-side match over label + text fields" → slice 1 (list RPC gains a match param; label + text fields; paged; injection-safe; no unbounded scans).
- "plain-words placeholder from the Record Type label" → slice 2 (placeholder derives from the type label, e.g. "Search diet concerns…").
- "Expression filtering moves behind Advanced filter using the F4 builder" → DEFERRED (F4 is mid-slice-1; leave the affordance point, do not build it; note in every report).
- "View words from glossary.ts" → slice 2 (all user-facing filter words via the glossary).

Slices (report each: say master "F2 SLICE n: PASS/BLOCKED <one line>"; rebase on main before each PASS):
1. Server free-text match: list param + handler + tests (match label + text, paging intact, empty query = today, hostile input safe). Proto change allowed (records.proto list params ONLY + make generate twice stable + gate policy already covers list).
2. Desk search: input + ~150ms debounce + derived placeholder + glossary words + tests + browser pass (type-to-filter live at 2 widths x 2 themes minimum, more if cheap).

Files: the list RPC/handler + tests, DataTable.vue search block (+ its test), glossary additions if needed. NOT allowed: RecordsPage.vue (6.10 owns it now), App.vue, any ledger/stock/events/proto file except the list params, F4 files (another lane's live branch — do not read or build on it).

Rules: batch conventions; spans without values; telemetry off unless set. Throwaway DBs + fake HOME. Full suites/browser only through .agents/bin/suite; package tests direct. Commit on feat/f2-filter; never merge/push/spawn.
Checks per slice: gofmt empty, go build/vet, affected or full suite, generate stable, web typecheck/test/build for slice 2.
