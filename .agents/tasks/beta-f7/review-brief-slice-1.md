# Review brief: F7 slice 1 bare-athena menu (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f7-s1-r, detached at 4be81da (on 3e1bc9d). Verify clean + diff is slice-1 scope only: command registry + menu.go + menu_test.go + F6-banner header call (+ docs if any).
Read: .agents/tasks/beta-f7/brief.md slice 1.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Registry: run() dispatches through it; every existing command reachable with identical behavior (no per-command drift — spot-check 3+ commands' help/behavior vs main).
2. Menu: TTY-gated (piped/redirected bare run byte-identical to help — verify, F6 tree kept); dynamic from the registry (a new command appears with NO menu edit — verify by reading, or test by adding a probe command in a scratch tree); F6 banner header with plain fallback (fallback path exercised).
3. Non-interactive proof: piped-run test real; pty smoke (menu renders + EOF quits 0) real; 12 filtered tests meaningful.
4. Backups absence: CONFIRM the menu lists no backups item AND that no backup command exists on main (dynamic-correct per master ruling, not a gap).
5. Gates: affected green (reuse-or-rerun cmd/athena through suite); gofmt/build/vet clean; generate stable.
6. Scope: cmd/athena menu/registry/help ONLY (no command behavior/flag changes — fail if any).

Report: say master "F7 SLICE 1 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f7/review-slice-1.md with the verdict line.
