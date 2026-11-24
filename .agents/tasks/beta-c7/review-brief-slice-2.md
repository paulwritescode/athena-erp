# Review brief: C7 slice 2 stock core (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c7-s2-r, detached at a69d8b3 (slice 2 on rebased slice-1 b3e9ca4; review ONLY the slice-2 delta b3e9ca4..a69d8b3 — slice 1 is reviewed and merging separately). Verify clean.
Read: .agents/tasks/beta-c7/brief.md slice 2 (11.2); batch-2 design G11 + requirements; ADR-0032 (Z3: weighted average, negative refusal, cancel mirrors); internal/posting seam (read-only).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Seam: Plan/Apply with G6 PostInTx correct (one plan→apply contract, tx boundaries honest, quantity-only group claims sound).
2. Costing: weighted average behind CostMethod; layers + applications; bins + rebuild; negative refusal for valued only (quantity-only items exempt per Z3?); exact cancel mirrors.
3. Tests: 15 stock tests REAL (GL-balance match, replay, cancel-block are true asserts, not shapes); affected green (reuse-or-rerun: cmd/athena + server + stock through suite); gofmt/build/vet clean; no proto change (generate untouched — verify).
4. Scope: internal/stock/** + tests ONLY (slice 2 must not modify posting/quantity/UoM — fail if it does); lifecycle wiring is slice 11, not this slice — note but do not demand.
5. Worker evidence: affected 251s/428s/114s zero FAILs — spot-verify in suite.jsonl; rebase b0510f1→a6dec5c conflict-free with no content change (verify diff of the pick).

Report: say master "C7 SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c7/review-slice-2.md with the verdict line.
