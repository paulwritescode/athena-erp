# Brief: FU-H shared browser harness (Muse, L5 right after batch-2 PASS)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-fu (same tree; batch-2 branch stays as reported). Cut a fresh branch `feat/fu-harness` from latest main (fetch first).
Read: tasks.md (this task is L5-assigned per night-shift orders); your own batch-1/batch-2 drivers (what broke: menu-close races, Reka popovers, seed timing, install lock); an existing lane script for scenario shape (e.g. the 6.10 or split driver if readable — read-only, do not modify other lanes' files).

Goal (Stylus spec, exact): one dependency-free Node CDP driver at `.agents/bin/browser` that every lane runs as `suite -- .agents/bin/browser <script>`. It MUST:
1. Start `athena serve` on a FREE port (probe-and-skip held ports; never a fixed port that collides) with its OWN `test_athena_*` database and a fake HOME (never the dev database, dev home, or another lane's DB/ports).
2. Create the first user and sign in through the CLI challenge (no seeded sessions).
3. Wait for the Desk by a STABLE selector (never sleep-based; bounded waits with diagnostics on timeout).
4. Set theme light/dark via the class toggle (deterministic; verify applied).
5. Visit routes from the script, screenshot to a GIVEN folder, collect console/page errors throughout.
6. ALWAYS exit and close Chrome (try/finally; no orphaned processes even on failure).
7. Exit codes: 0 pass / 1 app failure / 3 harness failure (3 = environment/driver problem: timeouts with no app error, CDP drops, port/DB setup failures). Never exit 0 on an inconclusive run.

Script format (you define it, keep it minimal): `<script>` is a Node file using the driver's exported helpers (e.g. `goto`, `shot(name)`, `assertVisible`, `setTheme`, `expectNoErrors`). Ship one example script exercising sign-in → Desk → theme flip → one route + shot, plus a README section documenting args (`--shots`, `--theme`, `--port`, `--db`), exit codes and the no-shared-ports rule.

Slices (report once at the end with say master "FU-H: PASS or BLOCKED <one line>"; rebase on main before reporting PASS):
1. Driver core + example script + README: implement the contract above; prove it by running the example green twice in a row through `suite` (gate the runs yourself).
2. Adoption readiness: delete nothing in other lanes; verify the driver runs from a second worktree path (it must not depend on cwd); document the per-lane port/DB assignment convention (probe-first + hard-exclude list).

Files allowed: `.agents/bin/browser` (new), `.agents/bin/browser-README.md` or README section (new), example script (new), tests if the driver has unit-testable parts. NOT allowed: `.agents/bin/suite` (Stylus owns it), any product code, any other lane's files.

Rules: dependency-free Node (stdlib + CDP only — the machine has no spare npm scope for this); telemetry off; throwaway DBs/Home only; never the dev database/home/ports (the user server holds the install lock — your attempt-2 lesson). Full verification runs only through `.agents/bin/suite` (it queues across slots; never bypass). Commit on `feat/fu-harness` (restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents.
Checks: shellcheck-style care (set -euo pipefail in wrappers), `node --check` on all JS, two consecutive green example runs through suite, an intentional failure drill (bad selector → exit 3 with diagnostics; broken app route → exit 1), no Chrome processes left behind (`ps` verified).
Report: say master "FU-H: PASS or BLOCKED <one line>" + commit + docs path + the two green log names. Then wait (review + merge follow; lanes adopt from main after merge).
