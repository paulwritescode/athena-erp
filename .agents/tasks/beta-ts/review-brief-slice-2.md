# Review brief: TS slice 2 sweep + stamped names + pool docs (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-ts-s2-r, detached at da178bb (on af1f891). Verify clean + diff is slice-2 scope only: internal/testdb/testdb.go (timestamped names + SweepStale) + sweep_test.go + shape_test.go + TestMain wiring in auth/server shared_test.go + README pool sentence + measure.md numbers.
Read: .agents/tasks/beta-ts/brief.md slice 2; Stylus's 3 orders (compare posting, TestMain cleanup, pool docs).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Naming: throwaways are test_athena_<pkg>_<unixts>_<rand12> with pkg from a runtime caller walk, zero call-site changes — verify the walk yields the right pkg for 2+ packages and nothing parses names.
2. Sweep conservative: SweepStale(1h) drops ONLY provably-stale DBs (old name timestamp, or pre-timestamp name with ALL tables older than 1h); keeps everything doubtful incl. live concurrent runs; non-fatal on error. Verify the keep-doubtful paths by reading + run sweep_test.go.
3. Tests real: sweep_test.go deterministic, no sleeps, skip-safe — verify by running it (must pass, must not flake on re-run).
4. Wiring: both TestMains (auth + server) call the sweep; no production file touched (test-only + README + measure.md).
5. Gates: affected green (reuse-or-rerun testdb + auth + server through the main-repo suite on da178bb); gofmt/build/vet clean.
6. Scope: the files above ONLY (no Makefile -p change — 2a dropped; no migrate-once — that's slice 3; fail if present).

Report: say master "TS SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-ts/review-slice-2.md with the verdict line.
