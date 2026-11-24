# Review brief: C7 slice 3 cost deltas (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c7-s3-r, detached at 110d9d8 (slice 3 on fix tip 325cca0; review ONLY the slice-3 delta 325cca0..110d9d8 — slice 2 + fix are a separate review). Verify clean.
Read: .agents/tasks/beta-c7/brief.md slice 3 (11.3); batch-2 design G11 deltas; ADR-0032 (cost-correction semantics).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Deltas: Plan/ApplyCostDelta on-hand/sold split by layer residual correct; legs balanced (stock/cogs/offset, sides flip on credit); appended rows carry receipt economic date + allowed posting date; layers UNTOUCHED (no re-blend — fail if delta code mutates layers); delta cancel mirrors.
2. Rebuild: RebuildBins chunked with COMMITTED watermarks + resume correct (crash-safe: no double-apply, no skip); PendingRebuild close predicate sane (close-path wiring is slice 11's, not this slice — note, do not demand).
3. Tests REAL (7 delta tests: splits, dates, fully-sold, credit flip, delta cancel, watermark resume, pending) — true asserts incl. a resume-from-midpoint case; affected green (reuse-or-rerun stock scope through suite); gofmt/build/vet clean; no proto change.
4. Scope: internal/stock/delta.go + its tests (+ stock_test appends?) ONLY; no other lane's files.

Report: say master "C7 SLICE 3 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c7/review-slice-3.md with the verdict line.
