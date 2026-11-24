# Summary: C7 slice 4 (Property 21)

Merged 2026-11-24 as 2630dd3 (cherry-pick of f68cf1d onto post-C2s2 main), 1 commit, 1 file.

Scope: Property 21 (append-only cost correction) in `internal/stock/stock_test.go` (new): drawn histories (receipts/issues/returns, backdated dates) then two delta batches; every historical l_stock row byte-identical after correction; bin + scoped GL move only by on-hand deltas. Tests only, no product-code changes, no proto.

Evidence: 100/100 rapid iterations direct + affected green (stock scope, dedup) + merge gate on 0052c9d: 843s, 18 Go pkgs ok (server 793s, cmd/athena 594s, stock 560s DB-backed fresh; posting cached), 646 web passed, zero FAIL.

Review: r-c7s4 PASS (review-slice-4.md) — property real and green (100/100 verified), affected dedup on clean tip, scope one new test file. No follow-ups recorded.
