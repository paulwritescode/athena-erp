# Summary: C5 slice 5 (Property 12 party ledger equals control)

Merged 2026-11-24 as 646cc6d (rebase of 0c4b59e onto post-C4s5 main, conflict-free), 1 commit, 2 files.

Scope: 7.5 — Property 12 rapid test in `internal/posting/settlement_test.go` (signed party residuals == control balance at every cutoff; base + FX-integral parts; per-iteration uuid isolation) + one rapid `.fail` corpus file. Tests only. The .fail file records a dev-time seed that failed on a test-helper bug (fixed since); ruled DEBRIS by the author, accepted by Stylus — merges as-is (replays green), deleted in slice 6 with a one-line note.

Evidence: worker affected green; reviewer green twice + affected reuse. Merge gate on 0c4b59e: exit 0, 24m (19 Go ok all fresh under suite -p 2, 684/684 web, zero FAIL, log 1124-183717). Rebase onto post-C2s5/C4s4 main conflict-free (posting tests vs secrets/ledger deltas), files identical, build+vet green.

Review: r-c5s5 PASS (review-slice-5.md) — genuine rapid equation both parts, green twice, tests-only.
