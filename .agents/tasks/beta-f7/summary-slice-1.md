# Summary: F7 slice 1 (bare-athena menu)

Merged 2026-11-24 as 032a090 (2 commits 7d49cef+032a090, rebase of a175026+ad8f191 onto post-C2s4 main), 4 files.

Scope: command registry in main.go (run() dispatches through it, byte-identical behavior) + menu.go (TTY-gated bare-athena menu built dynamically from the registry, F6 banner header with plain fallback) + menu_test.go (piped-run proof, pty smoke) + README line. Backups correctly absent (no backup command on main; appears with D4).

Evidence: worker affected green (cmd/athena incl MariaDB); reviewer affected green (exit 0). Merge gate: first run RED on contention (posting 20m timeout as sole Go run, all else green incl. auth 1212s parallel, log 1124-134653); retry-p4 GREEN exit 0, 6.8m (19 Go ok — posting 390s fresh; 684/684 web; zero FAIL, log 1124-143015). Merged after C2s4 under the disjoint-additive rule (conflict-free rebase, code identical to reviewed, build+vet green on the combined tip).

Review: r-f7s1 PASS (review-slice-1.md) — registry byte-identical, menu dynamic/TTY-gated/bannered, 2 test-only follow-ups.
