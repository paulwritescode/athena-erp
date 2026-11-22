# Review: B11 slice 1, CLI install path + guide

Branch `feat/b11-install` (2c30c84) in `../athena-erp-b11`, diffed against `main`.

Verdict: PASS

No blockers. Three follow-ups below (F1–F3); none stops the merge.

## Checks run (all green)

- `gofmt -l cmd internal` — empty.
- `go build ./...`, `go vet ./...` — pass.
- `make test` (exit 0) — Go: 14 packages ok with MariaDB tests enabled via
  `.env.local` fallback (throwaway `test_athena_*` databases only); web: 49
  files / 529 tests pass.
- `go test ./cmd/athena/ -run 'TestNew|TestRun'` — all 7 pass, including the 3
  new tests (`TestNewFlagsWorkAfterTheName`,
  `TestNewMissingLogoFailsBeforeTheNamePrompt`,
  `TestRunRejectsPositionalArguments`).
- `scripts/e2e-install.sh` — 28 passed, 0 failed, from a fresh `make build`.
  Throwaway-only verified: fake `HOME` + scratch under `/tmp`, database
  `test_athena_b11_slice1` only (guarded by a `test_athena_*` prefix check),
  loopback `127.0.0.1:3211`. After the run: scratch dirs gone, database
  dropped (`SHOW DATABASES LIKE` empty), server stopped, no lingering process.
- `make generate` twice — stable, no diff (the one working-tree change from
  verification, the web build deleting `internal/webui/dist/.gitkeep`, was
  restored; tree is clean at HEAD).
- Scope — diff is `cmd/athena/main.go`, `cmd/athena/new_test.go`,
  `docs/README.md`, `docs/guides/getting-started.md`,
  `scripts/e2e-install.sh` only. No Desk, proto, migration or server changes.

## Item-by-item (review brief)

1. CLI fixes — `cmd/athena/main.go:261-300`: `new` splits leading
   positionals from the flag run, so `--driver`/`--logo` work after the name
   as well as before, and every positional anywhere still joins into the name.
   `--logo` is read before the interactive prompt (fail fast), and `run`
   rejects positionals (`main.go:338-340`) with `unexpected argument %q`,
   matching `init`'s existing message. Existing tests
   (`TestNewPostgresWarns`, `TestNewMariaDBStaysSilent`) still pass, so no
   other init/new/run behavior changed; B5 identity + B8 root code untouched
   (no diff outside `cmd/athena`). Errors are plain language.
2. E2E script — 28/28 green (counted call sites: 4 init + 12 new + 4 run +
   5 sign-in + 3 CLI = 28). Covers the trailing-`--logo` fix, driver
   mismatch, missing logo, duplicate name, outside-home, run-arg rejection,
   second-server lock, root seeding handoff, sign-in, password rotation and
   first-owner invite.
3. Guide — `docs/guides/getting-started.md` matches the tested path. Every
   quoted error verified against source: `workspace.go:105,108` + `create.go:68`
   (`(from name …)` suffix), `workspace.go:166,199`, `main.go:35,248,284`,
   `lock.go:55`, `server.go:124,127`, `access.go:215`, `identity.go:80`;
   root-seeding rule matches `CheckPassword` (`password.go:63-71`,
   12–256 chars, not the username); scaffolding list matches `create.go:134-137`;
   desk references exist (`SettingsPage.vue:25` Workspace identity,
   `SetupPage.vue:12` empty state). `docs/README.md:14` index row added.
4. Scope — CLI + script + docs only (see above).
5. Full gate — all run as listed above. No browser pass (slice 2 owns it).

## Follow-ups (non-blocking)

- F1: `README.md` "Homes and Workspaces" has no direct link to the new guide;
  the brief asked for README + docs index rows and only `docs/README.md:14`
  got one. The guide is still one hop away via README's "Start with
  `docs/README.md`", so this is a docs-link follow-up, not a blocker.
- F2 (nit): the guide quotes CLI errors as `athena: …` but the binary prints
  them via `log.Printf` (`main.go:35`), which prefixes a timestamp. Consider
  noting the prefix or showing one full line.
- F3 (nit): the e2e script uses a fixed slug and port, so two concurrent runs
  collide (`ADDR` is overridable, the slug is not). Consider a `SLUG`
  override for parallel workers.

## Environment note (not a slice defect)

A bare shell here has no `node` on `PATH` (`make build` fails at `web-deps`
with `env: node: No such file or directory`). All build/test/generate runs
above used `~/.nvm/versions/node/v26.7.0/bin` on `PATH` with the committed
lockfile.
