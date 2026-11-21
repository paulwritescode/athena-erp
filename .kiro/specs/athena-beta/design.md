# Design: Athena beta

**Status:** Draft for user approval, 2026-11-21. Implements `requirements.md` R1 to R14 through `tasks.md`. Kernel design for G6 to G18 is in `.kiro/specs/athena-batch-2/design.md`. Visual rules are in `docs/design/athena-design-system.md`. This file covers the architecture of the new pieces and the agent workflow.

## 1. Brand

The Athena mark is an olive tree whose trunk sweeps down the left side and curves into a shield, with roots at the base and leaves at the crown. It refers to Athena's olive and her aegis: growth that is also protection. It is a mark only, with no wordmark, drawn as a solid silhouette.

| File | Use |
| --- | --- |
| `athena-black-logo` (PNG 1254 x 1254, black on transparent) | light theme, print, favicon on light browser chrome |
| `athena-white-logo` (PNG 1254 x 1254, white on transparent) | dark theme |

Rules: the mark sits beside the word "Athena" (or the Workspace name) set in Satori. Minimum size 20 px. Clear space around it equals a quarter of its height. It is never recolored with the accent, rotated, outlined or placed on a photo. A vector SVG is traced from the PNG in task B5 so it stays sharp at every size; until then the PNGs are scaled down only.

The mark fits the design system well: one solid shape, no gradients, readable in one color. Its organic line also gives the hairline figures a reference: thin lines, natural curves, no fill.

## 2. Frontend architecture

```
web/src/
  theme.css            tokens and [data-accent] presets (B1)
  style.css            Tailwind @theme map, base layer (B1)
  typography.css       type tokens (existing)
  icons.ts             safe Lucide registry (B3)
  copy/glossary.ts     everyday words (B6)
  art/registry.ts      placement key -> figure or ASCII (B10)
  art/figures/         custom hairline figures (B10)
  pages/registry.ts    page key, label, icon, mode, permission, component (B2)
  components/ui/       shadcn-vue primitives (CLI only)
  components/          app compositions (PageHeader, DataTable, ...) (B2)
  session/             cross-tab channel, idle timer, step-up (B7)
  forms/               layout renderer and builder (B9)
```

- **Page registry.** `App.vue` keeps the shell and mode only. Navigation, routes and permission checks read the registry. A test compares it with `docs/design/pages-by-role.md` (R12.1).
- **Theme and preset.** `<html class="dark" data-accent="ink">`. The preset comes from the Workspace settings at sign-in and is cached per Workspace in `localStorage` so the first paint is correct.
- **Art registry.** Exposes `useArt(key)`. It returns nothing for a key not listed, so a page cannot add art on its own. Figures load lazily with dynamic `import()` so the Desk bundle carries only the figure in use.
- **Form layout.** The layout is JSON stored with the Record Type definition. `forms/render.ts` turns layout plus field definitions into sections. The builder edits the same JSON. Field rules still come from `fieldRules.ts`.

## 3. Server additions

| Area | Where | Notes |
| --- | --- | --- |
| Workspace identity | `internal/workspace`, new `WorkspaceService` in `proto/athena/v1/workspace.proto` | `GetIdentity`, `SetIdentity`; logo stored through `internal/files` under `brand/`; SVG sanitized; `GET /brand/logo-<hash>.<ext>` served by the Go server with long cache headers |
| Settings | `internal/ledger/settings.go` Settings chain (G5) | session lifetimes, rate limits, preset stored as Settings keys `security.session.*`, `security.rate.*`, `brand.accent` |
| Rate limits | new `internal/server/ratelimit.go` | `golang.org/x/time/rate` limiters keyed by IP, username and session, in a bounded LRU map with expiry; a ConnectRPC interceptor applies them before the gate; answers `ResourceExhausted` with `Retry-After` |
| Sessions | `internal/auth/service.go` | per-session idle and absolute expiry read from Settings at creation; ID rotation on privilege change and step-up; `step_up_at` column (new migration) |
| Root protection | `internal/auth`, `internal/server/auth_rpc.go`, `cmd/athena/access.go` | one guard function used by every path, with tests per path |
| Form layout | `internal/server/record_types.go`, `proto/athena/v1/record_types.proto` | `RecordType.layout` (new field number), validated server-side; Image field type stores a file reference |
| Public pages | new `internal/public` | Go `html/template` rendering from public projections; per-Workspace sitemap and robots; no Desk bundle on public pages beyond a small enhancement script |

Each row with durable data, a security change or a new public surface gets an ADR (numbers assigned at dispatch).

## 4. Session and tab design

- **Server is the authority.** It holds expiry and answers `Unauthenticated` with a reason (`idle`, `absolute`, `revoked`).
- **Tabs.** A `BroadcastChannel("athena-session")` carries `signed-out`, `workspace-changed`, `permissions-changed` and `activity` (throttled to once a minute). Every tab reacts to the first three. `activity` resets the idle warning in all tabs. Browsers without BroadcastChannel fall back to the `storage` event.
- **Idle warning.** Two minutes before expiry a dialog offers "Stay signed in" (one authenticated call) or "Sign out". Unsaved form drafts are kept in memory and restored after the user signs in again in the same tab.
- **Step-up.** Destructive actions in Build mode or as root call `AuthService.StepUp` with the password when the last step-up is older than 15 minutes.

## 5. Agent workflow (the instructions every agent follows)

### Roles

| Role | Agent | Owns |
| --- | --- | --- |
| Master | kiro-cli (Claude), terminal t6 | Plan, dispatch, merge, ticking `tasks.md`, shared files, talking to the user |
| Desk worker | Muse Spark t4 (`muse --yolo --model muse-spark-1.3-contributor --reasoning-effort max`) | Vue, CSS, web tests, screenshots |
| Backend worker | Codex (pane in the athena-erp tab); Muse Spark YOLO instances while Codex is over its limit (t1b B4 and B12 server, t1d B7 server) | Go, proto, `make generate`, CLI, migrations, ADRs, Go tests |
| Reviewer | the model that did not write the change; while Codex is over its usage limit (from 2026-11-21 12:50), a separate Muse Spark instance (t1f) with no access to the worker's session | Read-only review against the task, the requirements and the design system |

### One task, start to finish

1. **Dispatch.** The master creates a worktree `../athena-erp-<task>` on branch `feat/<task>-<slug>` from the latest `main`, copies `.env.local`, and sends a brief with: task id and text, requirement ids, files allowed, migration and ADR numbers, acceptance commands, and "do not commit, merge, push, start agents or touch other worktrees; report with `hc send t6`".
2. **Work.** The worker reads `AGENTS.md`, `.kiro/steering/athena-design.md`, the requirements and the design sections named in the brief. Desk work also follows the impeccable skill (Operate mode for the Desk) and the emilkowalski-motion skill.
3. **Report.** PASS or BLOCKED, files changed, exact command output, screenshot paths for UI.
4. **Review.** The master starts the reviewer with the brief saved in `.agents/tasks/<task>/review-brief.md`. The reviewer reports PASS or FAIL with file and line.
5. **Fix loop.** On FAIL the master sends the findings to the worker. At most three loops before the master asks the user.
6. **Merge.** On PASS the master reruns the checks itself in the worktree, commits with a Conventional Commit message, merges to `main` with `--ff-only` after rebasing, pushes, ticks the box in `tasks.md`, writes `.agents/tasks/<task>/summary.md`, and removes the worktree.

### Checks (the master runs these before any merge)

`go build ./...`, `go vet ./...`, `gofmt -l cmd internal` (empty), `make test` (it sets `ATHENA_TEST_DATABASE_URL`; a plain `go test ./...` skips MariaDB tests), `make generate` twice with no diff, `CI=true pnpm --dir web run typecheck`, `test` and `build`. UI tasks also need screenshots at 375, 768, 1280 and 1440 px in both themes. Restore `internal/webui/dist/.gitkeep` after a web build.

### Merge small and often (user direction, 2026-11-21)

- A task is cut into slices that each work end to end on their own (one screen, one flow, one server capability). Each slice merges to `main` as soon as it passes, without waiting for the rest of the task.
- One review per slice. Findings that break behavior, lose data, open a security hole or make a screen unusable block the merge; everything else is merged and recorded as a follow-up in `tasks.md`.
- The master merges within minutes of a PASS, restarts the user's server, and tells the user what to try.

### Parallelism

At most three workers at once. Tasks that edit the same shared file (`App.vue`, `gate.go`, `sysmigrations.go`, `api.ts`, a proto file) do not run at the same time. Docs edits are small appends in the task's own section.

## 6. Order and dependencies

```
B1 tokens ─► B2 shell+components ─► B3 type+icons ─► B4 presets
                     │
                     ├─► B5 identity+logo ─► B11 install+first run
                     ├─► B6 words+naming
                     ├─► B8 root+Build/View
                     ├─► B9 form builder
                     └─► B10 art
B7 sessions+rate limits (backend first, Desk after B2)
C1 G6 ─► C5 G7 ─► C6 G9 ─► C9 G14
C2 G8 ─► C6, C10
C3 G10 ─► D5 G17 ─► D6 kit ─► D7 G18
C4 G13, C7 G11 ─► C8 G12, D4 G16
D1 pages-by-role, D2 processes (docs, any time after B2)
D3 public pages after C3 and B5
E1 trial ─► E2 hardening ─► E3 release
```

Backend tasks (B7 backend, C1, C2, C3, C4) run in parallel with the Desk foundation (B1 to B4), because they touch different files.
