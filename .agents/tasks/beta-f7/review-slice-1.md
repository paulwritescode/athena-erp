# Review: F7 slice 1 — bare-athena menu from command registry

Reviewer: r-f7s1. Worktree: ../athena-erp-f7-s1-r, detached at 4be81da,
verified clean on arrival and at the end (`git status --short` empty both
times). Two commits on 3e1bc9d: `0f37d45 feat(cli): F7 slice 1
bare-athena menu from command registry` + `4be81da feat(cli): F7 slice 1
menu header uses the F6 banner`. Diff is exactly the 4 files the brief
names: README.md (+1 line), cmd/athena/main.go (registry + dispatch +
printUsage split), cmd/athena/menu.go, cmd/athena/menu_test.go.

Verdict: PASS

## Blockers

None.

## Checks (brief items 1–6)

1. Registry — PASS. run() dispatches through lookupCommand over the new
   `commands` table; all 21 old switch cases map 1:1 (init/new/run/serve
   keep their exact call shapes; files keeps os.Stdin; version/help keep
   byte-identical stdout via Fprintln/printUsageTo; --version/-v and
   --help/-h normalize to the same entries; unknown command keeps
   printUsage + the same error). Spot-checked 8 invocations against a
   base-tree (3e1bc9d) binary: help, --help, -h, version, --version, -v,
   `records bogus-subcommand`, `ledger bogus-x`, `rules bogus-x` —
   stdout byte-identical, same exit codes; unknown-command stdout
   identical and stderr identical modulo the log timestamp.
2. Menu — PASS. TTY-gated on stdin AND stdout (isInteractive); piped and
   redirected bare runs byte-identical to `athena help`, F6 tree kept
   (verified against the built binary, not just tests). Dynamic from the
   registry: printMenu iterates `commands`, menuPick resolves through
   lookupCommand — proven empirically by adding a zzprobe command in a
   scratch worktree with NO menu edit: it appeared in the menu listing
   and ran (expect-driven pty, exit 0; scratch removed after). F6 banner
   header present in the menu (pty-verified). Plain fallback exists but
   its path is NOT exercised, see follow-up 1.
3. Non-interactive proof — PASS. TestBareAthenaWithoutTerminalPrintsUsage
   is real: run(nil) vs run(["help"]) byte-compared through a genuine
   os.Pipe. TTY smoke real: menu renders + EOF quits 0 on a real pty,
   and a full expect-driven chain (pick 22 → "Arguments for version" →
   version output → re-prompt → `No menu entry "99"` → re-prompt → q →
   exit 0). Filter count: 10 new tests + the 2 existing banner tests =
   12 under a matching filter, all green; one test-quality note, see
   follow-up 2.
4. Backups absence — PASS (dynamic-correct, not a gap). No backups item
   in the menu (grep 0 over every pty transcript) AND no backup command
   exists: `grep -rin backup cmd/athena/` empty on this tree and on base
   3e1bc9d. The dynamic menu correctly lists no backups entry.
5. Gates — PASS.
   - `gofmt -l cmd internal`: empty. `go build ./...`: ok.
     `go vet ./cmd/athena`: ok.
   - `affected` (through the suite lock): `ok athena-erp/cmd/athena
     162.234s`, exit 0 on 4be81da.
   - `make generate` (nvm node + shared pnpm store): stable, tree clean
     after. (Direct `make generate` fails in agent shells without node
     on PATH — environmental, not a slice issue; no proto inputs.)
   - golang.org/x/term was already a dependency (go.mod + main.go);
     no new dependency.
6. Scope — PASS. cmd/athena menu/registry/help + one README line ONLY.
   No command handler, flag, or help-text change: the main.go diff is
   dispatch + the printUsage/printUsageTo split, and help output is
   byte-identical to base. No F6 files touched (banner consumed via the
   merged usageBanner), no RecordsPage/api.ts/proto/server changes.

## Follow-ups (non-blocking; neither breaks behavior, data, or security)

1. menuHeader's plain fallback is unreachable as written: usageBanner()
   is a plain func returning const concatenation (asciiMark +
   version), so it can never return "". The fallback branch cannot be
   exercised without a refactor (e.g. a banner variable) or it can be
   dropped. The real path (banner present) IS asserted by
   TestMenuListsEveryRegisteredCommand.
2. TestCommandAliases re-implements the alias switch inside the test
   instead of driving run() — it would pass even if run() dropped the
   aliases. Suggest driving run(["--version"]) etc. through
   captureStdout. The underlying behavior was verified by this review
   (byte-identical alias outputs vs base).

Note: main has moved to 96acb15 and feat/f7-cli to 15925e5 since this
slice; the review pins 3e1bc9d..4be81da per the brief — master rebases
at merge. Review worktree ../athena-erp-f7-s1-r left clean at 4be81da
for the merge gate; scratch worktrees removed.
