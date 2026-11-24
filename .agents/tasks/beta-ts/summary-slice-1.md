# Summary: TS slice 1 (cheap Argon2 in auth TestMain)

Merged 2026-11-24 as 1c757a0 (pick of e73ec95 onto post-C5s3 main), 1 commit, 3 files.

Scope: test-only — `internal/auth/shared_test.go` TestMain sets cheap {8MiB,1,1}; `internal/auth/auth_test.go` 2 password tests pin production {64MiB,3,4} with save/restore; `measure.md` numbers. Oracles unchanged, no production file touched, no t.Parallel added. Production default still tested (pins verified against password.go:33 by master delta-review).

Evidence: master-priority auth-only gate on e73ec95: exit 0, `ok internal/auth 188.020s` (log 1124-133042) vs 1178s baseline inside full gates — ~6x, removing the parallel-run starvation of posting/server. No before-run per Stylus order (baseline already in tonight's logs). Pick onto current main conflict-free (main never touched internal/auth since the TS base); build+vet green on the combined tip.

Review: master delta-review PASS (test-only, pins exact, oracles intact); solo merge per explicit Stylus order. No independent reviewer (noted as variance).
