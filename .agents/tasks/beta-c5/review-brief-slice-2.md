# Review brief: C5 slice 2 settlement applications (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c5-s2-r, detached at 9b91f8d (on current main 2954bce). Verify clean + diff is slice-2 scope only: `internal/posting/settlement.go` (new) + its tests (+ nothing else except docs-test fixtures if any).
Read: .agents/tasks/beta-c5/brief.md slice 2 (7.2); batch-2 requirements criterion 7.x (applications); internal/types/allocate.go (the used allocator); ADR-0031.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Correctness: open items per instalment; line-level applications (both endpoints, both currencies, base amount); LOCKS on both residuals; aggregate ceiling enforced UNDER the lock (no TOCTOU); unapplied cash separate from applications.
2. allocate.go used correctly (no reimplemented rounding/splitting); amounts exact (no floats).
3. Tests REAL: concurrency test genuinely races (not sequential); ceiling test pins the bound; unapplied-cash separation pinned; affected green (reuse-or-rerun cmd/athena + posting + server through suite); gofmt/build/vet clean.
4. Z2: slice 2 must NOT touch reversals, windows, books or sentinels — fail if it does (Z2 decided, but slice 4 owns it).
5. Scope: no other lane's files (events/ledger/record_types protos, App.vue, L3-owned UI).

Report: say master "C5 SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c5/review-slice-2.md with the verdict line.
