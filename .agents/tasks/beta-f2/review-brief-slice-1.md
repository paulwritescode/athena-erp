# Review brief: F2 slice 1 server free-text match (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f2-s1-r, detached at df2b80d (on 6c871c9). Verify clean + diff is slice-1 scope only: records.proto list match param + generated (proto + TS) + lookup.go/records.go handler + records_test.go (+ overview.md line only).
Read: .agents/tasks/beta-f2/brief.md slice 1; B2a API-gap notes.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Match semantics: label + text fields, hidden-safe, paged (no unbounded scans), hostile input escaped (LIKE wildcards/quotes/unicode), empty query = today's behavior exactly.
2. Proto: list param additive (no breaking change); generated files match sources (generate stable); existing list policy covers the param (no new gate entry needed — verify).
3. Tests REAL: escaping pinned, paging pinned, empty-query equivalence pinned; worker full-suite green on e8efbbd + CODE-DISJOINT rebase to df2b80d (shared file only overview.md, no test reads it — verify with git); affected reuse-or-rerun through suite; gofmt/build/vet clean.
4. Scope: no RecordsPage.vue/DataTable.vue changes in slice 1 (slice 2 owns the Desk); F4 item stays deferred (fail if slice 1 builds it).

Report: say master "F2 SLICE 1 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f2/review-slice-1.md with the verdict line.
