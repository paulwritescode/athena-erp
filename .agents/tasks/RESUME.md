# Resume point (Stylus, 2026-11-23 02:25)

This file is the single place to start from next time. It replaces the 2026-11-22 07:50 notes (they are in git history). Say to kiro-cli in `athena-erp`: **"Read `.agents/tasks/RESUME.md` and continue as Stylus."**

## The workflow (Stylus v2)

```
Stylus (kiro-cli, the main repo pane) ── reasons, plans, decides; may start read-only Codex thinkers th-*
└── master (Muse YOLO) ── five lanes, reviews, merge trains, the record, 10-minute updates
    ├── w-<task> (Muse YOLO) ── one worker per lane, at most 5
    │   └── sm-<task> (kiro-cli --trust-all-tools or Codex bypass) ── only for a big task master approves
    │       └── sw-<task>-<n> (Muse YOLO) ── at most 2 per submaster
    └── r-<task> (Muse YOLO, fresh session, own -r worktree) ── at most 3
```

Binding documents (read these first, every agent reads AGENT_RULES):
- `.agents/stylus/AGENT_RULES.md`: permissions, `say`, the suite lock, scoped tests, splitting, review worktrees, closing terminals.
- `.agents/stylus/MASTER.md`: master's brief (lanes, one task start to finish, merge trains, the record, health). Its "Queue now" section must be updated before restarting master (see Queue below).
- `.agents/stylus/lanes.json` (five lanes), `limits.json` (caps: 12 agents, 5 workers, 3 reviewers, 2 submasters, 15 % free memory, 8 GB free disk), `migrations.md` (next free number: **20**).

Tools in `.agents/bin/` (call by absolute path from any worktree):

| Tool | What it does |
| --- | --- |
| `spawn <role> <name> --task T --lane L --cwd ../athena-erp-x --brief f "msg"` | starts an agent as a split pane beside the caller (never a new tab), checks caps, memory and disk, records it in `agents.jsonl` |
| `say <to> "KIND: text"` | every message between agents; recorded in `comms.jsonl` |
| `suite --label L -- make test` | heavy runs one at a time machine-wide; ordered queue with master first; reuses a green run on the same clean commit; node fallback to nvm |
| `affected` | scoped tests for workers and reviewers (changed Go packages plus importers, web tests if `web/` changed; full suite for proto, go.mod, Makefile); refuses a worker whose branch lacks code from main |
| `progress "text"` | master's update to Stylus; `progress.jsonl` |
| `ticker` | every 10 minutes asks master for its update and lists close candidates |

## Restart, step by step (in Herdr, space `athena-erp`)

1. **Database and disk.** `brew services start mariadb@11.8` (it is left running); `df -h /` must show more than 8 GB free (12.7 GB at clock-out).
2. **Stylus.** Tab `1`: start kiro-cli in `/Users/iampaulking/projects/duchess/athena-erp` and say the line at the top of this file. Stylus's terminal address goes in `limits.json` `"stylus"` (it was `t6`; change it if Herdr gives another address, or messages to Stylus will not arrive).
3. **Dashboard.** Tab `dashboard`, left pane: `python3 .agents/dashboard_web.py`, then open <http://127.0.0.1:8765>. It may still be running from tonight; check the page first. Operations panel: Team (agent tree with Kiro, Muse and Codex logos), Lanes, Messages, Updates, Suite; `?ops=lanes` opens a tab directly.
4. **Ticker.** Split under the dashboard pane:
   ```sh
   hc run --bg --outlive-parent --name ticker --cwd "$PWD" -- python3 .agents/bin/ticker
   herdr-app do pane.focus --terminal <dashboard address>
   herdr-app do terminal.keep --terminal <ticker address> --place down
   ```
5. **Fresh shift record.** Keep the old files in git: `git mv .agents/stylus/night-log.md .agents/stylus/night-log-2026-11-22.md` (and the same for `comms.jsonl`, `progress.jsonl`, `suite.jsonl`, `agents.jsonl` if you want them archived; empty files are fine), then write `shift.json` with the current time and `git rev-parse --short main` (format: `{"start": "...", "startEpoch": N, "mainAtStart": "..."}`), and start `night-log.md` with `- HH:MM STARTED shift 3`.
6. **Update MASTER.md "Queue now"** from the Queue section below.
7. **Master.** Resume tonight's session, or start fresh (fresh is fine; it rebuilds context from the files):
   ```sh
   hc run --bg --outlive-parent --name master --cwd /Users/iampaulking/projects/duchess/athena-erp -- \
     muse --yolo --model muse-spark-1.3-contributor --reasoning-effort max resume 01a204e4-6ad1-7e90-a54b-92d6d8dec1b3
   # fresh instead: ... --reasoning-effort max 'You are master for Stylus shift 3. Read .agents/tasks/RESUME.md, then .agents/stylus/MASTER.md and follow it, starting with its Queue now. Agents are split panes in tab 1, never new tabs. Log to .agents/stylus/night-log.md.'
   herdr-app do pane.focus --terminal <stylus address>
   herdr-app do terminal.keep --terminal <master address> --place right
   ```
   Note: a message sent with `hc send` into a plain shell that then starts muse may not submit; starting muse with the first message as its argument (as above) works.
8. **Workers.** Master restarts them with `spawn` (split panes in tab 1), resuming their sessions where the table below gives one: first message `muse ... resume <id>` is not possible through spawn, so either resume by hand in the worktree (`muse --yolo --model muse-spark-1.3-contributor --reasoning-effort max resume <id>`, then `herdr-app do terminal.keep`) or spawn fresh with "Read and follow <brief>; you are continuing: run git status and git log main.. first". Fresh is simpler and safe because every tree is clean and all work is committed.
9. **User server (optional).** It was not running at clock-out. `make run` in the main repo (node must be on `PATH`: `export PATH=$HOME/.nvm/versions/node/v26.7.0/bin:$PATH`).

## State at clock-out (2026-11-23 02:22)

- `main` = `da0ca2f`, pushed (`origin/main` equal). The main checkout has only the shift record files modified (night log, report, `.jsonl` records); they are committed with this file.
- Merged this shift (from `1e25ab9`): C1 slice 3, B11 slice 2 (**B11 complete, ticked**), B4a steering sync (ticked), D3 slice 1 (ADR-0030 Proposed), C4 slice 1 (migration 18, ADR-0029 Proposed), C1 6.8 (posting Rule type and activation gate), C1 6.9 (preview, status, log RPCs and post CLI), plus all Stylus tooling.
- No suite running, the suite lock is free; one dead queue ticket was removed.
- All agents stopped at safe points: every worktree is clean and all work is committed on its branch. Panes closed by Stylus after the report.

### Lanes and open worktrees

| Lane | Worktree / branch | Head | State | Next step |
| --- | --- | --- | --- | --- |
| L1 Books and posting | `athena-erp-c1` / `feat/c1-hooks` | `edf94a1` | 6.11 docs committed (1 commit ahead, 3 behind main) | rebase, `affected`, web build via `suite`, review, merge. 6.10 (Desk voucher view) waits for a GO; then C1 6.12, then C5 |
| L2 Jobs and events | `athena-erp-c2` / `feat/c2-events-jobs` | `e2c2bf2` | slice 3 plus fix for review B1 and F1, F2, F4, F5, and F3 as migration 19 | rebase (4 behind), re-gate with `affected`, FIX PASS, `r-c2` round 2, merge; then slices 4 to 6 (runner hardening, invite and reset delivery through the outbox, events.proto, handlers, CLI, Jobs and Settings pages) |
| L3 Masters and Packs | `athena-erp-c4` / `feat/c4-trees` | `2d921c3` | slice 2 (13.2) code complete, narrow checks green | rebase (4 behind), `affected` (full: protos), PASS, review, merge; slice 3 (UoM and determination ranges; may touch accounts and masters); then C3 |
| L4 Desk, first run, access | `athena-erp-d3` / `feat/d3-public` | `6f2ad1b` | slice 2 review PASS (`10ce8cb`), slice 3 committed | merge slice 2 (merge train), re-gate and re-report slice 3, review; B7 waits on the user |
| L5 Quality | `athena-erp-fu` / `feat/fu-batch-1` | `faa093d` | 12 follow-up fixes (B9a, B4, B4b, B9b), typecheck, touched Vitest, build and `affected` green | rebase (21 behind), browser pass through `suite`, report, review, merge; then more follow-ups, D1 and D2 drafts |

### Muse sessions (for `muse ... resume <id>` in the worktree)

| Role | Name | Session id | Worktree |
| --- | --- | --- | --- |
| master | green-aegaeon | `01a204e4-6ad1-7e90-a54b-92d6d8dec1b3` | `athena-erp` |
| w-c1 | plum-megrez | `01a204ff-8c15-7b63-93f1-ec9c09f38ebd` | `athena-erp-c1` |
| w-c2 | agate-corona | `01a2051f-8a08-7540-808c-e198e39ce306` | `athena-erp-c2` |
| r-c2 | cloud-circinus | `01a204fe-b576-71c1-b48b-3a08c107f3c0` | `athena-erp-c2` (move to `athena-erp-c2-r` for round 2) |
| w-c4 | horizon-setebos | `01a204ff-02c6-7ff3-8550-580876c03f4a` | `athena-erp-c4` |
| w-d3 | lagoon-merak | `01a20505-baf0-7823-be3d-b44ba939745a` | `athena-erp-d3` |
| w-fu | orchid-singularity | `01a204ff-0ca3-7522-92a4-9cfeff3c7aa7` | `athena-erp-fu` |

## Queue after resume (copy into MASTER.md "Queue now")

1. Merge train: D3 slice 2 and C2 slice 3 plus fix (after its FIX PASS and `r-c2` round 2) on current main, one full gate on the stack tip.
2. C1 6.11 gate and review; C4 slice 2 gate and review; FU batch 1 browser pass and review. Every branch rebases first (`affected` refuses otherwise).
3. Then per lane: C1 6.12 (and 6.10 once GO), C5; C2 slices 4 to 6, C10; C4 slice 3, C3; D3 slice 3 then D3 complete; more follow-ups, D1 and D2 Proposed drafts, Bruno coverage.

## Questions waiting for the user

- **B7:** "a Workspace switch clears caches" has no trigger (no Workspace switch in the Desk). Recommended: descope until a switch flow is designed.
- **B10:** custom Athena figures need the `hairline-create` skill (not installed). Install it, or accept the stock figures.
- **Notifications full-list page:** route, nav entry and permission need design first.
- **Z13 (new):** segregation of duties for posting Rule activation (the author may approve today; audit rows record both). Recommended: a Workspace setting, on by default when two or more users hold the approve permission.
- **C1 6.10:** GO GIVEN 2026-11-24 — queued in L1 after C5 slice 1 merges (worker cap 5/5 refused a parallel w-c1; worktree `../athena-erp-c1-610`, branch `feat/c1-voucher`, brief `.agents/tasks/beta-c1-hooks/brief.md` slice 3).
- **Second suite slot (offered, not decided):** allow scoped runs in parallel with full suites to cut queue waits (30 to 50 minutes tonight). Risk: contention timeouts; would be monitored.
- Section Z still open: Z1 to Z7, Z10, Z11, Z13 (Z2 blocks parts of C1, Z3 C7, Z4 C2 secrets, Z5 C3 licence text).

## Lessons from tonight (already built into the tools and rules)

- The suite queue is the bottleneck (10 to 13 minutes per full run): scoped `affected` runs, one green run per commit, master's gate first, merge trains, rebase before any gate.
- Never merge while another merge gate runs; never move main during a gate (docs commits wait for the train).
- Read-only tools must not take `index.lock` (dashboard and tools use `GIT_OPTIONAL_LOCKS=0`).
- Reviewers use their own `<task>-r` detached worktree; no test copies in `/tmp`; heavy packages only through the lock.
- Close a terminal as soon as its work is done.

## Loose ends

- `posts.md` at the repo root (LinkedIn and X drafts) is untracked on purpose; publish or delete it.
- `athena-black-logo` and `athena-white-logo` at the repo root are untracked originals (copies live in `web/src/assets/brand/`).
- Empty folders `../athena-erp-b3-experience`, `-b4b12-server`, `-b6-words` (0 bytes, not worktrees) can be deleted.
- Unnamed shells `t3g` (tab 3) and `t3v` (tab 2) were not started by Stylus; left alone.
- `internal/webui/dist/.gitkeep` is deleted by web builds; restore with `git checkout internal/webui/dist/.gitkeep` before committing.
