# Athena beta: unified task list

**Status:** Active, 2026-11-21. Written by the master (t6). Read with `requirements.md` (R1 to R14) and `design.md` (architecture and the agent workflow in section 5) in this folder.
**Replaces as the working plan:** the open tasks of `.kiro/specs/athena-batch-2/tasks.md` (G6.2 to G18) and `.agents/tasks/erp-kernel-roadmap.md`. Those files stay as the source text for each group; this list sets the order, adds the design, product and security work, and is the only checklist agents tick.
**Sources merged:** `AGENTS.md`, `README.md`, `docs/architecture/*`, `docs/decisions/*` (ADR-0001 to 0023), `docs/revamp/*` (playbook, process catalogue, industry, checkout and learning, records and movements, research), `.kiro/specs/athena-batch-2/*`, `.agents/tasks/*`, `docs/design/athena-design-system.md`, `.agents/tasks/orchestration-plan.md`.
**Beta definition:** a person installs Athena, creates a Workspace from the CLI, signs in as the root administrator, sets up a company with a logo, installs the Business starter Packs, invites a storekeeper and a cashier, and they buy, sell, count stock and close a month through screens a new user can use without help. Public storefront pages are indexable. Every claim is backed by a passing check.

## Conventions (apply to every task)

- Agents: Codex for Go, proto and CLI; Muse Spark (`--yolo`, `muse-spark-1.3-contributor`, effort max) for the Desk; the other model reviews. Master assigns, merges after PASS and ticks boxes. One worktree per task.
- Frontend tasks follow `docs/design/athena-design-system.md`, `.kiro/steering/athena-design.md`, and the impeccable and emilkowalski-motion skills.
- Backend conventions from `.kiro/specs/athena-batch-2/tasks.md` "Conventions" still apply (policies in `gate.go`, `make generate`, CLI per capability, numbered migrations, Proposed ADRs, property tests, spans without values).
- Migration and ADR numbers: next free migration is **15**, next free ADR is **0024**. They are assigned by the master at dispatch, in merge order, and written into the task. The numbers printed in batch-2 tasks.md for G7 onward are stale.
- Checkpoint for every task: `go build ./...`, `go vet ./...`, `make test` against MariaDB, `make generate` twice with no diff, `CI=true pnpm --dir web run typecheck`, `test`, `build`; for UI work a browser pass at 375, 768, 1280, 1440 px in both themes with screenshots in `.cache/screens/`.
- OPEN decisions are implemented as settings or left out, never invented. Each is listed in section Z.

## A. Foundation (design system first; nothing else in the Desk starts before A is merged)

- [x] **B1. Design tokens** (Muse) — R1. Merged 2026-11-21 after three review loops; three token-guard edge cases recorded as known limits (user chose option A) and moved to B2.
  - Add the *Proposed* tokens from design-system §2.4, §2.5, §3, §6 to `web/src/theme.css` and expose them in the `@theme` map in `web/src/style.css`. Keep every existing color value.
  - Set dark `--sidebar-primary` to `var(--primary)` (§2.2).
  - Theme browser surfaces: selection, caret, scrollbars, focus ring (impeccable craft floor).
  - Add `web/src/tokens.test.ts` that fails if a `.vue` or `.css` file under `web/src` outside `theme.css` contains a hex, `rgb(` or named color.
  - Accept: tokens resolve in both themes; test green; no visual regression on existing pages.
- [x] **B2. Shell and component library** (Muse) — R2. Merged 2026-11-21 after four review loops (user approved the fourth, blockers only); follow-ups moved to B3.
  - Add missing shadcn-vue components with the CLI (design-system §7 list).
  - Rebuild the shell: header, sidebar (desktop), rail (tablet), bottom nav and More sheet (mobile), user menu, command palette stub, Build/View mode switch as a segmented control.
  - Replace the hard-coded nav and page switch in `web/src/App.vue` with a page registry (`web/src/pages/registry.ts`: key, label, icon, mode, permission, component). Stub pages with empty states for Reports, Jobs, Settings, Work queue, Count, Setup.
  - Build shared compositions in `web/src/components/`: PageHeader, EmptyState, ErrorState, PermissionDenied, LoadingSkeleton, ConfirmDialog, FormSection, FieldRow, DetailSheet, StatusBadge, DataTable (TanStack, stacked rows on mobile).
  - Replace the regex token guard in `web/src/tokens.test.ts` with a parser-based check (`@vue/compiler-sfc` for templates and bindings, a CSS parser such as `postcss` for nested rules) that closes the three known limits in `docs/architecture/frontend-theme-and-type.md`. Keep every existing fixture.
  - Accept: every existing page renders inside the new shell; component gallery page in Build mode (`/build/gallery`) shows every component in every state; the token guard has no known limits.
- [ ] **B3. Typography and icons** (Muse)
  - Carry-over from B2 review: Permissions phone cards limited to 2 or 3 fields; sign-in padding 16/24/32 by breakpoint; stacked labels at caption weight 500; unique phone label for the Work queue stub; focus box only on focus-visible for the sidebar brand; Ledger inner tables stack on phones; the two residual token-guard limits in `docs/architecture/frontend-theme-and-type.md`.
  - Apply design-system §4 roles to every page; `text-wrap` rules; tabular numbers in tables.
  - Create `web/src/icons.ts` (safe Lucide registry with fallback) and use the §5 table everywhere.
  - Accept: grep finds no direct font sizes or Lucide imports outside the registry and `components/ui`.
- [ ] **B4. Accent presets and charts** (Muse; decided: both presets, design-system §2.3)
  - Ship `ink` (default) and `red` presets as `[data-accent]` blocks in `theme.css` for light and dark, and the chart series; a Workspace setting chooses one (Settings page, with a live preview); verify text on primary ≥ 4.5:1 for every preset and theme.
  - Accept: only buttons, focus ring, selected-nav marker and charts carry the accent.
- [ ] **B4a. Steering sync** (master)
  - Confirm `AGENTS.md`, `.kiro/steering/athena-design.md` and `docs/design/athena-design-system.md` agree with what B1 to B4 shipped; move tokens from *Proposed* to implemented; update `docs/architecture/frontend-theme-and-type.md` and `docs/README.md`.

## B. Platform completion before ledgers

- [ ] **B5. Workspace identity and logo** (Codex then Muse)
  - Workspace settings for display name and logo (stored as a Workspace file, validated SVG/PNG ≤ 512 KB, served same-origin with a content hash URL). CLI `athena workspace set --name --logo file`. `athena new --logo`.
  - Desk: logo and name in header, sign-in, tab title, favicon; Athena mark fallback; "Powered by Athena" footer.
  - Logos are `athena-black-logo` (light theme) and `athena-white-logo` (dark theme) at the repository root, PNG without an extension. Rename to `.png`, generate favicon and header sizes, and switch the mark with the theme.
- [ ] **B6. Everyday vocabulary and help** (Muse, Codex for the server side)
  - `web/src/copy/glossary.ts` with plain labels and one-line explanations for every status, lifecycle state, ledger and access term; `circle-help` popovers; View mode uses only glossary words.
  - Naming registry `docs/design/naming.md`: product and layer names (section N below), forbidden old terms, and how a name maps to CLI, proto service, package and UI label. `terms_test.go` extended to new forbidden terms only after user approval.
- [ ] **B7. Sessions, rate limits and tab management** (Codex, ADR required, security)
  - Current: idle timeout 8 h, absolute 7 days, 5 failed sign-ins lock 15 min (`internal/auth/service.go`). No request rate limiting exists.
  - Make idle and absolute lifetimes Workspace settings with safe bounds (OWASP Session Management): defaults idle 30 min for everyday users and 15 min for Build mode and root, absolute 12 h; "Keep me signed in on this device" extends absolute to 7 days for non-builder sessions only. Root and Build require re-entering the password after 15 min idle before a destructive action (step-up).
  - Rotate the session ID at sign-in, privilege change and step-up. Sign-out and password change send `Clear-Site-Data: "cache", "cookies", "storage"`.
  - Rate limiting in the Go server: per IP and per username on `AuthService.Login`, `CompleteChallenge`, `StartReset` (token bucket, for example 10 per minute per IP, 5 per 15 min per username, with `Retry-After`); per session on write procedures (for example 60 per minute) and on expensive reads (Reports, export). Library: `golang.org/x/time/rate` (maintained by the Go team, no CGO, fits the single binary). Limits are Workspace settings. Denials are logged to the Access Log without values.
  - Tabs: one session across tabs of the same browser; `BroadcastChannel("athena-session")` so sign-out, expiry, Workspace switch and role change in one tab update all tabs; a warning dialog 2 min before idle expiry with "Stay signed in"; activity in any tab counts; a Workspace switch clears caches (playbook §8). Session list shows device and last seen; revoke from the list.
  - Accept: tests for each limit and timeout; Bruno checks for 429 and expiry; Desk test for cross-tab sign-out.
- [ ] **B8. Root administrator and Build/View presentation** (Codex then Muse)
  - Confirm ADR-0018 behavior end to end: created at `athena new` / first boot with the database password, bypasses all gates, cannot be suspended, edited, have Roles changed, or be deleted, through API, CLI or Desk (tests per path). Every root action is audit-logged as root. Change-password rotates only root's own password.
  - Desk: root sees a quiet "Root administrator" caption and a `lock` badge on its own user row with the reason; its row actions are disabled with the reason.
  - Fix: a fresh load of `?mode=build` lands in View because the `canBuildDesk` watch fires during startup (found in B2, present before B2).
  - Build/View: mode switch per design-system §7; Build shows a persistent "Building" caption and a hairline-separated sidebar group; View hides all Build navigation; a "Preview as" control in Build shows View as a chosen Role (read-only, from playbook §8 "Developer entry").
- [ ] **B9. Form builder for Record Types** (Codex then Muse, ADR required, changes durable definitions)
  - Record Type definitions gain a layout: sections, columns (1 or 2), field order, field width, help text, conditional visibility (CEL expression, ADR-0013), and an Image field type (stored through Workspace files, thumbnails, alt text required) plus static image and text blocks.
  - Desk: drag-and-drop builder (keyboard alternative with move up/down) with live preview at mobile and desktop width; the View form renders from the layout. Layout changes are non-destructive saves through the journaled publisher.
  - Accept: layout round trip tests; Image field upload, display, delete; keyboard-only reordering works.
- [ ] **B10. Art registry, hairlines and ASCII** (Muse; needs approval to add `@lucasmarkes/hairline`)
  - `web/src/art/registry.ts` per design-system §9; figures on sign-in, first-run, empty states, 404 and offline; ASCII mark for the CLI banner (`cmd/athena`) and sign-in footer. Custom figures made with `hairline-create` go in `web/src/art/figures/`.
  - Accept: one figure per screen at most (test over the registry); reduced motion rests every figure.
- [ ] **B11. Install and first-run flow** (Codex then Muse)
  - Document and test the path: install binary → `athena init` → `athena new "Name" [--logo]` → `athena run` → sign in as root → Setup wizard (company, logo, currency, period, invite first owner) → readiness view. Every step has a CLI and Desk equivalent and a plain-language error.
  - Write `docs/guides/getting-started.md` from the tested path.

## C. Kernel (batch-2 groups, reordered; text in batch-2 tasks.md)

Order follows dependencies. Desk parts of each group follow the design system and come after the group's backend is merged.

- [ ] **C1. G6 Posting engine** 6.2 to 6.12 (6.1 done). Desk voucher view uses DetailSheet and plain words ("Recorded in the books").
- [ ] **C2. G8 Jobs, outbox, events, workflows, secrets, lifetimes.** Invite delivery through the outbox. JobsPage and SettingsPage.
- [ ] **C3. G10 Pack packaging.** PackService, install plan preview, PacksPage.
- [ ] **C4. G13 Trees, effective dating, determination specificity, UoM.**
- [ ] **C5. G7 Party ledger, settlements, realised FX.** "What customers owe" and "What we owe" in View vocabulary.
- [ ] **C6. G9 Balances, Query compiler, Reports v1, Report Runs.** ReportsPage with charts using `--chart-*` only.
- [ ] **C7. G11 Stock ledger, reservations, serial and batch, valuation.** (Needs valuation method, Z3.)
- [ ] **C8. G12 Remaining ledgers.**
- [ ] **C9. G14 Report engine, work queues, Activity timeline, Explain this movement.**
- [ ] **C10. G15 Integration surface.** Attachments reuse the B9 Image storage.

## D. Product surface

- [ ] **D1. Roles and what each user sees** (master writes, user approves)
  - `docs/design/pages-by-role.md`: every route, its audience (design-system §11), required permission, everyday name, icon and empty state, for root, owner, builder, manager, accountant, storekeeper, cashier, customer and visitor. The page registry (B2) is generated from it or tested against it.
- [ ] **D2. Process maps** (master writes, user approves)
  - `docs/design/processes.md`: each flow from `docs/revamp/Athena-Process-and-Event-Catalogue.md` and `Records-and-movements.md` as steps in everyday words: who does it, which screen, what the system records, what the user sees next, what can go wrong. Starts with: buy, receive, sell, take payment, return, count stock, close the month, invite a person, recover access.
- [ ] **D3. Public pages and SEO** (Codex then Muse; ADR required: adds server-rendered public routes)
  - Public routes served by the Go server as server-rendered HTML from approved public projections (no admin RecordService from the public): storefront, category, product, course catalogue, transcript verification, Workspace home. Per page: unique title and description, canonical, Open Graph and Twitter tags with a 1200x630 image, JSON-LD (Organization, Product, BreadcrumbList, Course), `sitemap.xml` and `robots.txt` per Workspace. Carts, checkout, tracking, sign-in and the whole Desk send `noindex`. Core Web Vitals targets LCP ≤ 2.5 s, INP ≤ 200 ms, CLS ≤ 0.1.
  - Keep the Desk SPA for signed-in work; do not move it to SSR.
- [ ] **D4. G16 PWA and offline stock count** (needs Z6).
- [ ] **D5. G17 Business starter Packs and Setup Wizard** (needs Z1, Z2). Wizard reuses B11.
- [ ] **D6. Pack component kit** (Muse with Codex)
  - From `docs/revamp/Athena-Checkout-Commerce-and-Learning.md` §7 and the industry doc: shared task-screen components registered by Packs: POS basket and tender panel, customer display, attendant queue, product card and listing table, cart and checkout, inbox thread, quiz runner, gradebook, count sheet, kitchen ticket, collection board. Each in the component gallery with all states.
- [ ] **D7. G18 Industry and channel Packs** (needs Z7): supermarket and POS, e-commerce and channels, WhatsApp, learning, construction, restaurant, dairy, poultry, hatchery (optional), butchery, manufacturing; built on D6.

## E. Beta release

- [ ] **E1. First-day usability trial** (playbook §8): owner, storekeeper, accountant tasks on a clean Workspace; record wrong turns and time to first transaction; fix the top issues.
- [ ] **E2. Hardening pass:** impeccable `audit` and `harden` on every page; accessibility (keyboard, contrast, screen reader labels); Bruno black-box suite extended to every service; security review of sessions, rate limits, public routes.
- [ ] **E3. Docs and release:** README, getting-started, decision log, ADR statuses, CHANGELOG `0.1.0-beta`, annotated tag after the user approves.

## N. Naming (inspiration, not rules; user approves before any rename)

Pattern: **Athena + one concrete noun**, 1 to 2 syllables, works as `athena-<noun>` in the CLI and as a word a user understands. Names are product labels only; code identifiers, proto services and table names stay as they are, so nothing breaks backwards. A rename of a code identifier needs its own ADR and a compatibility alias.

| Layer | Current code name | Candidate product name |
| --- | --- | --- |
| Go runtime, kernel, database | `athena` server, `internal/server` | Athena Core |
| ConnectRPC API | `athena.v1` services | Athena Wire |
| Starlark Rules | `internal/rules`, `RuleService` | Athena Script |
| Audit log and backups | `athena_audit_log`, `athena audit` | Athena Vault |
| Workflows and jobs | `internal/jobs` (G8) | Athena Loom |
| Ledgers and books | `internal/ledger`, `LedgerService` | Athena Ledger |
| Stock and logistics | `internal/stock` (G11) | Athena Stock |
| Reports | `internal/report` (G9) | Athena Prism |
| Telemetry | `internal/telemetry` | Athena Lens |
| Import and migration | importers (G15) | Athena Bridge |

Gates per name: pattern, 1 to 2 syllables, clean as `athena-<noun>`, the user can guess what it does. The everyday user mostly sees the plain section names ("Books", "Stock", "Reports"); the product names appear in docs, the CLI and Settings.

## Z. Decisions the user must make (OPEN)

| # | Question | Blocks |
| --- | --- | --- |
| Z0 | DECIDED 2026-11-21: both presets, chosen per Workspace | B4 |
| Z1 | Pilot company, jurisdiction, chart and tax | D5 |
| Z2 | Permitted backdating window (ADR-0023 O-011) and book policy (O-012) | C1 |
| Z3 | Stock valuation method and negative stock | C7 |
| Z4 | Retention periods and key custody | C2 |
| Z5 | Licence | C3, E3 |
| Z6 | `vite-plugin-pwa`, deployment profile and recovery objectives | D4 |
| Z7 | Pack build order, pilot country, provider qualification | D7 |
| Z8 | Session and rate-limit defaults in B7 | B7 |
| Z9 | DECIDED 2026-11-21: added `@lucasmarkes/hairline` 0.5.1 (MIT, pinned) and `golang.org/x/time` v0.16.0 (indirect until B7 imports `rate`) | B10, B7 |
| Z10 | Satori web embedding rights (bundle the font or keep local-only) | B3, D3 |
| Z11 | Product names in section N | B6 |
| Z12 | DECIDED 2026-11-21: committed on `feat/batch-2-g1-g6` (951ed0d, bf47716) with `.agents/` and `.kiro/` | everything |
