# Summary: TS slice 3 (posting shared-DB)

Merged 2026-11-24 in the s2+s3 train (tip 953c4d8; picks of b9b5f02+690a1a9 + AFTER amend), posting _test.go + measure.md.

Scope: posting setup() no longer migrates per test — new TestMain migrates once with sharedDatabase + Reset per test (server/shared_test.go pattern); all 5 migrate sites converted, zero keepers (no parallel/DDL/migration-state tests); no asserts touched, no tests deleted. measure.md carries before/after numbers (AFTER row amended at merge per reviewer recommendation).

Evidence: BEFORE posting-alone GREEN 396.2s (scratch tree, TestPostValidation 171.7s setup-bound, ~6s migrate floor x27); AFTER master-priority gate GREEN 13.7s (~29x, log 1124-193523). Train gate: exit 0, 21m (20 Go ok — posting 32.7s in full run; 684/684 web; zero FAIL, log 1124-212652).

Review: r-ts3 PASS (review-slice-3.md) — conversion real (5/5 sites, zero keepers hold), no weakening, numbers verified full-package.
