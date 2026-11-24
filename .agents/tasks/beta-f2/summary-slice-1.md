# Summary: F2 slice 1 (server free-text match)

Merged 2026-11-24 as 57e04c4 (cherry-pick of df2b80d onto post-C5s2 main), 1 commit, 7 files.

Scope: ListRecords gains a match param over label + text fields (hidden-safe, paged, hostile input escaped incl. LIKE wildcards/quotes/unicode; empty query = today's behavior exactly). Files: records.proto list param (additive) + generated Go/TS + lookup.go/records.go handler + records_test.go (+ 1 overview.md line). Existing list gate policy covers the param (no new entry). No Desk changes (slice 2 owns them); Advanced filter via F4 stays deferred.

Evidence: full make test green pre-rebase (17 Go pkgs ok, 636 web); rebase verified CODE-DISJOINT (only shared file overview.md 1 line, no test reads it) so the Stylus disjoint rule carried the PASS without a worker re-run. Merge gate on 22e0593: 1151s, 17 Go pkgs ok (posting 1059s, server 1023s, cmd/athena 676s DB-backed), 646 web passed, zero FAIL.

Review: r-f2s1 PASS (review-slice-1.md) — match correct, hidden-safe, paged, full suite green. No follow-ups recorded.
