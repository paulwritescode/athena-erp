# Brief: C4 slice 5 — CLI remainder + Desk remainder + docs + checkpoint (Muse, single slice)

Lane: L6 after F7 slice 2 merges (Stylus redirect 2026-11-24: CLI affinity with F7's cmd/athena work, one lane avoids conflicts; master orders the start; worktree/branch assigned at dispatch).
Read: tasks.md C4 (slices 1–4 + 13.3b merged); .agents/tasks/beta-c4/brief.md slices 5–6 (13.5–13.7, the source scope); ADR-0029; cmd/athena/ledger.go (determination subcommand EXISTS from slice 3 — extend, do not duplicate); RecordTypesPage/FieldSettingsDialog/fieldRules.ts + internal/server/testdata/field_rules.json (L3-C4 owns these, no live worker — recon what of 13.5 exists first).

Task (one slice, merging whole; report: say master "C4 SLICE 5: PASS/BLOCKED <one line>", rebase on main first):
1. Recon: list exactly which 13.5 CLI + Desk items are missing (CLI `tree` + `uom` subcommands are confirmed missing; determination CLI exists; Desk delta unknown — verify each 13.5 Desk line against the tree and report the delta before building).
2. CLI: `tree` + `uom` subcommands in cmd/athena/ledger.go (append-only edits) + tests.
3. Desk remainder from the recon (effective-date/UoM/as-of UI only; no unrelated Desk changes) + fieldRules.ts/json kept in step (change the rule in records.go + fieldRules.ts + field_rules.json together — the shared-table rule).
4. Docs: `docs/architecture/record-type-and-pack-model.md` + decision-log row.
5. Checkpoint: generate stable, full suite, typecheck/build, browser pass of the touched screens in both themes.

Rules: batch-2 conventions; gate policies if new procedures appear (none expected). Telemetry off unless set. Throwaway DBs + fake HOME. Full suites/browser only through .agents/bin/suite; package tests direct. Commit on the assigned branch; never merge/push/spawn.
Checks: gofmt empty, go build/vet, full suite through the lock, generate stable, web typecheck/test/build.
