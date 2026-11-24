# browser: the shared Desk harness

One dependency-free Node CDP driver (stdlib only) for every lane's browser
passes. Each lane runs it from its own worktree through the suite lock:

```sh
.agents/bin/suite --label "<lane> browser" -- /abs/path/.agents/bin/browser /abs/path/scenario.js --shots /tmp/<lane>-shots
```

Use the absolute main-repo path for both tools from any worktree. Full
passes never bypass `suite`.

## What it does

1. Verifies the current directory is an athena checkout, then `make build`s
   it (restoring `internal/webui/dist/.gitkeep` after; `bin/athena` stays).
2. Creates its own `test_athena_*` database (never the dev database),
   migrates it, ensures root, and issues root a reset challenge through the
   CLI, completed on the real Desk form (no seeded sessions). Root bypasses
   every gate, so lanes see all of Build — an Instance Owner would not (no
   `build` permission, denied reads).
3. Serves it on a probed-free loopback port with a fake HOME and scratch
   files dir (never the dev HOME — the user's server holds the install
   lock, so a shared HOME would refuse to start).
4. Opens headless Chrome on a probed-free CDP port with a fresh profile.
5. Completes the reset challenge on the Desk form as root (no seeded
   sessions), sets the requested theme, then runs your scenario.
6. Screenshots to `--shots`, collects console/page errors throughout, and
   always closes Chrome and the server (try/finally — no orphans even on
   failure), drops the database, and removes its scratch dirs.

## Args

| Flag | Meaning |
| --- | --- |
| `<script.js>` | Scenario file (required): must `module.exports` one async function. |
| `--shots DIR` | Screenshot folder, created if missing (default: fresh temp dir, printed). |
| `--theme light\|dark` | Desk theme before the script runs (default `light`). |
| `--port N` | Pin the server port; must be free and outside the exclusion list. Default: first free port in 3052–3079 outside the list. |
| `--db NAME` | Database name, must start with `test_athena_`. Default: `test_athena_browser_<port>`. |
| `--self-test` | Run the driver's unit checks (arg parsing, DSN/challenge parsing, port exclusion). No Chrome, server or database. |
| `--help` | Usage. |

`KEEP=1` keeps the database and scratch dirs for inspection (paths print).
`CHROME_PATH` overrides Chrome discovery. DSN comes from
`ATHENA_TEST_DATABASE_URL`, else `ATHENA_DATABASE_URL` in the target's
`.env.local` (MariaDB form only).

## Exit codes

- `0` pass: the scenario ran and no page errors fired.
- `1` app failure: scenario asserts, visible app errors, an unbuildable
  tree, or a wait that timed out with app-error evidence.
- `3` harness failure: timeouts with no app error, CDP drops, missing
  prerequisites (pnpm, mariadb/mysql, Chrome, DSN), port/DB setup failures.

Timeouts always print the page URL, a text snippet, and any page errors.
Exit 0 never reports an inconclusive run.

## Scenario API

The exported function receives one helpers object:

- `BASE`, `DIR` — server origin and shots folder.
- `goto(path)` — navigate to a Desk route (`/build/settings`).
- `shot(name)` — capture the viewport to `DIR/name.png` (1280×900 default).
- `setViewport(w, h)` — resize before a shot (width matrix).
- `waitFor(jsExpr, what, timeoutMs?)` — bounded poll (default 20 s) with
  timeout diagnostics; never raw sleeps.
- `evaluate(jsExpr)` — read page state (values, computed styles, counts).
- `fill(sel, value)`, `click(sel)`, `clickText(tag, text)`, `key(k, code, n)`
  — real input paths (missing selectors are harness failures).
- `setTheme('light'|'dark')` — pin the stored theme and reload; verifies
  the class applied. Deterministic; never drive theme menus.
- `assertVisible(sel, what?)`, `assert(cond, message)` — app asserts (exit 1).
- `expectNoErrors()` — fail on collected console/page errors (exit 1).

Rules learned the hard way: navigate fresh instead of waiting on menu or
popover animations; assert state (derived keys, checked radios, applied
classes) before submitting; read live values rather than trusting renders.

## Ports and databases: probe-first + hard excludes

Parallel passes must never share a port or database. The driver probes
`bind(127.0.0.1)` for the first free server port (3052–3079) and CDP port
(3771–3799), skipping these pinned lanes even when they look free:

| Lane | Server | CDP | Database |
| --- | --- | --- | --- |
| FU (mine) | 3052 | 3771 | `test_athena_fu2`, `test_athena_browser_*` |
| split | 3062 | 3781 | `test_athena_split_browser` |
| F6 | 3120 | 3791 | `test_athena_f6_browser` |

Claiming a new pair: add it to this table and to `SERVER_EXCLUDE` /
`CDP_EXCLUDE` in `browser`, on main, before the first parallel run. `--db`
overrides the derived database name but never the `test_athena_` prefix.

## Troubleshooting

- `port N belongs to another lane` — drop `--port` and let the prober pick.
- `cannot create database` — MariaDB is down; the harness never starts it.
- `server never healthy` — read `serve.log` in the shots folder.
- `not an athena checkout` — run from the worktree under test; the driver
  builds and serves the current directory.
- Shots of the wrong theme — the theme persists per user; the driver pins
  `--theme` before the script, and `setTheme` re-pins mid-script.
