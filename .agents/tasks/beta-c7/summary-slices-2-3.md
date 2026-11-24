# Summary: C7 slices 2–3 (stock core + fix + cost deltas)

Merged 2026-11-24 as 1f6ae9d/63ba358/3272765 (cherry-picks of dc41f13/32a76d0/44f5ed5 onto post-C2s1 main), 3-commit train, one tip gate.

Scope: slice 2 `internal/stock/` (Plan/Apply seam with G6 PostInTx, weighted average behind CostMethod, layers + applications, bins + rebuild, negative refusal for valued, exact cancel mirrors, quantity-only group claims) + fix (cancel-edge kinds + stored-kind asserts, issue-row FOR UPDATE + lock-proof concurrency test, quantity-only/receipt/return cancel tests, ordering contract + slice-11 naming; 3 nits done, 2 skips per review) + slice 3 `internal/stock/delta.go` (append-only cost deltas, on-hand/sold split by layer residual, balanced legs with credit flip, receipt-date economics, chunked watermarked RebuildBins + resume, PendingRebuild close predicate). Layers untouched by deltas.

Evidence: worker affected greens (stock 114–428s scopes) + 15 stock + 7 delta + 26 suite tests; rebase replays verified byte-identical. Train gate on fd20556: first run RED on contention (posting+server 20m panics under a rogue full-stock run outside the lock + cold cache; stock itself green 586s) → rogue killed, suite-only rule broadcast; retry-clean GREEN 703s (18 Go ok — posting 670s, server 643s after Stylus dropped 87 stale DBs + raised the pool to 512M), 646 web passed, zero FAIL.

Reviews: r-c7s2 NEEDS_CHANGES then PASS on the fix (4 findings verified + lock-proof test + nits disposition); r-c7s3 PASS delta-only on slice 3 (7 tests verified, follow-ups in review-slice-3.md).
