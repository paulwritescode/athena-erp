# Review brief: C5 slice 3 Property 11 (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c5-s3-r, detached at 6a879f4 (slice 3 on slice-2 9b91f8d; review ONLY the slice-3 delta 9b91f8d..6a879f4 — slice 2 is reviewed and merging separately). Verify clean.
Read: .agents/tasks/beta-c5/brief.md slice 3 (7.3); batch-2 requirements Property 11 (allocation sums); internal/types/allocate.go.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Property REAL (rapid, not example lists): exact sums, determinism, tie-break in types_test.go; settlement-level sums + id-order model with shuffled input in settlement_test.go. Check seeds/runs are meaningful, shrinks sane.
2. APPEND-ONLY: diff of internal/types/types_test.go is a PURE APPEND at end of file (no edits above, no import changes) — fail otherwise (L3 shares this file).
3. Tests green + affected green (reuse-or-rerun types + posting + importers through suite); gofmt/build/vet clean.
4. Z2: slice 3 must NOT touch reversals, windows, books or sentinels — fail if it does (slice 4 owns Z2).
5. Scope: the two test files ONLY (no product-code changes in slice 3 — fail if any).

Report: say master "C5 SLICE 3 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c5/review-slice-3.md with the verdict line.
