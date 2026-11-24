# Review brief: C5 slice 5 Property 12 party ledger equals control (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c5-s5-r, detached at 8468821 (on 43ded7c). Verify clean + diff is slice-5 scope only: Property 12 tests in settlement_test.go (+ helpers if any).
Read: .agents/tasks/beta-c5/brief.md slice 5 (7.5: signed party residuals equal the control balance at every cutoff).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Property REAL (Property 12): genuine rapid property, not examples — signed party residuals == control balance at every cutoff, base part + FX part (integral rates). Verify the control figure is independently derived (not the same code under test) and cutoffs vary.
2. Isolation: per-iteration uuid companies — verify no cross-test leakage (run the property twice, must pass both).
3. FX exactness: integral rates keep conversions exact — verify the claim (read the rate handling); worker notes fractional-rate rounding dust as follow-up (no booked-base column) — confirm genuinely out of scope, not a gap.
4. Gates: affected green (reuse-or-rerun posting + importers through the main-repo suite on 8468821); gofmt/build/vet clean; no proto change.
5. Z2: no reversals/windows/sentinels/books invented in the delta (grep revers*/sentinel/book-policy).
6. Scope: tests ONLY (no product-code changes — fail if any).

Report: say master "C5 SLICE 5 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c5/review-slice-5.md with the verdict line.
