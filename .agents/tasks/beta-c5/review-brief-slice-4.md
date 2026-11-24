# Review brief: C5 slice 4 realised FX + settlement reversal (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c5-s4-r, detached at 5c7b7bf (on 3cc6022). Verify clean + diff is slice-4 scope only: settlement.go FX + reversal paths + tests (+ docs if any).
Read: .agents/tasks/beta-c5/brief.md slice 4 (7.4) + Z2 RULE at the top + amended ADR-0023 O-011/O-012.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. FX: gain/loss posted as plans through the G6 engine (not direct writes); names/sides correct for both directions; payment==item kept, payment/base free per worker design.
2. Reversals MIRROR applications (criterion 6: same lines, flipped sides, reversal link) AND land in the CURRENT period per Z2 (never edits, never backdated) — verify with a test or a live run.
3. Closes respected: soft close (adjustments need `ledger:close` + reason), hard close refuses everything; settlement-level period gate correct. Worker's 4 deferred Z2 edges (ledger:close perm + audit, statutory-book naming, FX-into-soft, control-source check) must be genuine 7.6/engine scope, not slice-4 gaps — fail if any belongs here.
4. No sentinels: no failed_precondition/ErrCorrectionWindowOpen-style sentinel behavior where the Z2 policy now speaks; one KES book, named others, unnamed refused.
5. Gates: affected green (reuse-or-rerun posting + importers through suite on 5c7b7bf); gofmt/build/vet clean; generate stable (no proto expected).
6. Scope: settlement.go + tests ONLY (no engine, server, proto, gate, CLI, Desk changes — fail if any).

Report: say master "C5 SLICE 4 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c5/review-slice-4.md with the verdict line.
