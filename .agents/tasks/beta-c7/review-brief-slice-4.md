# Review brief: C7 slice 4 Property 21 (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c7-s4-r, detached at f68cf1d (slice 4 on s3 44f5ed5; review ONLY the slice-4 delta 44f5ed5..f68cf1d — slices 2–3 are a separate train). Verify clean.
Read: .agents/tasks/beta-c7/brief.md slice 4 (11.4); batch-2 requirements Property 21 (append-only cost correction).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Property REAL (rapid, not example lists): drawn histories (receipts/issues/returns, backdated dates) then two delta batches; historical l_stock rows byte-identical after correction; bin + scoped GL move ONLY by on-hand deltas. Check distributions cover backdated + returns + fully-sold; runs meaningful.
2. Tests green (100/100 direct claimed — verify by running) + affected green (reuse-or-rerun stock scope through suite); gofmt/build/vet clean; no proto change.
3. Scope: stock_test.go (+ delta_test.go if the property lives there) ONLY — no product-code changes in slice 4 (fail if any); no other lane's files.

Report: say master "C7 SLICE 4 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c7/review-slice-4.md with the verdict line.
