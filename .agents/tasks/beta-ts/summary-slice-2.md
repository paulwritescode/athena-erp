# Summary: TS slice 2 (sweep stale test DBs + stamped names + pool docs)

Merged 2026-11-24 in the s2+s3 train (tip 953c4d8, 5 commits: 2 slice-2 + 2 slice-3 + AFTER amend; rebased clean onto post-parseTime main), test files + README + measure.md.

Scope: throwaway DBs named test_athena_<pkg>_<unixts>_<rand12> (pkg via runtime caller walk, zero call-site changes); testdb.SweepStale(1h) in auth+server TestMains (drops only provably-stale, keeps everything doubtful, non-fatal); deterministic sweep_test.go + shape_test.go; innodb_buffer_pool_size=512M sentence in README tests section. No production code touched.

Evidence: focused testdb green 7.6s (master-priority); affected 7 pkgs green (1103s). Train gate: exit 0, 21m (20 Go ok all fresh — posting 32.7s with slice-3 conversion; 684/684 web; zero FAIL, log 1124-212652).

Review: r-ts2 PASS (review-slice-2.md) — sweep conservative, names stamped, both TestMains wired, affected green.
