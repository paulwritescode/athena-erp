# Brief: C1 slice 7 — Z2 backdating window + book policy (Muse)

Lane L1, AFTER C5 slice 2 merges (master orders the start; do not start early). Worktree/branch: master assigns at dispatch (fresh tree off latest main).
Read: tasks.md C1; .agents/stylus/night-shift-orders.md line Z2 (the decided text, binding — quote it, do not redesign); docs/decisions/adr-0023-* (O-011/O-012, amended by the L5 decisions commit — read the amended text); internal/posting (the failed_precondition sentinels: find every one); batch-2 requirements O-011/O-012.

Task: replace the Z2 failed_precondition sentinels with the decided policy. Post only into open periods; no fixed day window. Each month soft-closes on the 10th of the next month (clerks can no longer post; holders of `ledger:close` post adjustments with a reason); hard-closes after the VAT return (due the 20th); hard-closed periods refuse everything. Reopening needs `ledger:close` + a reason, reopens later periods too, and is audited. Corrections after close go into the current period as reversals, never edits. One statutory book per company (KES); other books must be named; unnamed books stay refused.

Scope: internal/posting + its tests + the RPC/CLI error surfaces that expose the sentinels (keep procedure shapes; only the policy changes). Docs: decision-log row only. Do NOT amend ADR-0023 (L5 owns that amendment); do NOT touch RecordsPage.vue (F1 owns it now) or any other lane's files.

Rules: batch-2 conventions; real tests for every policy edge above (soft-close date math incl. the 10th boundary, hard-close refusal, reopen cascade + audit, current-period reversal landing, named/unnamed book refusal). Telemetry off unless set. Throwaway DBs + fake HOME. Full suite through .agents/bin/suite only; package tests direct. Commit on the assigned branch; never merge/push/spawn.
Checks: gofmt empty, go build/vet, affected or full suite, generate stable. Report: say master "C1 SLICE 7: PASS/BLOCKED <one line>" (rebase on main first).
