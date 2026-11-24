# Summary: C2 secrets slice 4 (lifetimes + disposal)

Merged 2026-11-24 as fe41dcd (pick of 75bb9df onto post-TS main), 1 commit, 5 files.

Scope: 8.10 — lifetime attributes carried in `internal/server/attributes.go` + `record_types.go` (append-only, flat JSON, validated, preserved across saves); new `internal/jobs/disposal.go` (preview/apply/holds/restore-reapply) honoring the exact Z4 schedule (7yr ledger/invoice/stock/audit never auto-deleted in beta; Access Log 1yr; outbox/events 30d; job runs 90d) + disposal_test.go + lifetime_test.go. Honest gap: guest anonymisation needs field markers (L3/G18), code does not pretend otherwise.

Evidence: worker affected green carried under the disjoint rule (5 files byte-identical). Merge gate: first run RED on contention (posting 20m timeout as sole Go run, all else green, log 1124-140840); retry-p4 GREEN exit 0, 6.2m (19 Go ok — posting 355s fresh; 684/684 web; zero FAIL, log 1124-143705).

Review: r-c2s4 PASS (review-secrets-slice-4.md) — lifetimes per design, disposal real with exact Z4, guest gap honest, no secret material, disjoint carry verified + 8 live tests.
