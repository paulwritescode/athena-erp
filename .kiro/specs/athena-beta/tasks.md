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
- [x] **B4a. Steering sync** (master). Complete 2026-11-22.
  - Confirmed `AGENTS.md`, `.kiro/steering/athena-design.md` and `docs/design/athena-design-system.md` agree with what B1 to B4 shipped; tokens moved from *Proposed* to implemented; `docs/architecture/frontend-theme-and-type.md` (nine presets, Satori not Cal Sans, Appearance page) and `docs/README.md` updated.

## B. Platform completion before ledgers

- [x] **B5. Workspace identity and logo** (Codex then Muse). Complete 2026-11-22.
  - Slice 1 merged (2e5595e+e80ad39+union 4698f96): GetIdentity/SetIdentity, brand/ storage, /brand/ routes, favicons, CLI identity + new --logo. Slice 2 merged (bd49f75+c2674c2): Desk header/sign-in/tab/favicon, Settings identity section.
  - Workspace settings for display name and logo (stored as a Workspace file, validated SVG/PNG ≤ 512 KB, served same-origin with a content hash URL). CLI `athena workspace set --name --logo file`. `athena new --logo`.
  - Desk: logo and name in header, sign-in, tab title, favicon; Athena mark fallback; "Powered by Athena" footer.
  - Logos are `athena-black-logo` (light theme) and `athena-white-logo` (dark theme) at the repository root, PNG without an extension. Rename to `.png`, generate favicon and header sizes, and switch the mark with the theme.
- [x] **B6. Everyday vocabulary and help** (Muse, Codex for the server side)
  - `web/src/copy/glossary.ts` with plain labels and one-line explanations for every status, lifecycle state, ledger and access term; `circle-help` popovers; View mode uses only glossary words.
  - Naming registry `docs/design/naming.md`: product and layer names (section N below), forbidden old terms, and how a name maps to CLI, proto service, package and UI label. `terms_test.go` extended to new forbidden terms only after user approval.
- [x] **B7. Sessions, rate limits and tab management** (Codex, ADR required, security). Complete 2026-11-24 (verified in-tree: ratelimit.go + x/time, sessionChannel, step-up, Clear-Site-Data, 30m idle, rotation, idle warning, session list). Descope: Workspace-switch cache clear descoped until a switch flow exists (B7 decision).
  - Current: idle timeout 8 h, absolute 7 days, 5 failed sign-ins lock 15 min (`internal/auth/service.go`). No request rate limiting exists.
  - Make idle and absolute lifetimes Workspace settings with safe bounds (OWASP Session Management): defaults idle 30 min for everyday users and 15 min for Build mode and root, absolute 12 h; "Keep me signed in on this device" extends absolute to 7 days for non-builder sessions only. Root and Build require re-entering the password after 15 min idle before a destructive action (step-up).
  - Rotate the session ID at sign-in, privilege change and step-up. Sign-out and password change send `Clear-Site-Data: "cache", "cookies", "storage"`.
  - Rate limiting in the Go server: per IP and per username on `AuthService.Login`, `CompleteChallenge`, `StartReset` (token bucket, for example 10 per minute per IP, 5 per 15 min per username, with `Retry-After`); per session on write procedures (for example 60 per minute) and on expensive reads (Reports, export). Library: `golang.org/x/time/rate` (maintained by the Go team, no CGO, fits the single binary). Limits are Workspace settings. Denials are logged to the Access Log without values.
  - Tabs: one session across tabs of the same browser; `BroadcastChannel("athena-session")` so sign-out, expiry, Workspace switch and role change in one tab update all tabs; a warning dialog 2 min before idle expiry with "Stay signed in"; activity in any tab counts; a Workspace switch clears caches (playbook §8; descoped 2026-11-24 until a switch flow exists, B7). Session list shows device and last seen; revoke from the list.
  - Accept: tests for each limit and timeout; Bruno checks for 429 and expiry; Desk test for cross-tab sign-out.
- [x] **B8. Root administrator and Build/View presentation** (Codex then Muse). Complete 2026-11-22.
  - Slice 1 merged (4d03467): root guards shared across API/CLI/Desk + tests per path, audit attribution, password rotation. Slice 2 merged (48d804d+d81473f): root caption/badge, ?mode=build fix, mode switch, View hiding, read-only Preview-as.
  - Confirm ADR-0018 behavior end to end: created at `athena new` / first boot with the database password, bypasses all gates, cannot be suspended, edited, have Roles changed, or be deleted, through API, CLI or Desk (tests per path). Every root action is audit-logged as root. Change-password rotates only root's own password.
  - Desk: root sees a quiet "Root administrator" caption and a `lock` badge on its own user row with the reason; its row actions are disabled with the reason.
  - Fix: a fresh load of `?mode=build` lands in View because the `canBuildDesk` watch fires during startup (found in B2, present before B2).
  - Build/View: mode switch per design-system §7; Build shows a persistent "Building" caption and a hairline-separated sidebar group; View hides all Build navigation; a "Preview as" control in Build shows View as a chosen Role (read-only, from playbook §8 "Developer entry").
- [x] **B9a. Record Types list and editor page** (Muse; added 2026-11-21 at the user's request; design-system §19; before B9, after B3 merges)
  - Replace the list-plus-inline-form page with the §19 list (DataTable on desktop and tablet with name, description, kind, fields, records, Pack, last changed, changed by; stacked rows and a summary drawer on phones) and the dedicated editor page at `/build/record-types/<key>` with Form, Fields, Settings, Rules and History tabs. Move the existing editing (fields, settings, save, destructive-save planning, delete) onto the editor page with no behavior change. The Form tab is a placeholder until B9.
  - Server: Record counts and last-changed actor per Record Type if `RecordTypeService.ListRecordTypes` lacks them (Codex or Muse server worker, additive proto fields).
  - Accept: no form shows on the list; every existing Record Type action still works from the editor page; links to `/build/record-types/<key>` survive reload.
- [x] **B4b. Appearance page redesign** (Muse t4, after the sessions slice merges; design-system §18 revised): left column of choices (Accent with nine named presets plus Custom, Charts palettes, Your theme) and a realistic preview card in light and dark. Server: extend the preset list (Ink, Slate, Blue, Sky, Green, Amber, Orange, Brown, Red) and chart palettes, validated against one shared list; contrast test for every preset. Merged 2026-11-22 (fb603a6) after one fix loop (scoped preview ramp, legacy custom charts).
- [x] **B2a. Table system** (Muse; design-system §21): one DataTable for every table (Records, Record Types, Access users, roles, sessions and permissions, Ledger masters, Rules and Rule Runs, Files, Notifications full list): bordered full-width frame with proper padding, select column with bulk actions where they exist, row numbers across pages, primary column with icon and second line, actions menu, toolbar with search, filters and Columns, selection bar, footer with range, rows per page, page count and Previous and Next. Server paging through the existing keyset tokens; totals where available. Complete 2026-11-22.
  - Slice 1 merged (afc70a6): DataTable itself, Records with server keyset paging, Gallery showcase. Slice 2 merged (b4791a3+96edd68): Access, Rules, Files. Slice 3 merged (f3c8c1b): Ledger masters, Record Types list. Decisions recorded: the Access permissions matrix stays a plain table (Role×RecordType action grid, not a list — DataTable does not fit); the notifications full list is skipped until designed (Stylus; bell inbox covers §17).
- [x] **B9b. New Record Type flow** (Muse; design-system §20): name-first creation dialog, derived key, kind with explanations, Create saves a named Record Type with a default Name field and opens the editor; editable title in the editor. Merged 2026-11-22 (2468598), review PASS, no fix loop.
- [x] **B9. Form builder for Record Types** (Codex then Muse, ADR required, changes durable definitions). Complete 2026-11-22.
  - Slice 1 (server: layout + Image field) merged pre-shift (9260d04). Slice 2.1 merged (99cc6ed): View renderer + Image field UI. Slice 2.2 merged (4113b23): drag/keyboard builder with preview. Follow-up 2.3 merged (7c7192d): list-image thumbnails on B2a columns.
  - Record Type definitions gain a layout: sections, columns (1 or 2), field order, field width, help text, conditional visibility (CEL expression, ADR-0013), and an Image field type (stored through Workspace files, thumbnails, alt text required) plus static image and text blocks.
  - Desk: drag-and-drop builder (keyboard alternative with move up/down) with live preview at mobile and desktop width, in the editor page's Form tab (B9a); optional image block and an optional hairline or ASCII header from the art registry (B10); the View form renders from the layout. Layout changes are non-destructive saves through the journaled publisher.
  - Accept: layout round trip tests; Image field upload, display, delete; keyboard-only reordering works.
- [x] **B10. Art registry, hairlines and ASCII** (Muse; needs approval to add `@lucasmarkes/hairline`). Complete 2026-11-22.
  - Slice 1 merged (82cf024): registry + sign-in/404/offline/CLI placements with stock figures. Slice 2 merged (cf95819): empty-state figures per the meaning table (Records carries no art).
  - `web/src/art/registry.ts` per design-system §9; figures on sign-in, first-run, empty states, 404 and offline; ASCII mark for the CLI banner (`cmd/athena`) and sign-in footer. Custom figures made with `hairline-create` go in `web/src/art/figures/`.
  - Accept: one figure per screen at most (test over the registry); reduced motion rests every figure.
- [x] **B11. Install and first-run flow** (Codex then Muse). Complete 2026-11-22.
  - Slice 1 merged 2026-11-22 (b27b6c3): CLI flag hardening + e2e-install.sh 28/28 + getting-started guide. Slice 2 merged 2026-11-22 (7ccf163+4c666b5): Desk Setup wizard (company, logo, currency, period, invite first owner) + readiness view with per-step denied states; round-1 blocker (period step wedges after failed save) fixed with in-place retry.
  - Document and test the path: install binary → `athena init` → `athena new "Name" [--logo]` → `athena run` → sign in as root → Setup wizard (company, logo, currency, period, invite first owner) → readiness view. Every step has a CLI and Desk equivalent and a plain-language error.
  - Write `docs/guides/getting-started.md` from the tested path.

## C. Kernel (batch-2 groups, reordered; text in batch-2 tasks.md)

Order follows dependencies. Desk parts of each group follow the design system and come after the group's backend is merged.

- [ ] **C1. G6 Posting engine** 6.2 to 6.12 (6.1 done). Desk voucher view uses DetailSheet and plain words ("Recorded in the books").
  - Slice 1 merged 2026-11-22 (9d87fbf+a9e1d49): plan validation + single-transaction writer + balanced-postings property (6.2+6.3); Z2 policy parts return failed_precondition sentinels. Open: slices 2–6 (operations, reversal, hooks, RPCs/CLI, voucher view).
  - Slice 2 merged 2026-11-22 (f94fe09+367a56c): Operation_ID contract + idempotent replay + concurrent-replay property (6.4+6.5); cross-actor winner disambiguated by group id (fix loop 1). Z2 unchanged (2 sentinels, still pinned). Open: slices 3–6 (reversal, hooks, RPCs/CLI, voucher view).
  - Slice 3 merged 2026-11-22 (4334fc3): reversal mirrors + correction-period rules (6.6+6.7); Property 10 submit+cancel netting; rebase adaptation onto the C2 event bus (insertOutbox via events.Emit) delta-reviewed. Z2 unchanged (ErrCorrectionWindowOpen sentinel, still pinned). Open: slices 4–6 (hooks, RPCs/CLI, voucher view).
  - Slice 4 merged 2026-11-23 (8fd7bef): lifecycle hooks + Posting Rule type with activation gate (6.8); internal/lifecycle deviation accepted (registry suffices); records.proto +posting_preview_hash; fixture replay + plan diff + two-step named approval; Z2 sentinels byte-identical. Open: slices 5–6 (RPCs/CLI, voucher view + docs + checkpoint).
  - Slice 5 merged 2026-11-23 (cf2f674): posting RPCs + CLI (6.9); preview-hash loop closed; dryRun side-effect-free; gate policies + both-halves test; Z2 untouched. Open: slice 6 (6.10 voucher view + 6.11 docs + 6.12 checkpoint; 6.11 committed on the branch, Desk still gated).
  - Slice 6 merged 2026-11-23 in two parts (train1 919bab4): 6.11 docs (fdf98c7+d897d01) — G6.8–6.9 hooks/Rule-type/RPC/CLI docs, one review-found false mechanism claim fixed; 6.12 checkpoint verified on 919bab4 with no commit (r-c1 evidence review: merge-gate reuse valid, browser 22/22, 6/6 shots match). Z2 untouched. Open: 6.10 Desk voucher view only (GO given 2026-11-24, queued in L1 after C5 slice 1; worktree + brief ready).
  - 6.10 merged 2026-11-24 (train4 bafe51d, 1 commit 9a299ca): Desk voucher view — preview dialog with hash-carrying submit, Books-entries sheet (status/lines/attempts), Recorded-in-the-books badge, blocked-post toasts; browser 25/26 + 15/15 shots (1 harness-class fail, gaps disclosed). Review PASS (5 follow-ups). Z2 untouched. Open: slice 7 (Z2 backdating/book policy, briefed, queued in L1 after C5 slice 4 — same worker/package, avoids mid-slice interrupt); C1 ticks when it merges.
  - Follow-up: Posting Rule activation — approver may be the author (four-eyes not enforced for beta, Stylus-accepted); approval record keeps author (v2 actor) and approver (v3 actor) so it can be audited (Z13).
- [ ] **C2. G8 Jobs, outbox, events, workflows, secrets, lifetimes.** Invite delivery through the outbox. JobsPage and SettingsPage.
  - Slice 1 merged 2026-11-22 (5d1e55f): migration 17 (events/outbox/inbox + subscriptions + jobs/occurrences + workflows/tasks) + ADR-0028 (8.1 adjusted); Z4 secrets/lifetimes deferred. Open: slices 2+ (subscriptions, worker, jobs, workflows, secrets/lifetimes once Z4 is decided).
  - Slice 2 merged 2026-11-22 (6be771e): event bus (Emit/Claim/Declare/Match + dispatcher) + posting/lifecycle emit hooks + Property 13 inbox dedup. Z4/Z2 clean. Open: slices 3+ (jobs/worker, workflows, secrets/lifetimes once Z4 is decided).
  - Slice 3 merged 2026-11-23 (415fed7+dce52a3+461633f+f3d547f, train1 919bab4): job runner (leases/fencing/backoff/occurrences/worker startup) + review fixes (B1 row-first recovery, F1/F2/F4/F5, F3 migration 19 occurrence_key 700). Round-2 review PASS. Z4/Z2 clean. Open: slices 4–6 (workflows, invite/reset delivery, events.proto/handlers/CLI/Desk; split sm-c2 running, sub-slice A merged on the task branch at 8684ccb).
  - Slices 4–6 merged 2026-11-24 (2dd8bbf..87db25e, 7 commits via sm-c2 split): workflows + Property 14 + consumer wiring, invite/reset outbox delivery, EventService/JobService + CLI + Jobs/Settings surface. Review PASS (6 follow-ups F1–F6). Open: secrets slices 8.8–8.10 (Z4, new migration + ADR-0033), then C10.
  - Secrets slice 1 merged 2026-11-24 (65e59cc, 2 commits): migration 22 (secret refs, key versions, disposal register, holds) + ADR-0033 (Z4 exact). Review PASS. Merge gate 1150s (17 Go ok, 646 web). Open: slices 2–5 (8.8 core, 8.9 Property 16, 8.10 disposal, CLI + docs), then C10.
- [ ] **C3. G10 Pack packaging.** PackService, install plan preview, PacksPage. (Unblocked 2026-11-24: Apache-2.0 decided, Z5; implementation pending.)
- [ ] **C4. G13 Trees, effective dating, determination specificity, UoM.**
  - Slice 1 merged 2026-11-23 (d54d6e4): migration 18 `trees_effective_uom` + ADR-0029 (Proposed). Open: slices 2–6 (attributes, UoM, Property 25, CLI + Desk, docs + checkpoint).
  - Slice 2 merged 2026-11-23 (919bab4, train1): tree + effective-date Record Type attributes (path/depth/cycle/range, 12 files incl. protos, additive only). Review PASS (4 follow-ups). Open: slices 3 (UoM, in review), 13.3b (ledger-save recompute, added per Stylus — ADR-0029 line 27 had no implementing slice), 4–6 (Property 25, CLI + Desk, docs + checkpoint).
  - Slice 3 merged 2026-11-24 (21e5973+bc9b1e3): UoM families + conversions (exact factors, overrides, cross-family rejection) + determination effective ranges (total as-of contract); 12dp factor-scale fix from review. Review PASS (5 follow-ups). Open: 13.3b (ledger-save recompute, committed, re-gating), 4–6 (Property 25, CLI + Desk, docs + checkpoint).
  - Slice 13.3b merged 2026-11-24 (8add95b, train3): ledger saves recompute tree path+depth in-tx with descendants on reparent (Stylus-added: ADR-0029 line 27 had no slice). Review PASS (5 follow-ups F1–F5).
  - Slice 4 merged 2026-11-24 (094610d): Property 25 (period ranges + DB UoM round trip, pure bound+exact; genuinely randomized, half-ulp bound verified). Review PASS (3 follow-ups). Open: slice 5 single (13.5–13.7 remainder: CLI tree+uom, Desk remainder, docs + checkpoint; briefed, L6 after F7s2 per Stylus).
- [ ] **C5. G7 Party ledger, settlements, realised FX.** "What customers owe" and "What we owe" in View vocabulary.
  - Slice 1 merged 2026-11-24 (f00fc58): migration 20 `party_ledger` (l_party, party items, settlement applications) + ADR-0031 (Proposed). Review PASS (1 wording follow-up). Open: slices 2–9 (settlement engine, properties, FX, RPCs/CLI/Desk, fixture, docs + checkpoint).
  - Slice 2 merged 2026-11-24 (929d3dd, 1 commit): settlement open items + line-level applications (locks, ceiling, unapplied cash) via allocate.go. Review PASS. Merge gate 1124s (17 Go ok, 646 web). Open: slice 3 (Property 11, in review), 4 (Z2-native FX + reversals, building), 5–9.
- [ ] **C6. G9 Balances, Query compiler, Reports v1, Report Runs.** ReportsPage with charts using `--chart-*` only.
- [ ] **C7. G11 Stock ledger, reservations, serial and batch, valuation.** (Z3 decided 2026-11-24: perpetual weighted average, FEFO, rentals tracked-not-consumed, no negative stock.)
  - Slice 1 merged 2026-11-24 (6c99d3a, 1 commit): migration 21 (10 stock tables) + ADR-0032. Review PASS (cache-provenance crux cleared: uncached server green + migration tests rerun). Merge gate: real 1042s run (17 Go ok, 632 web). Open: slice 2 (stock core, in review), 3–10, 11 lifecycle wiring (Stylus-added: seam dead until wired).
  - Slices 2–3 merged 2026-11-24 (3272765, 3-commit train): slice 2 stock core (Plan/Apply seam, weighted average, layers/bins, negative refusal, cancel mirrors) + 4-finding fix (cancel kinds, return locking, cancel tests, ordering) + slice 3 cost deltas (append-only, on-hand/sold split, watermarked rebuild). Both reviews PASS (re-review for the fix). Train gate 703s (18 Go ok, 646 web) after a contention red. Open: slice 4 (Property 21, REVIEW PASS, merges next), 5–11.
- [ ] **C8. G12 Remaining ledgers.**
- [ ] **C9. G14 Report engine, work queues, Activity timeline, Explain this movement.**
- [ ] **C10. G15 Integration surface.** Attachments reuse the B9 Image storage.

## D. Product surface

- [ ] **D1. Roles and what each user sees** (master writes, user approves)
  - `docs/design/pages-by-role.md`: every route, its audience (design-system §11), required permission, everyday name, icon and empty state, for root, owner, builder, manager, accountant, storekeeper, cashier, customer and visitor. The page registry (B2) is generated from it or tested against it.
- [ ] **D2. Process maps** (master writes, user approves)
  - `docs/design/processes.md`: each flow from `docs/revamp/Athena-Process-and-Event-Catalogue.md` and `Records-and-movements.md` as steps in everyday words: who does it, which screen, what the system records, what the user sees next, what can go wrong. Starts with: buy, receive, sell, take payment, return, count stock, close the month, invite a person, recover access.
- [x] **D3. Public pages and SEO** (Codex then Muse; ADR required: adds server-rendered public routes) COMPLETE 2026-11-24 (slices 1–5).
  - Slice 1 merged 2026-11-22 (e801e78): ADR-0030 (Proposed) + decision-log/README rows. Open: slices 2–5 (plumbing + Workspace home, commerce pages, catalogue pages + vitals, docs + checkpoint).
  - Slice 2 merged 2026-11-23 (dde91f4, train1 919bab4): public route group + Workspace home + sitemap/robots + Desk noindex. Round-1 PASS stood; re-confirm PASS (byte-identical after rebase). Open: slices 3–5.
  - Slice 3 merged 2026-11-23 (db0f85b, train2 2d9c17d): commerce pages (storefront/category/product, SEO + JSON-LD + hash-pinned images, allow-list projection, escaped by construction). Review PASS (6 follow-ups a–f). Open: slices 4–5 (catalogue + vitals, docs + checkpoint).
  - Slice 4 merged 2026-11-24 (2377cb7, train3): catalogue + transcript verification + 1200x630 fallback PNG + honest vitals (LCP 28–52 ms, CLS 0, INP unmeasured); formal Z10 verdict (trial only, nothing bundled). Review PASS (3 follow-ups). Open: slice 5 (docs + checkpoint).
  - Slice 5 merged 2026-11-24 (9a29fb7, docs-only): overview/README/log rows + checkpoint evidence (generate stable, full suite + 24/24 browser green on c0c34c8, Desk untouched). Review PASS. D3 complete.
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
- [ ] **E3. Docs and release:** README, getting-started, decision log, ADR statuses, CHANGELOG `0.1.0-beta`, annotated tag after the user approves. (Unblocked 2026-11-24: Apache-2.0 decided, Z5; implementation pending.)

## F. Night shift asks

- [x] **F1. View tables like the shadcn data table example.** Merged 2026-11-24 (6ecf06d, 3 commits 507ffb5/6b29ed1/6f1b1e3): footer count, right actions, explicit padding + left cells + select symmetry; browser 15/15 + 8/8 shots at 375/768/1280/1440 x light/dark; DISJOINT from 6.10 held. Review PASS. Note: skills unavailable in this environment, design system applied as authority.
- [ ] **F2. Type to filter.** Search filters from the first character (~150 ms debounce, server-side match over label + text fields), plain-words placeholder from the Record Type label ("Search diet concerns…"). Expression filtering moves behind "Advanced filter" using the F4 builder. View words from `glossary.ts`.
  - Slice 1 merged 2026-11-24 (57e04c4, 1 commit): server free-text match on ListRecords (label + text, paged, escaped, hidden-safe; empty = today). Review PASS. Merge gate 1151s (17 Go ok, 646 web). Open: slice 2 (Desk search + Records E2E, building; Find verb per §21, RecordsPage granted).
- [ ] **F3. Field settings that fit and make sense.** Field settings dialog becomes a side sheet (drawer on mobile), never exceeds viewport, scrolls inside. Sections: Basics open (label, type, required, default, help); "Rules for this field" + "Advanced" collapsed. Plain-word labels + one-line help, no `doc.` syntax; conditions use the F4 builder; existing-item references are selects (field keys, link/child Record Types, options source, permission levels); numeric child-field totals as picks.
- [ ] **F4. Rules and lists without code.** Condition builder ("When [record] [field ▾] [is/is not/is empty/is any of/more than …] [value/field ▾]", AND/OR groups, autocomplete, compiles to `internal/expr` and round-trips, "Show as expression" toggle). Used by Rules page (validation/guard/derivation without Starlark; Starlark stays for advanced), F3 conditions, F2 filters. Lists (saved queries, e.g. "RSVP yes but missing diet or parking"): live list, PDF export now, "Email them"/"Message them" queue via C2 outbox (provider pending → button says queued). Audience rule excludes "invited but not responded" from day-of sends. Research first (Airtable/Notion/HubSpot), design into `.agents/tasks/beta-f4/design.md` + ADR (Proposed) before code.
- [ ] **F5. Circle Space sample Workspace.** `athena new "Circle Space" --sample event-management` (+ Setup Wizard choice in D5 later) creates a full test company managed only through Athena. Pack "Event Management" (Events, Guests+RSVP, Catering, Vendors+Rentals, Inventory, Billing). Record Types: Client, Venue, Event (+ schedule child lines), Guest, Invitation, RSVP (response, plus-ones, dietary, allergies, parking, vehicle reg, consent), Dietary Requirement, Supplier, Caterer, Dish (+ recipe lines), Ingredient, Menu, Purchase Order, Goods Receipt, Stock Issue to Event, Rental Item, Rental Booking, Vendor, Vendor Booking, Seating Table, Seat Assignment, Parking Pass, Sales Invoice, Purchase Invoice, Payment (M-Pesa/bank/cash). Kenya ledger masters per Z1. Seed: 2 Nairobi events (corporate launch, wedding), ~120 guests (mixed RSVP/dietary), caterers, DJs, MCs, rentals, stock, invoices, payments. Reports: guest list by RSVP, dietary summary, ingredient needs vs stock + shortfall list, stock movements, event P&L, unpaid invoices. Prefilled F4 Rules (missing diet/parking, not responded, over-capacity, allergen clash). Public pages: signed-link RSVP page per invitation, event page with schedule for yes-RSVPs only. Applied via kernel save paths until C3 (no direct SQL), then converted to an installable Pack. Never seeded into the platform shell or other Workspaces.
- [x] **F6. Live ASCII Athena logo.** Merged 2026-11-24 (bcd2913, 4 commits incl. step-ms fix 9d841c8): generator + 40x8 frames, AsciiLogo draw-in on sign-in/first-run/404/offline, CLI tree banner; 32/32 shots eye-checked, 23/23 art vitest. Review PASS after one NEEDS_CHANGES (doc/code step-ms mismatch, fixed + regen-verified).
- [ ] **F7. CLI that helps.** Bare `athena` opens a menu (server, Workspaces, users/roles, Record Types, records, rules, ledger, backups, doctor, help) on the same commands; non-interactive unchanged. `athena doctor` (MariaDB, migrations, disk, ports, no runtime node), completions (zsh/bash/fish), consistent `--help` with examples. E2E script runs every documented command against a test DB; README and help agree.
- [ ] **F8. Ready to ship.** One `athena` binary (server+CLI+embedded Desk). GoReleaser (macOS/Linux, arm64/amd64, checksums, SBOM), `install.sh`, Homebrew tap (`athena-erp` unless `athena` free; tap owner OPEN), `athena version` (version/commit/date from flags), `athena setup` guided install. Names: Athena, Athena Core, Athena Desk, Athena Packs. No release published; tagging waits for the user.

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

- B9a: Delete next to Save in the editor header; editor title not an h1; field-row action icons cramped; Required checkbox alignment; dead `.records-layout` rule (style.css:178). Resolved 2026-11-23 by FU batch 1 (train2 2d9c17d): icons + dead rule fixed; other three verified already satisfied by B9b.
- B3: Rule Run sheet shows messages, not a diff (needs a RuleRun diff field on the server).
- B4: unset custom chart swatches show black; "Selected page" preview lacks the accent; Security section intro copy too playful. Resolved 2026-11-23 by FU batch 1 (train2 2d9c17d): Selected-page accent fixed; black swatches pre-handled and improved; Security copy already plain.
- Security RPC: MariaDB-only upsert; last-writer-wins on concurrent sets.
- B4b: accent dots render light primaries on dark pages (ink dot invisible); dark ramp washes toward white for near-white primaries; CLI usage still says `--preset ink|red` with no chart-palette flag; ramp-fallback/Custom-swatch ink dots and `text-[11px]` month labels in preview. Mostly resolved 2026-11-23 by FU batch 1 (train2 2d9c17d, 6 fixes); open: `--chart-palette` flag (needs flag wiring, was usage-text-only scope).
- B9b: stale "draft" comments in `useDeskRoute.ts`; one-way Change (no return to auto-derive key); duplicate-name Enter shows no inline error until blur. Resolved 2026-11-23 by FU batch 1 (train2 2d9c17d, 3 fixes).
- B2a slice 1: toolbar search text starts under the icon (`pl-9` loses to Input `px-3`); dead `.entries-grid`/`.entries-resizer` CSS; hardcoded Draft/Submitted/Cancelled in table (use glossary); clearing filters fires a duplicate list request; `pressTimer` not cleared on unmount; search width 448 px vs 480 px in §21. API gaps (server lacks them): totals, free-text search, sort params, bulk-delete RPC. Deferred to FU batch 2 (none half-built; `pressTimer` already cleared pre-batch). Batch 2 verified all six already fixed by b4791a3, no changes.
- B5 slice 1: `athena role` help omits `settings:*` permissions; `new --logo` validates content only after creating the Workspace; concurrent SetIdentity can silently overwrite (last-writer-wins, old generations kept); prune error surfaces after the save committed; favicons re-render per request; `identity set` with no flags writes an identical generation.
- B9 form 1: alt-text error renders twice (inner + outer); required-but-empty field in a hidden section blocks save invisibly; empty ImageField label `for` points nowhere; section list uses filtered index as key/label; WorkspaceImage comment mentions lists (forms-only until 2.3); visibility evaluates at open only, not on draft edits.
- B5 slice 2: "No logo" empty text unreadable in mismatched swatch/theme combos; logo errors lack the CircleAlert icon pattern; clearing the display name toasts success but snaps back; theme-color static in dark theme (pre-existing); fallback favicon is the black mark in both themes.
- B2a slice 2: no bulk RPCs anywhere (tables correctly hide select); runs list hardcoded pageSize 50, next token ignored, no total; centered columns stay centered in stacked phone rows; mapping no-file rows focusable despite no-op click; Actions column shows when row-actions returns []; unexplained px-4 py-3 rewrite in DataTable.
- Notifications full list page (route, nav entry, permission and DataTable) needs design before it is built; the bell inbox covers design-system section 17 meanwhile.
- B10 slice 1: "Back to home" from a direct 404 leaves the bad address in the bar; async chunk carries all 33 figures (~120 KB, defeats tree-shaking); FigureName closed union needs extending for custom figures; custom Athena figure set still wanted (hairline-create skill unavailable). Custom illustrations accepted 2026-11-24 (B10; see F6 for the logo).
- B9 form 2: History time/author stale until reload (pre-existing shared code); focus falls to body when its control unmounts; drop-indicator divs inside ul; picker listbox roles without arrow-key nav; picker alt duplicates visible path; icon-only buttons 28 px wide (needs design-system decision); section cards keyed by index.
- B2a slice 3: current-company marker lost in the companies table (restore when DataTable supports marker rows); workspace CLI DSN lacks ParseTime so CLI period/date reads fail (Desk unaffected); no ledger bulk/delete RPCs.
- B8 slice 1: EnsureRoot taken-name skip branch untested; serve-boot seeding duplicates kernel-open call with only one seam covered; Desk isRootSelected exact-matching RESOLVED in slice 2 (isRootUsername trims + lowercases).
- B8 slice 2: Access Users Roles column shows "No roles" although assignments are live (pre-existing data behavior; column reads a source omitting assignments).
- C2 slice 1: consider adding `athena_key_versions` to the deferral absence-test list; subscription PK (consumer, event_type) allows one action per consumer+type (confirm when 8.2 lands); ADR-0027 "(no migration 17)" row is historically accurate, not a conflict.
- C1 slice 2: nil-Dialect fresh post still panics in audit.Append (doc says Dialect required; constructor/guard would be safer); replay conflict branch reachable only in a narrow race window, untested; tx.Commit failure returns raw with no log row; RequestID/Actor unvalidated (overlong values fail late with raw DB errors); slice-1 follow-ups 1–3 still open.
- C2 slice 2: dispatch fail-comment claims ExhaustSkip parks silently but code always records last_error and no path reads ExhaustBehavior; claim orders retried rows ahead of fresh pending rows; DispatchNext marks dispatched_noop when no handler is registered (confirm dropping vs parking); suite wall-time nears the 20m budget (auth/posting/server 4–6 min each).
- B11 slice 1: README Homes and Workspaces lacks a direct guide link; guide CLI errors omit the log timestamp prefix; e2e script slug/port fixed (no SLUG override for parallel runs).
- B11 slice 2: getting-started period command omits required `--label`; currency step repeats the stale-list class on setBase race; checkLabel/checkDisplayName count chars, server counts bytes; GetReadiness "today in open period" uses PeriodFor ignoring status; 375 px content passes under the fixed tab bar (shell-wide); gotoStep never updates furthest; first invited owner cannot open Setup (Instance Owner lacks build — confirm intended); skips persist in memory only.
- B10 slice 2: filtered Rules table + open runs tab can show two figures at once; empty-users/roles/sessions/runs figures unit-tested but never observed live.
- C1 slice 1: fitsLedger allows 28 significant digits vs DECIMAL(28,6) 22 integer digits (raw DB error on huge values); validDate accepts non-calendar dates; no multi-company balance test; hashPlan hashes Decimal text (100.00 vs 100 differ — normalize in slice 2); zero-value Engine Dialect panic risk on concurrent-insert race.
- C1 slice 3: same-op-id submit+cancel conflict untested (one-line TestReverseReplay addition); no concurrent-reversal test (racing reversals in Property 10 would close the loop); assertMirror does not pin bijectivity; checkIdentity wording on the reversal path; consider TypePostingReversed in the events package instead of the posting-local literal.
- C4 slice 1: backfill leftover-row error says "cycle or missing parent" but also fires for chains deeper than 100 passes (cosmetic; name the depth bound).
- C1 6.8: rules.proto posting comment now stale; Desk Rules page lacks the posting type (posting Rules are API-only); parent_id filter error branches untested.
- C1 6.9: plan legs lack voucher-line linkage; no test asserts preview leaves dry_run=1 rows only; `post log` first-page-only (no --page-token); hooks-don't-rewrite-doc assumption noted for the hash loop.
- FU batch 1 merged 2026-11-23 (train2 2d9c17d, 12 commits db6a7fd→425791b): B9a/B4/B9b fully resolved (fixed or verified-satisfied), B4b resolved except `--chart-palette` flag; B2a deferred to batch 2. Review PASS (3 follow-ups below).
- FU batch 2 merged 2026-11-24 (train4 bafe51d, 9 commits 77971a2→58a8cf4): B2a verify-only (all six already fixed, no changes), 6x B5, FU-1 wheel, C1 nit, +1 browser-found chart :value fix. Review PASS (editor timeouts proven load flake via controls; 1 follow-up below).
- FU-H merged 2026-11-24 (train4 bafe51d, 2 commits cb9a3c3+1e1450f): shared Node CDP browser driver at .agents/bin/browser + example + README; all lanes use it from here on. Review PASS (6 follow-ups below).
- C1 6.11: overview.md:101 log-write timing wording ("after rollback" implies post-rollback; code writes on a separate connection during Post).
- C4 slice 2: tree depth/path-length guard with friendly error (VARCHAR(2048) raw DB error today); enable-effective plan text should name the unique-index drop; `TestEnableEffectiveRefusesSharedEntities` misnomer (tests empty-table success); disable-effective path untested (duplicate-history refusal, confirmation gate).
- D3 slice 2: ADR-0030 "RPC paths are POST-only" wording is imprecise for Connect v2 GET (reword to unlinked + robots disallow); Workspace logo used verbatim as og:image incl. SVG (raster note for slice 4).
- D3 slice 3: (a) adversarial escaping test for commerce names (proven live, unguarded); (b) ImageWidth/Height int fields shadow embedded string fields (fragile); (c) corrupt image JSON / image read errors 500 instead of degrading to no-image/404; (d) published-but-empty categories 200 + sitemap-listed (intended?); (e) CoursesURL placeholder for slice 4; (f) slice-2 notes (a)/(c) still stand.
- FU batch 1: Accent-row indicator under Custom with nothing picked (consider wheel); harden browser theme-menu predicate to the radio group (flaked 1 in 4); capture wheel/picker-ramp/Auto-link states in a future browser pass.
- C4 slice 3: posting must pass the voucher's business date (not today) when adopting ResolveAccount; as-of not yet on RPC/CLI (slice 5 owns CLI/Desk); trim-before-validate consistency; pin the two-leg conversion bound in Property 25; FromBase-with-override + g→mg test gaps.
- C4 slice 4: pin the composed two-leg bound formula; fix ledgers_test comment overclaiming period proof of record dating; rapid bound-redraw nit.
- C5 slice 1: ADR-0031 "mirrors negate by convention" → "mirrors swap sides" (engine swaps sides, amounts never negative).
- C2 split: no `athena workflows` CLI; no Desk Workflows page; row-level exhaust (pre-existing); light-shot caveat note; harness wait + shot retake; silent unregistered-handler skips need a counter at G10.
- C4 13.3b: cross-level reparent test; reparent-to-root test; label-only save skips refresh (repair or ADR note); NULL-parent plain error → inputErrorf; hardcoded test revisions.
- D3 slice 4: filesWorkspace one-liner (take in slice 5); course image width/height parity; CLS re-measure on scroll if images move above the fold.
- C1 6.10: account_label server-side (proto change) + re-capture light sheet widths; strip engine "Posting:" prefix in View; 375 credit clip (scroll/stacked legs); reversed badge variant; re-capture reversed settled.
- FU batch 2: records-flow shots (records/dark/dialog-key) remain uncaptured; FU-H owns driver hardening.
- FU-H: CDP close handler + fetch timeouts (F1); stale example comment (F2); README wording (F3); --db collision note (F4); B11 owner echo (F5); SIGINT/SIGTERM handlers (F6).

## Z. User decisions (all Z0–Z13 decided; rows carry the dates)

| # | Question | Blocks |
| --- | --- | --- |
| Z0 | DECIDED 2026-11-21: both presets, chosen per Workspace | B4 |
| Z1 | DECIDED 2026-11-24: pilot Circle Space (event management, Kenya); KES, 16 % VAT (+ zero-rated/exempt), Jan–Dec FY, IFRS for SMEs chart; M-Pesa/bank/cash recorded only; KRA eTIMS out of beta (C10 follow-up) | D5 |
| Z2 | DECIDED 2026-11-24: open-periods-only posting, no day window; soft-close 10th (`ledger:close` adjustments with reason); hard-close after the VAT return (20th, refuses all); reopen needs `ledger:close` + reason + later periods + audit; corrections as current-period reversals; one statutory KES book + named books; ADR-0023 amended (O-011/O-012) | C1 |
| Z3 | DECIDED 2026-11-24: perpetual weighted-average valuation for all stock (IAS 2.25); FEFO picking for perishables; rentals tracked-not-consumed; negative stock refused. ADR-0032 (C7-owned) | C7 |
| Z4 | DECIDED 2026-11-24: retention 7 yr ledger/invoice/stock/audit (no beta auto-delete); guest data anonymised 90 d post-event + dietary consent; Access Log 1 yr; outbox/events 30 d; job runs 90 d; Workspace key from env/OS keychain, owner-held; `athena secrets rotate`. ADR-0033 (C2-owned) | C2 |
| Z5 | DECIDED 2026-11-24: Apache-2.0 (LICENSE, NOTICE, README section); C3 + E3 unblocked | C3, E3 |
| Z6 | DECIDED 2026-11-24: `vite-plugin-pwa`; single server on loopback; nightly `mariadb-dump`, 4 h restore / 24 h data-loss targets | D4 |
| Z7 | DECIDED 2026-11-24: first industry Pack is Event Management (Circle Space) | D7 |
| Z8 | DECIDED 2026-11-21 ("start all of those things"): B7 defaults as written | B7 |
| Z9 | DECIDED 2026-11-21: added `@lucasmarkes/hairline` 0.5.1 (MIT, pinned) and `golang.org/x/time` v0.16.0 (indirect until B7 imports `rate`) | B10, B7 |
| Z10 | DECIDED 2026-11-24: Satori on public pages too; licence checked 2026-11-24 (Personal-Use trial only) → no Satori file bundled until the user buys a web licence; interim system stack on public, Desk via `local()` only | B3, D3 |
| Z11 | DECIDED 2026-11-24: product name Athena (final) | B6 |
| Z12 | DECIDED 2026-11-21: committed on `feat/batch-2-g1-g6` (951ed0d, bf47716) with `.agents/` and `.kiro/` | everything |
| Z13 | DECIDED 2026-11-24: Workspace setting "Require a second approver for posting Rules", on by default when two or more users hold the approve permission | nothing for beta (C1 follow-up) |
