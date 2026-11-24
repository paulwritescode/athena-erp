# Rules for every Stylus agent (binding)

You run in YOLO or auto-approve mode, so nothing asks before you act. These rules replace the asking. The tools below live in `/Users/iampaulking/projects/duchess/athena-erp/.agents/bin/` (call them by that absolute path from any worktree).

## Who is who

| Role | Name | Tool | Started by | Reports to |
| --- | --- | --- | --- | --- |
| Stylus | `t6` | kiro-cli | the user | the user |
| Thinker | `th-<topic>` | Codex, read-only | Stylus | Stylus |
| Master | `master` | Muse YOLO | Stylus | Stylus |
| Worker | `w-<task>` | Muse YOLO | master | master |
| Reviewer | `r-<task>` | Muse YOLO (another session, never the worker) | master | master |
| Submaster | `sm-<task>` | kiro-cli `--trust-all-tools` or Codex bypass mode | the task's worker | that worker |
| Subworker | `sw-<task>-<n>` | Muse YOLO | the submaster | that submaster |

Talk only to your parent and your own children. A parent closes each child terminal as soon as the child's work is complete (`hc kill <name>`); nobody leaves finished terminals open. Stylus reasons and plans; master acts and merges; workers build; reviewers judge; submasters split a big task and integrate it.

## Permissions

Allowed:
- Reading and writing inside `/Users/iampaulking/projects/duchess/athena-erp` and the sibling worktrees `athena-erp-*`.
- Build, test, lint, generate, `pnpm` installs from the committed lockfile, throwaway Athena servers on free loopback ports, `test_athena_*` databases and throwaway Workspaces.
- Commits on your own branch. Only master merges into `main`, pushes (`git push origin main`, never force) and removes merged, clean worktrees.
- `hc` and `herdr-app` to read, message and (for your own children) start and close terminals.

Not allowed:
- Anything outside the folders above (home files, `~/.ssh`, `~/.aws`, other repos), except the Go module cache and pnpm store.
- `git reset --hard`, `git push --force`, `git clean -fdx`, `git branch -D` on unmerged work, `rm -rf` outside a worktree's `.cache`, `node_modules`, `dist` or `/tmp` scratch, dropping a database that is not `test_athena_*`, editing an applied system migration, deleting another agent's uncommitted work.
- The user's dev database (the one in `.env.local`), except through the user's own `make run` server.
- Network calls beyond lockfile installs and master's push. No new dependency unless Stylus agrees and it is logged.
- Deciding a section Z question in `tasks.md`. Skip that part and report it.
- ADRs are written **Proposed**; never mark a decision accepted.

## Talking

- Every message goes through `say <to> "KIND: text"`. KIND is UPDATE, PASS, BLOCKED, QUESTION, ANSWER, ORDER, ERROR, DONE or INFO. `say` records it in `.agents/stylus/comms.jsonl`; the dashboard shows the thread. Do not use raw `hc send` for agent messages.
- Workers and submasters: send an `UPDATE` to your parent at least every 10 minutes while working (what is done, what is next, any risk), and a `PASS`, `BLOCKED` or `QUESTION` when you stop. Then wait for instructions.
- Master: on every ticker ORDER run `progress "<text>"` (it records the update and sends it to Stylus).
- Questions go up one level. A parent answers within 15 minutes or escalates; if nothing comes back, take the safest choice that follows the docs, log it as "decided without answer", and continue.

## Heavy commands (one at a time, machine-wide)

The machine has 8 CPUs and 8 GB RAM. A full `make test`, a full `pnpm --dir web run test`, `make build` for a browser pass, and a Playwright or Chrome browser pass run only through `suite`:

```sh
/Users/iampaulking/projects/duchess/athena-erp/.agents/bin/suite --label "C1 slice 4" -- make test
```

It waits its turn in an ordered queue (master's merge gate first, then everyone in arrival order) and logs the result. If you stop a waiting `suite` with ctrl+c it leaves the queue cleanly. Two full suites in parallel time out; never bypass the lock.

Without the lock you may only run narrow checks: one package's tests filtered with `-run`, a single Vitest file, `go build`, `go vet`, typecheck. Any unfiltered `go test` of any package that touches MariaDB (`./internal/server`, `./internal/auth`, `./internal/posting`, `./internal/ledger`, `./internal/jobs`, `./internal/stock`, `./internal/events`, `./internal/party` and every new kernel package, plus `./cmd/athena`) goes through `affected` or `suite`, whatever folder it runs in. Rule of thumb: if it takes more than a minute, it goes through `suite`. (2026-11-24: an unlocked `./internal/stock` run turned a merge gate red.) Never copy the tree to `/tmp` to test outside the lock.

Reviewers review in their own clean worktree, because the worker keeps building the next slice in the task worktree: `git worktree add --detach ../athena-erp-<task>-r <reported commit>` (copy `.env.local`), review and run `affected` there, then `git worktree remove ../athena-erp-<task>-r` when the verdict is sent.

Scoped runs for workers and reviewers: `/Users/iampaulking/projects/duchess/athena-erp/.agents/bin/affected` runs, through the same lock, only the Go packages your change touches plus every package that imports them, and the web tests when `web/` changed. It falls back to the full `make test` for proto, `go.mod`, `Makefile`, shared field-rule testdata or a change that reaches most packages. `affected --print` shows the plan. Use it for slice reports and reviews. Master's merge-time gate after the rebase is always the full `make test`, so nothing reaches `main` without the whole suite.

One green run per commit: `suite` records the commit and whether the tree was clean. If the same command already passed on the same clean commit, it prints that run's log and exits 0 without rerunning. A reviewer whose worktree is at the worker's reported commit gets the worker's result this way; read that log instead of rerunning. Any change (a fix, a rebase) is a new commit and runs again. Master's merge-time run after the rebase always runs. `SUITE_FORCE=1` reruns when you suspect a flaky pass.

## Splitting a big task (workers)

If your task has more than about three independent slices, or one slice will take more than about 90 minutes, ask master: `say master "QUESTION: split <task>? <slices>"`. When master says yes:

1. Write `.agents/tasks/beta-<task>/split.md`: each sub-slice, the files it may touch (no two sub-slices share a file), its acceptance check, and its branch `feat/<task>-<sub>` cut from your branch with its own worktree `../athena-erp-<task>-<sub>`.
2. Start the submaster beside you:
   `spawn submaster sm-<task> --task <task> --lane <lane> --cwd ../athena-erp-<task> --cli kiro --place down --brief .agents/tasks/beta-<task>/split.md "Run the split in the brief."`
3. The submaster starts at most two subworkers (`spawn subworker sw-<task>-<n> ...`, each in its own worktree), reviews their diffs against `split.md`, merges them into the task branch in order, runs the checks, and reports `PASS` to you. You stay responsible for the task; master's reviewer still reviews the whole task branch.

`spawn` refuses when a cap in `.agents/stylus/limits.json`, free memory or free disk would be crossed. Do not work around a refusal; report it.

## Shared files and migrations

- Take the next migration number from `.agents/stylus/migrations.md`, where master reserves it for your task. Never pick one yourself.
- `internal/server/gate.go`, `web/src/api.ts`: add entries at the end of their list; master resolves rebase conflicts at merge. `web/src/App.vue`: one lane at a time, ask master. A new service gets its own `.proto` file.
- Rebase on `main` before reporting `PASS` and say if the rebase changed anything.

## Done means

The checks in `.kiro/specs/athena-beta/design.md` §5 pass (`go build ./...`, `go vet ./...`, `gofmt -l cmd internal` empty, `make test` through `suite`, `make generate` stable, web typecheck, test and build), docs match the change, the design system holds for Desk work, and your report lists anything you skipped and why. Restore `internal/webui/dist/.gitkeep` before committing.
