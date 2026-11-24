# Review brief: C7 slice 6 Property 23 availability (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c7-s6-r, detached at e5007d4 (on eb04345). Verify clean + slice-6 delta is stock_test.go property only (the tip also carries the slice-5 fix below it — review ONLY the Property 23 addition, which r-c7s5 does not cover).
Read: .agents/tasks/beta-c7/brief.md slice 6 (11.6).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Property REAL (Property 23: availability): genuine rapid property, not examples — drawn reservation sets, independent arithmetic model (available == on-hand minus active holds after every take/release/consume/expiry), 100 rapid iterations. Verify the model is independent of the implementation (recomputed, not calling the same code path).
2. Edges: over-holds refused, duplicates rejected, expiry statuses pinned, final DB invariant (holds never exceed on-hand) — each pinned by a real assertion, verify one by reading.
3. Worker carried the affected green across a byte-identical rebase (no re-gate): verify the carry claim (empty diff of slice files across the rebase) and run the property directly (100/100) yourself.
4. Gates: gofmt/build/vet clean; no proto change; affected reuse-or-rerun (internal/stock) through the main-repo suite if you doubt the carry.
5. Scope: stock_test.go property ONLY (no product-code changes in the slice-6 delta — fail if any).

Report: say master "C7 SLICE 6 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c7/review-slice-6.md with the verdict line.
