# Summary: C5 slice 2 (settlement applications)

Merged 2026-11-24 as 929d3dd (cherry-pick of 9b91f8d onto post-F6 main), 1 commit, 2 files.

Scope: `internal/posting/settlement.go` (new): open items per instalment, line-level applications (both endpoints, both currencies, base amount), locks on both residuals, aggregate ceiling under the lock, unapplied cash kept separate; allocation via `internal/types/allocate.go` (no reimplemented rounding). No Z2 touch (reversals/windows/books are slice 4, now under the decided Z2 rule).

Evidence: affected green (cmd/athena, posting, server); concurrency + ceiling + separation tests; gofmt/build/vet clean. Merge gate on 8298269: 1124s, 17 Go pkgs ok (posting 1047s, server 1008s, cmd/athena 608s DB-backed), 646 web passed, zero FAIL.

Review: r-c5s2 PASS (review-slice-2.md) — locks + ceiling + unapplied-cash correct, allocator used right, tests real, Z2 untouched. No follow-ups recorded.
