# Resume after restart (written by the master, 2026-11-21 23:05)

The laptop is being restarted. Herdr does not restart agents after a reboot. This file gives every task, every worktree and the Muse session to resume, so work continues from the same point.

## State at restart

- **main** (pushed to origin): `ec98c5f`. Everything listed under "Merged" in `.kiro/specs/athena-beta/tasks.md` is on main and was live on the user's server.
- **Disk is 98 % full (5.2 GB free).** Herdr's core stopped writing its registry ("cannot save terminals: registry writer stopped"), which is why the master could no longer message workers. Free space before restarting work: each worktree has its own `web/node_modules` and `.cache` (about 350 MB to 2 GB each); `athena-erp/.cache` is 2.1 GB and safe to delete (it is rebuilt by `make web-deps`).
- Uncommitted work sits on disk in each worktree and survives the reboot. Nothing below is lost by restarting.

## Worktrees and tasks

| Worktree | Branch | Task and brief | State | Muse session to resume |
| --- | --- | --- | --- | --- |
| `../athena-erp-b9-server` | `feat/b9-form-layout-server` | B9 slice 1, server: form layout and Image field. Brief `.agents/tasks/beta-b9/server-brief.md` | **Reviewed PASS and committed on its branch (`3be9e76`)**, not merged. The merge check is blocked by `cmd/athena` `TestFilesCommands` and `TestFilesPutImageCommands` failing with `database "test_athena_files_cli_002" already exists`: a fixed database name collides with a leftover from a parallel run. Next: give `cmd/athena` tests random Workspace names (or drop that leftover `test_athena_files_cli_002`), rerun `make test`, then merge. | none needed (master merges) |
| `../athena-erp-b9b` | `feat/b9b-new-type` | B9b: name-first New Record Type flow. Brief `.agents/tasks/beta-b9/b9b-brief.md` | **Worker reported PASS**, not yet reviewed or merged (8 changed files) | worker t1b: `basalt-dorado` `01a1fd86-a5c6-7783-8c18-13151ba1dcb6` (it was started in the removed `athena-erp-b4b12-server`; resume it from `athena-erp-b9b`) |
| `../athena-erp-b2a` | `feat/b2a-tables` | B2a: table system. Brief `.agents/tasks/beta-b2a/brief.md` | In progress, slice 1 (DataTable rewrite plus 5 new files, 15 changed files) | worker t1t: `scarlet-alnitak` `01a1ff9b-2140-7d51-b0d4-dd9ce99a5a27` |
| `../athena-erp-b5` | `feat/b5-identity` | B5: Workspace identity and logo. Brief `.agents/tasks/beta-b5/brief.md` | In progress, slice 1 server (11 changed files) | worker t1q: `gravel-deneb` `01a1ff8f-5b36-7bd0-a384-ffb62fba723a` (started in the removed `athena-erp-b6-words`; resume it from `athena-erp-b5`) |
| (none yet) | | B4b: Appearance page redesign (design-system §18 revised) | Not started; assigned to Muse t4 | worker t4: `bare-zodiacal` `01a1f8c5-d4f9-75e0-bba4-0bc0e3e033c7` |

Reviewers had finished their reviews and need no resume: `weathered-centaurus` (B9 server review) and `hollow-heliopause` (sessions review). The `iron-indus` session belongs to the user's `lighthouse-playground` project, not Athena.

## How to resume

1. Free disk space (see above), then open Herdr.
2. Start the master again in the `athena-erp` space (kiro-cli in `/Users/iampaulking/projects/duchess/athena-erp`) and tell it: "Read `.agents/tasks/RESUME.md` and continue as master."
3. The master starts each worker in its own pane, in its worktree:
   `cd <worktree> && muse --yolo --model muse-spark-1.3-contributor --reasoning-effort max resume <session id>`.
   If a session cannot resume, it starts a fresh Muse with "Read and follow <brief>; you are continuing: run git status and git diff first, keep what matches the brief, then finish."
4. Restart the status tick (every 10 minutes) and the user's server (`make run` in `athena-erp`).

## Order after resume

1. Merge B9 server once its test-name collision is fixed.
2. Review B9b, then merge it (it rebases after B9 server only if files overlap; they should not).
3. Continue B2a slice 1 and B5 slice 1; review and merge each slice as it passes.
4. Start B4b (Appearance redesign) on t4.
5. Then B9 slice 2 (form builder Desk), B10 (art), B8 (root and Build/View), B11 (install flow), and the C, D and E groups in `tasks.md` order.

## Rules that still apply

`.kiro/specs/athena-beta/design.md` §5 (agent workflow; merge small and often), the design system in `docs/design/athena-design-system.md` (§1 to §21), `AGENTS.md`, and `.kiro/steering/athena-design.md`. Workers use throwaway Workspaces only, never the dev database in `.env.local`.
