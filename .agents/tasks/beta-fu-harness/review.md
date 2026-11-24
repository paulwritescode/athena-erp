# Review: FU-H shared browser harness (tool review)

Reviewer: r-fuh. Tree: `athena-erp-fu-h-r`, verified clean on arrival at
`1e1450f` (2 commits: `cb9a3c3` driver+example+README, `1e1450f`
root-via-reset sign-in). Diff `main...HEAD` is tool files ONLY:

- `.agents/bin/browser` (682 lines, new)
- `.agents/bin/browser-README.md` (106 lines, new)
- `.agents/bin/browser-example.js` (18 lines, new)

No product, proto, migration, gate, web, server, or suite changes.

Verdict: PASS

## Blockers

None. All merge-small gates hold: no contract gap on any exercised path,
no dev/prod data path, exit codes correct on every demonstrated path, no
orphans, example green on the reviewer's own unmodified run.

## Contract compliance (brief item by item, line by line)

1. Free-port probe-and-skip — PASS. `tryBind`/`freePort`
   (`.agents/bin/browser:208-221`) probes `bind(127.0.0.1)` over
   3052-3079 / 3771-3799 minus `SERVER_EXCLUDE`/`CDP_EXCLUDE`
   (`browser:54-55`); pinned `--port` is refused when excluded or held
   (`browser:560-562`). No fixed colliding ports.
2. Own `test_athena_*` DB + fake HOME — PASS. `checkDbName`
   (`browser:105-110`) gates both `--db` and the derived default
   (`browser:567-568`); `DROP/CREATE` touches only that name
   (`browser:374-384`); child env pins `ATHENA_DATABASE_URL` to the
   throwaway DB, `HOME`/`XDG_CONFIG_HOME`/`ATHENA_FILES_DIR` to scratch
   (`browser:594-600`). Verified: after my run, no `test_athena_rfuh_*`
   or `test_athena_browser_*` databases remain (control query listed 101
   DBs; leftovers are other lanes'/Go tests', untouched).
3. First-user creation + sign-in via CLI challenge, no seeded sessions —
   PASS with disclosed redesign (see below). `user reset administrator`
   via CLI (`browser:609`), completed on the real Desk form
   (`completeResetChallenge`, `browser:426-445`).
4. Stable-selector waits, bounded, diagnostics, never bare sleeps — PASS.
   `waitFor` (`browser:318-336`) polls a page predicate with a timeout
   and throws with URL + text snippet + page errors via `diagnose`
   (`browser:338-350`); remaining `sleep` calls are poll intervals and
   bounded shutdown waits only. Raw sleep is not exposed to scenarios.
5. Theme set + VERIFY applied — PASS. `setTheme` (`browser:449-462`) pins
   `localStorage['athena-theme']` (matches `KEY` in
   `web/src/composables/useTheme.ts:10`), reloads, and asserts the
   `documentElement` `dark` class (the mechanism in `useTheme.ts:19`);
   failure becomes `AppError`. Both themes verified applied in live runs.
6. Route visits / shots to the given folder / error collection — PASS.
   `goto` (`browser:478`), `shot(name)` to `--shots` (`browser:465-471`),
   `Log.entryAdded` + `exceptionThrown` + `console.error` collection
   (`browser:299-313`, 500-event buffer) with a final `PAGE_ERRORS` gate
   (`browser:640-642`) and `expectNoErrors` (`browser:535`).
7. Always exit + close Chrome — PASS. `try/finally` (`browser:645-663`):
   `cdp.close`, `killNicely` SIGTERM-then-SIGKILL for Chrome and server
   (`browser:241-250`), post-cleanup alive warnings, best-effort DB drop
   + scratch removal; `unhandledRejection` exits 3 (`browser:666-669`).
   External kills are covered suite-side (child in its own session,
   `killpg` tree on timeout/signal, `suite:207-217,231-250`); a direct
   Ctrl+C reaches the whole foreground group, so children die with it.
   `ps` after the worker's runs and after mine: only the user's dev
   server on :3000 (and, earlier, F1's live lane run) — no harness
   orphans, no stale `/tmp/browser-run-*` scratch.

## Sign-in redesign (brief item 2)

Evidence supports the call: `1124-034650-w-fu.log` (journal exit 1) shows
the owner flow failing with two `403 (Forbidden)` page errors after an
otherwise clean run — denied reads, exactly as disclosed. Root-based
harness auth is ACCEPTABLE for a test tool: `checkDbName` is a single
gate on every DB path (dev/prod names unreachable), the password is
random per run and never printed, the challenge is never printed (grep
for "challenge" in my `run.log`/`serve.log`: zero hits), HOME is fake
(dodges the user's install lock), and the suite lock serializes runs.
This echoes the B11 Instance-Owner question — recorded as follow-up F5,
not expanded here per the brief.

## Exit codes (brief item 3)

`AppError` -> 1, `HarnessError` -> 3, unknown -> 3 (`browser:32-50`,
`browser:675-681`); exit 0 only after the scenario plus the final
`PAGE_ERRORS: 0` gate, so no inconclusive pass. Journal confirms:
`1124-050940`/`1124-051010` exit 0 (greens), `1124-051049` exit 3
(bad-selector timeout with URL/snippet diagnostics),
`1124-051119` exit 1 (broken-route assert with URL/snippet
diagnostics), `1124-051203` exit 0 (second worktree: driver from
`athena-erp-fu` run with cwd `athena-erp` — proves no cwd dependence).
One latent gap is follow-up F1, not a demonstrated wrong exit.

## Reproduction (brief item 5)

Reviewer's own run, unmodified example, through `suite`:

- `suite --label "r-fuh review run" -- <r-tree>/browser <r-tree>/browser-example.js --shots /tmp/rfuh-shots --port 3055 --db test_athena_rfuh_review`
- Journal exit 0, clean tree, head `1e1450f`, 97 s, log
  `1124-052449-r-fuh.log`; `settings.png` + `settings-dark.png` written;
  server 3055 + probed CDP 3771; DB dropped afterwards.
- Also reproduced: `node --check` on both JS files clean;
  `browser --self-test` 16/16 (all 16 cases asserted in
  `browser:128-164`, counted and green).

## Docs (brief item 6)

README args table matches `parseArgs` (`browser:62-83`), exit codes match
the code, the probe-first + hard-exclude convention is documented with a
claim-a-pair rule, and troubleshooting matches observed failures. A new
lane can adopt it. Nits are follow-ups F2-F4.

## Follow-ups (non-blocking)

- F1: clean CDP socket close with pending calls has no `close` handler
  (`browser:252-291`), and the `fetch` calls in `waitForHealth` /
  `waitForDebugger` (`browser:396-421`) have no timeout — a silent drop
  would hang to the suite watchdog instead of exiting 3. Rare
  (ECONNRESET surfaces via `error` -> exit 3), bounded, orphan-free.
  Suggest a `close` listener rejecting pending calls plus
  `AbortSignal.timeout` on the fetches.
- F2: `browser-example.js:2` still says "signed in through the invite
  challenge" — stale after `1e1450f` (now reset challenge).
- F3: README exclusion table labels 3052/3771 "FU (mine)" —
  worker-personal wording in a shared doc; also steps 2 and 5 repeat the
  reset-challenge sentence.
- F4: README could state "never `--db` another lane's database" — a
  colliding `--db` would `DROP` it (dev/prod still unreachable via
  `checkDbName`, so not a blocker).
- F5: the owner-lacks-build finding echoes the B11 Instance-Owner
  question; follow up there, out of FU-H scope.
- F6 (defense in depth): no explicit `process.on('SIGINT'/'SIGTERM')` in
  the driver; group-directed signals already reach the children, so this
  only matters for pid-targeted kills.
