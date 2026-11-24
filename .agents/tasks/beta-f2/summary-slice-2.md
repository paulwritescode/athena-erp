# Summary: F2 slice 2 (desk type-to-filter)

Merged 2026-11-24 as 1601b3b (rebase of a4bde0e onto post-C2s3 main), 1 commit, 7 files.

Scope: type-to-filter live on Records — factored DataTableSearch (150ms debounced emit, immediate clear, unmount-safe) + dataTableSearch helpers + searchScope prop with Find-placeholder derivation + glossary find/clearSearch + RecordsPage query wiring (granted scope only: search→query, debounce removed there, status-only expression override kept for F4). Find verb per §21 (not task-text Search). F2 complete; Advanced filter via F4 stays deferred.

Evidence: worker affected web green on tip (70 files/660 tests) + browser PASS (seed 5 rows, first-char/type/clear asserts, 4 shots inspected, 0 page errors). Merge gate: first run RED on environment (npm registry flake → corrupt node_modules, Go green); retry GREEN 32s (Go cached, web 660 fresh, log 1124-123917). Rebase had one additive glossary conflict (F3s1 terms + F2s2 terms, union kept; other 6/6 files byte-identical); combined-tip web check green via suite (typecheck clean, 72 files/684 tests, log 1124-124209) + Go build/vet green.

Review: r-f2s2 PASS (review-slice-2.md) — debounce/glossary/grant/E2E/gates verified clean.
