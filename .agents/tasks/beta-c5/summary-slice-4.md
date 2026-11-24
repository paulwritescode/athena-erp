# Summary: C5 slice 4 (realised FX + settlement reversal)

Merged 2026-11-24 as 9d0ad48 (pick of 3b798c5, content-identical to reviewed 5c7b7bf), 1 commit, 2 files.

Scope: 7.4 — realised FX posted as plans through the G6 engine (both directions, payment==item kept) + settlement reversals that MIRROR applications into the CURRENT-open period (never edits/backdated) + settlement-level period gate (soft: `ledger:close` + reason; hard: refuse) in `internal/posting/settlement.go` + tests. One KES book, named others, unnamed refused. No sentinels. Deferred Z2 edges (genuine 7.6/engine scope): ledger:close perm + audit, statutory-book naming, FX-into-soft, control-source check.

Evidence: worker affected green (7 pkgs); reviewer affected green (8/8). Merge gate on 9d0ad48: exit 0, 26m (19 Go ok — posting 844s, server 633s, auth 557s, all fresh under suite -p 2; 684/684 web; zero FAIL, log 1124-163438). Fast-forward, main unmoved.

Review: r-c5s4 PASS (review-slice-4.md) — FX-as-plans both directions, reversals mirror into current-open, closes respected, no sentinels, scope clean.
