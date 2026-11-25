# Summary: TS slice 5 (auth fix + cmd shared-DSN + GO_TEST_P ?= 4)

Merged 2026-11-25 as 4 commits (tip 5ef34c8).

Scope: (5a) auth: dropped redundant per-iteration migrates on the shared DB
(profiled 81.1s alone, owner property 70.6s; zero testdb.New sites, zero
timers, Now-seam already used — no clock target exists; after-run 79.8s).
(5b) cmd/athena tests share one migrated database (TestMain + sweep + cheap
hashing + shared migrate + sharedDSNEnv + Shared cfg accessor, ~20 sites;
keepers migrate/doctor-fresh/ledger-connector/5 workspace tests why-commented;
after-run 44.2s). Numbers recorded. Makefile: GO_TEST_P ?= 4 + go test -p
$(GO_TEST_P) (dry-run verified default -p 4 + override; suite default -p=4
set separately by Stylus in tooling).

-p shootout on full runs: -p 4 GREEN 513s wall vs -p 2 836s (39% win; auth
464s still the pole, server 272s, files 138s, cmd 123s, secrets 99s).
7-minute target NOT yet met; slice 6 (files, secrets, auth last) waits for a
free lane per Stylus ruling.

Gates: worker after-runs green; -p 4 full retry green pre-merge.
Review: r-ts5 PASS (setup-only diffs, oracles untouched, conversions real,
numbers recomputed from logs, Makefile delta reviewed).
Merge gate: full `make test` exit 0 clean, 561s, 22/22 Go pkgs ok + 735 web
(log 1125-033500-master.log). Picks rebased onto post-F4 main (clean,
disjoint: test+Makefile vs F4 product files); build+vet green.

Test-only + Makefile: no server restart. L7 RESTING after this merge (w-ts
stood down, branch kept for slice 6); slot goes to w-f2/F5.
