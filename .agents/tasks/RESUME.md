# Resume point (Stylus, 2026-11-22 07:50)

The day is clocked out after the first Stylus night shift. This file is the single place to start from next time. It replaces the 2026-11-21 restart notes.

## The Stylus workflow

- **Stylus** is the kiro-cli agent in the main repo pane (`t6` last time). It does the reasoning and planning and answers the master's questions.
- **Master** is a Muse YOLO agent (terminal name `master`). It follows `.agents/stylus/MASTER.md`: it briefs and starts workers (`w-<task>`) and reviewers (`r-<task>`) as split panes, merges after a review PASS and green checks, pushes `main`, closes finished panes, and keeps `.agents/stylus/night-log.md` and `night-report.md`.
- Permission policy (in MASTER.md): work only inside `athena-erp` and `athena-erp-*`; nothing destructive; no dev database; no new dependencies; no section Z decisions.

## The dashboard (saved in the repo)

- Files: `.agents/dashboard_web.py` (server and data), `.agents/dashboard.html` (the n8n-style map page), `.agents/dashboard.py` (terminal fallback view).
- Start it: `python3 .agents/dashboard_web.py` from the repo root, in its own pane, then open <http://127.0.0.1:8765>.
- It reads `tasks.md`, `hc ls` / `hc read`, git worktrees and `.agents/stylus/` live; nothing else to configure. Task dependencies are the `EDGES` list in `dashboard_web.py`.
- The 🌙 Night report button reads `.agents/stylus/shift.json` (shift start and `mainAtStart`), `night-log.md` and `night-report.md`. **For a new shift**, write a new `shift.json` with the current time and `git rev-parse --short main`, and start a fresh `night-log.md` (keep the old one as `night-log-2026-11-21.md` if you want it; it is in git history either way).

## State at clock-out

- `main` = `722e183`, pushed. The master closed the shift at 07:58 with 18 merges pushed; the last night-log lines give the final state. The user server was left running on `722e183` (pane `server`).
- Merged tonight (from `9260d04`): B9b, B4b, B2a (notifications full list skipped by decision), B5, B9 (slices 1 and 2 plus the 2.3 thumbnails follow-up), B10, B8, B11 slice 1, C1 slices 1 and 2, C2 slices 1 and 2. Full list and commits: the Night report button, or `.agents/stylus/night-report.md`.
- Ticked in tasks.md: A group except B4a; B group except B4a, B7 and B11.

### Open worktrees

| Worktree | Branch | State | Next step |
| --- | --- | --- | --- |
| `../athena-erp-c1` | `feat/c1-posting` | C1 slice 3 (`981ee78`, reversal mirrors, 6.6 and 6.7) **review PASS**, but the merge-time `make test` was **red**. It ran at the same time as the B11 suite, near the 20-minute budget, so the likely cause is contention; the branch was green at `5bbc94e` for worker and reviewer | Run `make test` alone; if green, merge. If red again, triage (the master suspects the `insertOutbox` unification) |
| `../athena-erp-b11` | `feat/b11-install` | B11 slice 2 (`be91324` wizard and readiness, `5fbf685` fix) **review PASS round 2**, merge-time `make test` **red**, same contention pattern | Run `make test` alone after C1; if green, merge and tick B11 |
| `../athena-erp-c2` | `feat/c2-events-jobs` | C2 slice 3 job runner committed (`aec5366`, includes slice-2 F1 and F2 fixes, **not reviewed**); the worker was told to stop and had not acknowledged at clock-out. Slices 4 to 6 not done (4 per the worker's plan, 5: invite and reset delivery through the outbox, 6: events.proto, handlers, CLI, pages, docs) | Check `git status` for leftover work, review `aec5366`, then continue |
| `../athena-erp-b9-server` | `feat/b9-form-layout-server` | Merged already | `git worktree remove` it (clean) |

### Muse sessions (resume with `muse --yolo --model muse-spark-1.3-contributor --reasoning-effort max resume <id>` in the worktree)

| Role | Session | Id | Worktree |
| --- | --- | --- | --- |
| master | hazel-gibbous | `01a1ffe6-cf48-7951-8caa-a24d1e0b9190` | `athena-erp` |
| w-b11 | cherry-zosma | `01a200bd-ea6f-7881-bf71-e91bafb8c728` | `athena-erp-b11` |
| w-c1 (likely; the earlier of the two) | timber-himalia | `01a200e7-f7b3-7d81-b4b3-27a1a6ce46f4` | `athena-erp-c1` |
| r-c1 (likely) | seaglass-flare | `01a200fb-8ae0-76a0-93d3-fc90aadfa771` | `athena-erp-c1` |
| w-c2 | ruby-circinus | `01a20106-0e4a-7aa1-936f-aaf8026ed14f` | `athena-erp-c2` |
| r-c2 | misty-vela | `01a20136-e0e9-7de1-bae4-e9b20bd911a2` | `athena-erp-c2` |

The master and the reviewers can also be started fresh: the master from `.agents/stylus/MASTER.md`, reviewers from the review briefs in `.agents/tasks/beta-<task>/`.

## How to resume

1. `brew services start mariadb@11.8` if MariaDB is not running; check `df -h /` (keep 8 GB free).
2. Open Herdr, start kiro-cli in `athena-erp`, and say: "Read `.agents/tasks/RESUME.md` and continue as Stylus."
3. Stylus starts the dashboard pane (`python3 .agents/dashboard_web.py`), the user server pane (`make run`, with node on `PATH`), and the master (resume `hazel-gibbous`, or fresh: `muse --yolo ... 'You are master for the Stylus shift. Read and follow .agents/stylus/MASTER.md.'`). Update MASTER.md's "Queue" section first so it starts from this file's state.
4. The master adopts or restarts the workers in the table above.

## Queue after resume

1. Rerun `make test` **one suite at a time** for C1 slice 3, then B11 slice 2; merge each that is green, triage any that is red. Tick B11 when done. Run full suites one at a time from now on (two in parallel timed out).
2. C2 slice 3 review, then slices 4 to 6. Secrets and lifetimes stay deferred on Z4.
3. C1 remaining sub-tasks (6.8 to 6.12) that do not need Z2.
4. C4 (trees, effective dating, UoM), then C3 (Pack packaging, no licence text while Z5 is open).
5. B4a steering sync (docs only).
6. D1 and D2 as Proposed drafts for the user.

## Questions waiting for the user

- **B7:** "a Workspace switch clears caches" has no trigger because the Desk has no Workspace switch. Recommended: descope the bullet until a switch flow is designed and track it under B8 or B11. B7 stays unticked until decided (verify report: `.agents/tasks/beta-b7/verify-report.md`).
- **B10:** custom Athena figures need the `hairline-create` skill, which is not installed. Recommended: install it and let a worker make the custom set, or accept the stock figures.
- **B2a follow-up:** a Notifications full-list page needs a route, nav entry and permission designed before it is built.
- **Section Z** (`tasks.md`): Z1 to Z7, Z10, Z11 still open. Z2 blocks parts of C1, Z3 blocks C7, Z4 blocks C2 secrets, Z5 blocks C3 licence text.

## Loose ends

- `athena-black-logo` and `athena-white-logo` at the repo root are untracked originals; B5 copied them into `web/src/assets/brand/`. Leave or delete them as you prefer.
- `internal/webui/dist/.gitkeep` gets deleted by web builds; restore it with `git checkout internal/webui/dist/.gitkeep` before committing.
- Stale `test_athena_*` databases from killed runs exist in MariaDB; they are harmless.
