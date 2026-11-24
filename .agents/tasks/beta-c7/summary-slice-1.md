# Summary: C7 slice 1 (migration 21 + ADR-0032)

Merged 2026-11-24 as 6c99d3a (cherry-pick of staged 15ff044; branch commit 2625faf), 1 commit, 5 files, 402 insertions.

Scope: migration 21 (`l_stock`, `l_stock_value`, layers, applications, bins, reservations, serials, batches, integrity log, rebuild state) + migration test + ADR-0032 "Stock ledger and reservations" (Proposed, transcribes Z3 exactly) + decision-log/README rows.

Evidence: worker full suite exit 0 (Go all-cached-from-real, 616/616 web fresh, 14s — questioned, cleared by rerun); UNCACHED `go test -count=1 ./internal/server` exit 0, 267.8s, zero FAILs; gofmt/build/vet clean, generate stable. Merge War story: two staged gates (32s, 13s) ran without a database (fresh merge worktree lacks gitignored .env.local) — both VOIDED by the master pre-merge; real re-gate with env: 1042s, 17 Go pkgs ok (server 1028s, posting 1012s, ledger 626s, cmd/athena 619s), 66 files / 632 web passed, zero FAIL. Full-night audit: only those two gates (+ two narrows) were hollow; suite now refuses Go runs without DB env.

Review: r-c7 PASS (review-slice-1.md) — migration applies cleanly, ADR transcribes Z3 exactly, uncached evidence verified, migration tests rerun green. No follow-ups recorded.
