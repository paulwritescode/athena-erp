# Review brief: C7 slice 7 stock-to-GL check (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c7-s7-r, detached at 471107e (on 9b1c070). Verify clean + slice-7 delta is check.go + tests ONLY (the tip carries unmerged slice-5/fix/slice-6 below — review ONLY the check.go addition).
Read: .agents/tasks/beta-c7/brief.md slice 7 (11.7: per-control-account stock-to-GL comparison + Integrity Log entries).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Comparison real: per-control-account posted-by-cutoff (GL vs l_stock + on-hand deltas) — verify the cutoff semantics against the engine (posted vs economic dates) and that on-hand deltas are the right adjustment.
2. Integrity rows: expected/observed/owner/detail written unless identical open rows exist; matches write nothing — verify each behavior with a test (match, both break directions, cutoff, skip-same, per-account).
3. Gates: affected green (reuse-or-rerun internal/stock through the main-repo suite on 471107e); gofmt/build/vet clean; no proto change.
4. Z3: no valuation/picking/rental/negative-stock behavior touched (read-only check — fail if it mutates).
5. Scope: check.go + tests ONLY (no other stock files changed in the slice-7 delta — fail if any).

Report: say master "C7 SLICE 7 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c7/review-slice-7.md with the verdict line.
