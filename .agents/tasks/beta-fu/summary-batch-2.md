# Summary: FU batch 2

Merged 2026-11-24 in train4 (tip bafe51d), 9 commits 77971a2→58a8cf4 (77971a2 B5 role help; 0ffed3a B5 new --logo; c5cc932 B5 SetIdentity conflict; b712790 B5 prune best-effort; d195c62 B5 favicon cache; 726a4f7 B5 unchanged set; 80ec827 FU-1 wheel; 218803c C1 nit; 58a8cf4 chart :value fix).

Scope: B2a slice 1 verify-only (all six already fixed by b4791a3, code-verified, no changes), 6x B5 CLI/docs items, FU-1 Custom wheel indicator, C1 nit (overview log-write wording), +1 browser-found real bug (chart color inputs used :model-value, fixed to :value). Skipped per brief: --chart-palette flag, B3/Security-RPC/B9f1/B2a-s2-3/B8/B10/B11/C-lane items, operations.go:21 comment.

Evidence: full make test exit 0 pre-rebase (17 Go pkgs, 616/616 Vitest); rebase clean (C4s3/C5s1 touched none of our files); browser under the interim harness rule (invoice-editor timeouts, quoted). Train4 tip gate: 17 Go pkgs ok, 632/632 web, zero FAIL.

Review: r-fu PASS (review-batch-2.md) — editor timeouts proven load flake via clean-main + train2 + tip controls (editor green 0.3s/1 row/0 errors on all three). Follow-up: records-flow shots (records/dark/dialog-key) remain uncaptured; FU-H owns driver hardening.
