# Orchestration plan: finish athena-batch-2 (G5 checkpoint to G18)

**Status:** DRAFT, waiting for user approval. Written 2026-11-21 by the master (kiro-cli, terminal t6).
**Superseded as the task order by** `.kiro/specs/athena-beta/tasks.md` (2026-11-21). Sections 3, 5, 6 and 8 (roles, frontend constraints, conflict control, worker brief) still apply; section 5 is now backed by `docs/design/athena-design-system.md`.
**Spec:** `.kiro/specs/athena-batch-2/{requirements,design,tasks}.md`. Prior phase records: `.agents/tasks/phase-*/{summary,notes,review}`.

## 1. Benchmark (measured 2026-11-21 on `main`, HEAD aeb7694)

| Check | Result |
| --- | --- |
| `go build ./...`, `go vet ./...` | pass |
| `go test ./...` (with `.env.local`, so MariaDB tests ran) | all packages ok |
| `pnpm typecheck`, `pnpm test` | pass, 141 Vitest tests in 5 files |
| tasks.md sub-tasks | 41 done, 122 open (of 163) |
| Groups | G1 to G4 done. G5 done except checkpoint 5.10. G6 to G18 not started. |
| Git | 64 tracked files modified (+8463/-877) plus about 35 untracked files and dirs. Nothing from G1 to G5 is committed. Last commit is `aeb7694 docs(changelog): P4 to P9`. |
| Not yet verified | `make generate` leaves no diff, `pnpm build`, browser pass in both themes (these gate 5.10). |

**In flight (2026-11-21 00:40):** Muse Spark in terminal t4 (YOLO mode, main checkout) is already running checkpoint 5.10: full test suite, a Playwright browser pass of the Ledger setup page on a throwaway server (port 3100), and small fixes (UTC date handling in `internal/server/access_rpc_test.go` and `assignments_test.go`, a button label in `LedgerSetupPage.vue`). It owns the working tree until it reports done. No other worker edits or builds in the main checkout until then. Wave 0 item 2 below is therefore t4's job, not a new task.

Risk: all of G1 to G5 exists only as uncommitted changes. Worktrees branch from commits, so this must be committed first.

## 2. What remains

| Group | Sub-tasks | Depends on | Migration | ADR (corrected) |
| --- | --- | --- | --- | --- |
| G5 checkpoint | 1 | none | 13 | 0022 (done) |
| G6 Posting engine | 12 | G5 | 14 | 0023 |
| G7 Party ledger, settlements, FX | 9 | G6 | 15 | 0024 |
| G8 Jobs, outbox, events, workflows, secrets, lifetimes | 14 | G5 | 16 | 0025, 0026 |
| G9 Balances, Query compiler, Reports v1 | 10 | G6, G8 | 17 | 0027 |
| G10 Pack packaging | 7 | G5 | 18 | 0028 |
| G11 Stock ledger | 11 | G6 | 19 | 0029 |
| G12 Remaining ledgers | 7 | G6 | 20 | 0030 |
| G13 Trees, effective dating, UoM | 7 | G5 | 21 | 0031 |
| G14 Report engine | 9 | G8, G9 | 22 | 0032 |
| G15 Integrations | 7 | G8 | 23 | 0033 |
| G16 PWA, offline count | 9 | G11, G15 | 24 | 0034 |
| G17 Business starter Packs | 8 | G7, G9, G10, G11 | 25 | 0035 |
| G18 Industry and channel Packs | 11 | G17 | none | 0036 |

Dependencies are my reading of `design.md` section "Group map"; the master confirms each against the code before dispatch.

**Numbering fix.** tasks.md still says G6 uses ADR-0022 and later groups follow. ADR-0022 is already taken by the ledger foundation, so every later ADR number shifts by one (column above). Migration numbers 14 to 25 are unchanged. Workers must use the numbers in this table, not the ones in tasks.md.

**Sponsor decisions needed before the group starts** (design.md "Open questions"): permitted backdating window (G6, G11), stock valuation method and negative stock (G11), retention periods and key custody (G8), licence (G10), pilot company and jurisdiction (G17), `vite-plugin-pwa` (G16), pack build order and pilot country (G18). A worker must not invent these; it implements them as OPEN settings and reports.

## 3. Roles

| Role | Agent | Does | Never does |
| --- | --- | --- | --- |
| Master | kiro-cli (Claude), t6 | Dispatches, reads reports, merges after PASS, updates tasks.md, keeps shared files consistent | Writes feature code, merges without user approval |
| Backend worker | Codex (`codex-worker`) | Go, proto, `make generate`, CLI, ADR, tests for one group | Edits `web/src` pages |
| Desk worker | Muse Spark (`muse-worker`) | Vue pages and components for one group, after its proto is merged | Edits Go or proto |
| Reviewer | the other model (Codex reviews Muse, Muse reviews Codex) | Read-only diff review against the task, runs the checks, PASS or FAIL | Edits files |

Each group runs as: **A** backend branch, review, merge; then **B** Desk branch (needs A's generated TypeScript), review, merge; then the checkpoint (master). Where a group has no Desk task (G12 mostly, G18) only A runs.

## 4. Wave 0 (before any worker starts)

1. Commit the current work in logical commits on a branch (for example `feat/batch-2-g1-g5`), with Conventional Commit messages by group. User approves the branch and message plan first. `.agents/` and `.kiro/` are untracked: decide whether to commit them (recommended: yes, they are the project record).
2. Close checkpoint 5.10: run `make generate` twice, `pnpm build`, and a browser pass of the setup pages in both themes.
3. **Interface ready.** App.vue is a 348-line file with one long inline template, and every group edits it, so it is the main merge conflict. Before dispatch, one Desk worker task:
   - Replace the hard-coded nav and page switch with a single page registry (key, label, required permission, component). Adding a page becomes one registry line.
   - Add stub pages with an empty state for the pages the spec creates: `ReportsPage`, `JobsPage`, `SettingsPage`, `WorkQueuePage`, `CountPage`, `SetupPage`. Stubs are hidden unless the user holds the permission.
   - No behavior change for existing pages. Vitest and typecheck stay green.
4. Reserve shared-file ownership (section 6).

## 5. Frontend constraints (given to every Desk worker verbatim)

From AGENTS.md, README and `docs/architecture/frontend-theme-and-type.md`:

- Vue 3 + TypeScript + Vite. Package manager is pnpm 11.22.0 only, committed lockfile. No npm or Yarn.
- Components come from shadcn-vue (Reka Luma, Zinc). Existing set in `web/src/components/ui`: alert-dialog, badge, button, card, checkbox, command, dialog, dropdown-menu, input, input-group, label, marker, popover, select, separator, sonner, switch, table, tabs, textarea. Reuse these. Add a new shadcn-vue component only when none fits, and say so in the report.
- Colors only from semantic tokens in `web/src/theme.css`. No hex, rgb or named colors in components. Type only from `web/src/typography.css` (display-sm for page headings, title-md/sm for sections, body-sm for text, caption for dense metadata).
- Both themes must work. The class-based light/dark toggle is kept. Pointer cursor on clickable elements. Do not set `color: inherit` on buttons.
- Build and View modes stay separate. Build nav: Overview, Record Types, Packs, Rules, Files, Access, Ledger, shown by permission. View lists saved Record Types only and has no Build navigation.
- Action results (saved, deleted, conflict, blocked post, failure) are Sonner toasts, bottom right. Durable state (queued jobs, delivery, readiness) is shown on a page, not in a banner.
- Calls go through `web/src/api.ts` and the generated `web/src/gen/` ConnectRPC client. No `fetch` to ad hoc JSON endpoints.
- Any rule that mirrors the server stays in a pure `.ts` file with Vitest tests next to it (`fieldRules.ts` is the model). Changing a field rule means changing `internal/server/records.go`, `web/src/fieldRules.ts` and `internal/server/testdata/field_rules.json` together.
- Every page needs loading, empty, error and permission-denied states. Forms disable submit while saving and show the server's reason on refusal. Dialogs trap focus and close on Esc. Interactive elements are reachable by keyboard and have accessible names.
- Never render, log or trace passwords, Challenge values, session or CSRF tokens, secret values. Telemetry spans carry ids, counts, durations and outcome codes only.
- Old vocabulary is forbidden in source (`terms_test.go`): Record Type, Record, Pack, Workspace, Rule (Starlark only), Grant. Do not rename legacy CSS classes or the `athena-entry-columns:` storage key.
- Done means: `CI=true pnpm --dir web run typecheck`, `test` and `build` pass, and the page was opened in a browser in both themes. The worker states what it saw. The embedded build in `internal/webui/dist` is rebuilt.

## 6. Conflict control

Files every group touches: `internal/server/sysmigrations.go`, `internal/server/gate.go`, `internal/server/server.go`, `proto/athena/v1/ledger.proto`, `gen/**` and `web/src/gen/**` (generated), `web/src/api.ts`, `web/src/types.ts`, `docs/architecture/overview.md`, `docs/decisions/decision-log.md`, `docs/README.md`, `README.md`, `AGENTS.md`, `CHANGELOG.md`, `.kiro/specs/athena-batch-2/tasks.md`.

Rules:
- Workers do not edit `tasks.md` (the master ticks boxes after PASS).
- Generated files are never merged by hand: after a merge the master runs `make generate` and commits the result.
- Docs edits (overview, decision log, README, AGENTS) are small appends in the worker's own section. The master resolves conflicts when merging.
- Migration number and ADR number come from the table in section 2. Two groups never share one.
- One worktree and one branch per task: `feat/g6-posting-backend`, `feat/g6-posting-desk`. Merge order follows dependencies. Parallel groups merge one at a time, each rebased on the latest `main` and re-tested.

## 7. Waves

| Wave | Parallel work | Gate |
| --- | --- | --- |
| 0 | Commit baseline, close 5.10, registry and stubs (Muse) | User approval, green checks |
| 1 | G6 backend (Codex), G8 backend (Codex or a second Codex), G10 backend (Codex), G13 backend | Each: review PASS, merge, checks green |
| 2 | G6 Desk, G7 backend, G8 Desk, G10 Desk, G11 backend, G12 backend | Backdating decision before G6 submit hooks |
| 3 | G7 Desk, G9, G11 Desk, G14, G15 | Valuation decision before G11 |
| 4 | G16, G17 | Pilot company and jurisdiction decision |
| 5 | G18 | Pack build order decision |

Concurrency cap: three workers at a time. More than that makes the merge and review load exceed what the master can verify.

## 8. Worker brief (every task prompt contains)

1. Task text copied from tasks.md, plus the requirement ids and the design section.
2. Worktree path and branch. File list the worker may edit. Anything else: stop and report.
3. Migration and ADR numbers from section 2.
4. Acceptance commands (exact), from the group checkpoint.
5. For Desk workers: section 5 verbatim.
6. "Do not start agents, merge, push or touch other worktrees. Report with `hc send t6` when done or stuck: PASS or BLOCKED, files changed, command output."

## 9. Verification before a group is called done

Master runs, from the repository root on the merged `main`: `make generate` (second run no diff), `go build ./...`, `go vet ./...`, `make test`, `CI=true pnpm --dir web run typecheck`, `test`, `build`. A group's checkbox is ticked only with that output in hand. Reviewer verdicts are saved in `.agents/tasks/<group>/review.md` and `review.json`, as in earlier phases.

## 10. Decisions requested from the user

1. Approve committing G1 to G5 on a branch, and whether `.agents/` and `.kiro/` are committed.
2. Approve the split: Codex backend, Muse Spark Desk, cross-model review.
3. Approve Wave 0 item 3 (page registry and stubs).
4. Confirm how Muse Spark is launched so the `muse-worker` template is right (model flag, any sandbox flag).
5. Answers to the sponsor questions in section 2, or approval to proceed with each as OPEN.
