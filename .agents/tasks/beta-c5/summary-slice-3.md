# Summary: C5 slice 3 (Property 11 allocation sums)

Merged 2026-11-24 as ccb73c9 (pick of a1b5917, content-identical to reviewed 6a879f4), 1 commit, 2 files.

Scope: tests only — `internal/types/types_test.go` (+100, append-only, no import touch) and `internal/posting/settlement_test.go` (+171). Genuine rapid properties (100 checks each): splitter exact-sum/determinism/zero-weight/tie-break; settlement applied+unapplied==payment, id-ordered model match, stored residuals, party-row sides. Validates 7.3. No Z2 touch.

Evidence: worker affected green (types/posting/importers); reviewer affected green (cmd/athena 439s, ledger 445s, posting 850s, server 820s, exit 0). Merge gate on ccb73c9: exit 0, 19.5m (19 Go ok — posting 1119s, server 1088s DB-backed, auth cached; 684/684 web; zero FAIL, log 1124-131114). Fast-forward, main unmoved.

Review: r-c5s3 PASS (review-slice-3.md) — properties genuine at both levels, append-only holds, no Z2 touch.
