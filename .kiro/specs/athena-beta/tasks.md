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
- [x] **B3. Typography and icons** (Muse)
  - Carry-over from B2 review: Permissions phone cards limited to 2 or 3 fields; sign-in padding 16/24/32 by breakpoint; stacked labels at caption weight 500; unique phone label for the Work queue stub; focus box only on focus-visible for the sidebar brand; Ledger inner tables stack on phones; the two residual token-guard limits in `docs/architecture/frontend-theme-and-type.md`.
  - Apply design-system §4 roles to every page; `text-wrap` rules; tabular numbers in tables.
  - Create `web/src/icons.ts` (safe Lucide registry with fallback) and use the §5 table everywhere.
  - Accept: grep finds no direct font sizes or Lucide imports outside the registry and `components/ui`.
- [x] **B4. Appearance: presets and custom colors** (Codex for the server, then Muse for the Desk; design-system §2.3, §18)
  - Server: Workspace appearance stored as a setting (`brand.accent` preset or custom accent and chart colors in OKLCH), `WorkspaceService.GetAppearance` and `SetAppearance` with permission `settings:appearance`, contrast check returning the nearest passing value, applied at sign-in and on the sign-in page.
  - Desk: Settings → Appearance with swatches, Custom with a color picker for the accent and charts, a light and dark live preview, save and reset; `data-accent` or custom properties applied to `<html>` at load; per-user light, dark or system choice.
  - Accept: Go tests for storage, permission and contrast; choosing a color changes only accent spots for every user after reload.
  - Accept: only buttons, focus ring, selected-nav marker and charts carry the accent.
- [x] **B3a. Experience pass: every page, actions, sidebar, detail sheets, mobile native** (Muse t4, after B3 in the same worktree)
  - Sidebar grouped per design-system §15; Build/View control and every container on the radius family (no square corners).
  - Action button system per §14 (`web/src/components/ActionButton.vue` and `ActionGroup.vue`), applied to every action on every page: Edit, Resend, Reset, Suspend, Reactivate, Revoke, End session, Delete, Save, Invite and the rest.
  - Detail panels per §16: full-viewport-height right sheet on desktop and tablet (Access users, roles and sessions, Record rows, Rule runs, Files); bottom drawer on phones. Remove inline detail cards.
  - Page pass on Overview, Record Types, Records, Rules, Files, Access, Ledger and Sign-in: spacing tokens, one primary action, hierarchy per §4, empty, loading and error states, impeccable `layout` and `polish`, emilkowalski-motion.
  - Native mobile and tablet per §13: floating bottom navigation, bottom drawers for every dialog, form, picker, filter and menu (content height, swipe to close, dirty-form guard), edge-swipe back and forward, minified tables and charts, pull to refresh, web app manifest.
  - Accept: browser pass at 375, 768, 1280, 1440 in both themes on every page; a scripted check that no dialog opens centered below 1024 px; swipe and drawer gestures checked in Chrome device emulation with touch.
- [x] **B12. Notifications** (Codex for the server, then Muse for the Desk; ADR required, new durable data)
  - Migration: `athena_notifications` (recipient user, kind, subject reference, actor, created, read_at) and `NotificationService` (List with keyset paging, UnreadCount, MarkRead, MarkAllRead). Notifications are written in the same transaction as the event, by the services that own it: auth (invite accepted, access changed, sign-in failed or locked, session ended by someone else), rules (save stopped), lifecycle (approval waiting), ledger (period closed). Text is built from a kind and parameters; no Record values in the row beyond the subject's display label.
  - Desk per design-system §17: bell with unread count, inbox popover (bottom drawer on phones), Today and Earlier, mark read, deep links, 30 s polling.
  - Accept: a Go test per source; the Desk shows a new notification within 30 s without reload.
- [ ] **B4a. Steering sync** (master)
  - Confirm `AGENTS.md`, `.kiro/steering/athena-design.md` and `docs/design/athena-design-system.md` agree with what B1 to B4 shipped; move tokens from *Proposed* to implemented; update `docs/architecture/frontend-theme-and-type.md` and `docs/README.md`.

## B. Platform completion before ledgers

- [x] **B5. Workspace identity and logo** (Codex then Muse). Complete 2026-11-22.
  - Slice 1 merged (2e5595e+e80ad39+union 4698f96): GetIdentity/SetIdentity, brand/ storage, /brand/ routes, favicons, CLI identity + new --logo. Slice 2 merged (bd49f75+c2674c2): Desk header/sign-in/tab/favicon, Settings identity section.
  - Workspace settings for display name and logo (stored as a Workspace file, validated SVG/PNG ≤ 512 KB, served same-origin with a content hash URL). CLI `athena workspace set --name --logo file`. `athena new --logo`.
  - Desk: logo and name in header, sign-in, tab title, favicon; Athena mark fallback; "Powered by Athena" footer.
  - Logos are `athena-black-logo` (light theme) and `athena-white-logo` (dark theme) at the repository root, PNG without an extension. Rename to `.png`, generate favicon and header sizes, and switch the mark with the theme.
- [x] **B6. Everyday vocabulary and help** (Muse, Codex for the server side)
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
- [x] **B9a. Record Types list and editor page** (Muse; added 2026-11-21 at the user's request; design-system §19; before B9, after B3 merges)
  - Replace the list-plus-inline-form page with the §19 list (DataTable on desktop and tablet with name, description, kind, fields, records, Pack, last changed, changed by; stacked rows and a summary drawer on phones) and the dedicated editor page at `/build/record-types/<key>` with Form, Fields, Settings, Rules and History tabs. Move the existing editing (fields, settings, save, destructive-save planning, delete) onto the editor page with no behavior change. The Form tab is a placeholder until B9.
  - Server: Record counts and last-changed actor per Record Type if `RecordTypeService.ListRecordTypes` lacks them (Codex or Muse server worker, additive proto fields).
  - Accept: no form shows on the list; every existing Record Type action still works from the editor page; links to `/build/record-types/<key>` survive reload.
- [x] **B4b. Appearance page redesign** (Muse t4, after the sessions slice merges; design-system §18 revised): left column of choices (Accent with nine named presets plus Custom, Charts palettes, Your theme) and a realistic preview card in light and dark. Server: extend the preset list (Ink, Slate, Blue, Sky, Green, Amber, Orange, Brown, Red) and chart palettes, validated against one shared list; contrast test for every preset. Merged 2026-11-22 (fb603a6) after one fix loop (scoped preview ramp, legacy custom charts).
- [ ] **B2a. Table system** (Muse; design-system §21): one DataTable for every table (Records, Record Types, Access users, roles, sessions and permissions, Ledger masters, Rules and Rule Runs, Files, Notifications full list): bordered full-width frame with proper padding, select column with bulk actions where they exist, row numbers across pages, primary column with icon and second line, actions menu, toolbar with search, filters and Columns, selection bar, footer with range, rows per page, page count and Previous and Next. Server paging through the existing keyset tokens; totals where available.
  - Slice 1 merged 2026-11-22 (afc70a6): DataTable itself, Records with server keyset paging, Gallery showcase. Slices 2 (Access, Rules, Files) and 3 (Ledger, notifications, Record Types) open.
- [x] **B9b. New Record Type flow** (Muse; design-system §20): name-first creation dialog, derived key, kind with explanations, Create saves a named Record Type with a default Name field and opens the editor; editable title in the editor. Merged 2026-11-22 (2468598), review PASS, no fix loop.
- [ ] **B9. Form builder for Record Types** (Codex then Muse, ADR required, changes durable definitions)
  - Slice 1 (server: layout + Image field) merged pre-shift (9260d04). Slice 2.1 merged 2026-11-22 (99cc6ed): View renderer + Image field UI. Open: slice 2.2 (builder), follow-up 2.3 (list-image thumbnails after B2a).
  - Record Type definitions gain a layout: sections, columns (1 or 2), field order, field width, help text, conditional visibility (CEL expression, ADR-0013), and an Image field type (stored through Workspace files, thumbnails, alt text required) plus static image and text blocks.
  - Desk: drag-and-drop builder (keyboard alternative with move up/down) with live preview at mobile and desktop width, in the editor page's Form tab (B9a); optional image block and an optional hairline or ASCII header from the art registry (B10); the View form renders from the layout. Layout changes are non-destructive saves through the journaled publisher.
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

## Follow-ups from merged slices (2026-11-21)

- B9a: Delete next to Save in the editor header; editor title not an h1; field-row action icons cramped; Required checkbox alignment; dead `.records-layout` rule (style.css:178).
- B3: Rule Run sheet shows messages, not a diff (needs a RuleRun diff field on the server).
- B4: unset custom chart swatches show black; "Selected page" preview lacks the accent; Security section intro copy too playful.
- Security RPC: MariaDB-only upsert; last-writer-wins on concurrent sets.
- B4b: accent dots render light primaries on dark pages (ink dot invisible); dark ramp washes toward white for near-white primaries; CLI usage still says `--preset ink|red` with no chart-palette flag; ramp-fallback/Custom-swatch ink dots and `text-[11px]` month labels in preview.
- B9b: stale "draft" comments in `useDeskRoute.ts`; one-way Change (no return to auto-derive key); duplicate-name Enter shows no inline error until blur.
- B2a slice 1: toolbar search text starts under the icon (`pl-9` loses to Input `px-3`); dead `.entries-grid`/`.entries-resizer` CSS; hardcoded Draft/Submitted/Cancelled in table (use glossary); clearing filters fires a duplicate list request; `pressTimer` not cleared on unmount; search width 448 px vs 480 px in §21. API gaps (server lacks them): totals, free-text search, sort params, bulk-delete RPC.
- B5 slice 1: `athena role` help omits `settings:*` permissions; `new --logo` validates content only after creating the Workspace; concurrent SetIdentity can silently overwrite (last-writer-wins, old generations kept); prune error surfaces after the save committed; favicons re-render per request; `identity set` with no flags writes an identical generation.
- B9 form 1: alt-text error renders twice (inner + outer); required-but-empty field in a hidden section blocks save invisibly; empty ImageField label `for` points nowhere; section list uses filtered index as key/label; WorkspaceImage comment mentions lists (forms-only until 2.3); visibility evaluates at open only, not on draft edits.
- B5 slice 2: "No logo" empty text unreadable in mismatched swatch/theme combos; logo errors lack the CircleAlert icon pattern; clearing the display name toasts success but snaps back; theme-color static in dark theme (pre-existing); fallback favicon is the black mark in both themes.

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
| Z8 | DECIDED 2026-11-21 ("start all of those things"): B7 defaults as written | B7 |
| Z9 | DECIDED 2026-11-21: added `@lucasmarkes/hairline` 0.5.1 (MIT, pinned) and `golang.org/x/time` v0.16.0 (indirect until B7 imports `rate`) | B10, B7 |
| Z10 | Satori web embedding rights (bundle the font or keep local-only) | B3, D3 |
| Z11 | Product names in section N | B6 |
| Z12 | DECIDED 2026-11-21: committed on `feat/batch-2-g1-g6` (951ed0d, bf47716) with `.agents/` and `.kiro/` | everything |
