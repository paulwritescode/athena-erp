# Brief: F7 slice 3 — completions + help consistency + E2E script (Muse, single slice)

Lane: L6 after C4s5 merges (Stylus order 2026-11-24; master orders the start; branch feat/f7-cli, rebased empty on main).
Read: tasks.md F7; .agents/tasks/beta-f7/brief.md (source scope, old slices 3–5 now ONE slice); cmd/athena/ registry (new commands since slice 1: doctor, C4s5 tree/uom/determination-asof, C2s5 secrets — all covered); README CLI section.

Acceptance mapping (every remaining F7 line → slice-3 work):
1. "Shell completions (athena completion zsh|bash|fish)" → `athena completion` command generating all three shells from the registry (no hand-written drift); test: generated scripts parse (zsh -n / bash -n / fish --no-execute) and complete every registered command name.
2. "Consistent --help with examples" → every command's --help has usage + one example minimum, same flag vocabulary; test: help-text test asserting each registered command's help contains an example block.
3. "E2E script runs every documented command" → script (repo-owned, runnable through suite) executing every documented command against test_athena_f7_e2e with fake HOME; green run required.
4. "README and help agree" → README CLI section regenerated or diff-checked against --help output (names/flags word-for-word); test or script step fails on drift.

Task (one slice, merging whole; report: say master "F7 SLICE 3: PASS/BLOCKED <one line>", rebase on main first):
Generate, don't hand-write (completions + README check derive from the registry so new commands can't drift).

Rules: batch-2 conventions; cmd/athena behavior/flags UNCHANGED (help/completion/e2e only — fail the slice if any command behavior moves); no secrets in help/errors; telemetry off unless set. Throwaway DBs + fake HOME. Full suites/browser only through .agents/bin/suite (suite -p default applies); package tests direct. Commit on feat/f7-cli; never merge/push/spawn.
Checks: gofmt empty, go build/vet, affected (cmd/athena) + full suite through the lock, generate stable, e2e script green.
