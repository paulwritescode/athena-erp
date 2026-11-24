# Review brief: C7 slice 5 reservations + tracking + counts (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c7-s5-r, detached at 5153252 (on 8b0efdf). Verify clean + diff is slice-5 scope only: new internal/stock/{reserve,tracking,count}.go + tests.
Read: .agents/tasks/beta-c7/brief.md slice 5 (11.5) + Z3 (weighted-average, FEFO, rentals tracked-not-consumed, no negative stock).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Reservations: locked takes, release/consume/expire all real (exercise each path); no double-take under concurrency (read the locking, run the concurrency test if present).
2. Tracking: serial lifecycle + rental on_rent with exact cancel reversals (verify a cancel mirror); batches + FEFO oldest-first NULLS-last (verify ordering with a test); value-preserving transfers incl. quarantine + serial moves (verify value conservation).
3. Counts: delta vs cutoff-adjusted expected, gains at average, zero no-op (verify with a test).
4. Rentals tracked-not-consumed: proven end-to-end with no GL, no layers, value 0 (verify the e2e test is real, not a stub).
5. Z3: weighted-average untouched, FEFO for perishables, negative stock refused (spot-check one refusal path).
6. Gates: affected green (reuse-or-rerun internal/stock through suite with GOFLAGS=-p=4 on 5153252); gofmt/build/vet clean; no proto change.
7. Scope: the 3 new files + tests ONLY (no engine, server, proto, gate, CLI, Desk changes — fail if any).

Report: say master "C7 SLICE 5 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c7/review-slice-5.md with the verdict line.
