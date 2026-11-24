# Brief: F7 CLI that helps (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-f7, branch feat/f7-cli from latest main. First: git status clean + git log main..HEAD empty.
Read: tasks.md F7 (beta tasks.md line 167); .agents/stylus/night-shift-orders.md F7; README CLI section; cmd/athena/ (all commands — read the registry shape); F6's banner is UNMERGED (feat/f6-logo) — do NOT read or build on that branch.

Task: bare `athena` in a terminal opens a menu (start server, create Workspace, users/roles, Record Types, records, rules, ledger, backups, doctor, help) built on the same commands; non-interactive use byte-identical (TTY detection; piped/redirected bare `athena` keeps today's behavior). `athena doctor` (MariaDB reachable, migrations applied, disk, ports, node-not-required-at-runtime). Shell completions `athena completion zsh|bash|fish`. Consistent `--help` with examples on every command. E2E script runs every documented command against a test database; README and CLI help agree word-for-word on names/flags.

Slices (report each: say master "F7 SLICE n: PASS/BLOCKED <one line>"; rebase on main before each PASS):
1. Bare-athena menu, built DYNAMICALLY from the command registry (no per-command hardcode — new commands appear automatically). Header: plain title for now; the F6 banner header integrates after F6 merges (master orders it; do not touch F6 files).
2. `athena doctor` with the five checks above, exit codes + machine-readable line.
3. Completions for zsh/bash/fish (generate, don't hand-write drift).
4. `--help` consistency + examples pass over every command; README/CLI agreement.
5. E2E script (every documented command against test_athena_f7_e2e, fake HOME) + checkpoint (generate stable, full suite, typecheck/build).

Files: cmd/athena/** (menu/doctor/completion/help only — do not change any command's behavior or flags), the e2e script, README/help docs. NOT allowed: RecordsPage.vue, DataTable.vue, api.ts, any proto/server/ledger/stock file, F6 files (they are not on main yet), App.vue.

Rules: Go conventions; no secrets in help/errors; telemetry off unless set. Throwaway test DBs + fake HOME only. Full suites through .agents/bin/suite only; package tests direct. Commit on feat/f7-cli; never merge/push/spawn. TTY behavior: prove non-interactive unchanged with a piped run in tests.
Checks per slice: gofmt empty, go build/vet, affected or full suite where it applies, generate stable.
