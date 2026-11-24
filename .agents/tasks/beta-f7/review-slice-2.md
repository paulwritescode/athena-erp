# Review: F7 slice 2 athena doctor (r-f7s2)

Verdict: PASS

Tree: `../athena-erp-f7-s2-r`, detached at `0b4e563` (commit `feat(cli): F7 slice 2 athena doctor with five checks`). Tree verified clean; slice-2 commit diff is scope-only: `cmd/athena/doctor.go` (+225), `cmd/athena/doctor_test.go` (+156), `cmd/athena/main.go` (+7: registry entry + usage stanza), `README.md` (+1 bullet). No command behavior or flag changes elsewhere.

## Brief items

1. Five checks, each meaningful — PASS. `mariadb`, `migrations`, `disk`, `port`, `frontend` all present. Failure path verified live: unreachable DB fails `mariadb` ("cannot reach mariadb/nodb: dial tcp ... connection refused") and `migrations`, exit 1. Busy/invalid port warns with a meaningful detail (live: invalid port → `warn port ... invalid port`). Fail-vs-warn scheme is coherent (fail = broken now: unreachable DB, unreadable migration state, unstattable disk; warn = needs attention: pending migrations, busy port, missing built UI, low disk).
2. Contract — PASS. Exit 0 when no check fails / 1 when any fails (`main()` logs and `os.Exit(1)`; live failing run `EXIT=1`, healthy path covered by `TestDoctorRunsFiveChecks`). Last line stable: `doctor status=<ok|warn|fail> checks=5 ok=2 warn=1 fail=2`. `--json` is valid JSON with the same verdicts (parsed live; counts match the text run) and stdout stays pure JSON (the error goes to stderr).
3. No-secret leak — PASS. Label path uses driver/dbname only (`doctorTargetLabel`); DSN parse errors are the generic "invalid database URL"; `resolveWorkspace` errors surface only when no DSN fallback applies. Live failing run with `doctor-s3cret` in the DSN: password absent from stdout, JSON and the stderr error line (grep confirms).
4. Wiring — PASS. Registry entry only; menu iterates `commands` (`menu.go:71`), so doctor appears dynamically — `TestMenuListsDoctor` green. `doctor help` and top-level `help` stanza verified live; README bullet matches the usage text.
5. Gates — PASS. `affected` green through suite: `ok athena-erp/cmd/athena 134.937s`. `gofmt -l cmd internal` empty, `go build ./...` and `go vet ./cmd/athena` clean, `make generate` stable (tree clean after; needed nvm node on PATH — plain-PATH `node` is missing in this environment, unrelated to the slice).
6. Scope — PASS. Doctor + wiring only; +389 lines, zero changes to other commands.

## Notes (non-blocking)

- Warn-not-fail for busy port and pending migrations is a judgment call; it reads as intended (machine line and JSON both expose warn counts, so automation can key on them).
- `make generate` requires node, which is only available via nvm (`~/.nvm/versions/node/v26.7.0/bin`) in this environment — worth a PATH note for agents, not a slice issue.
