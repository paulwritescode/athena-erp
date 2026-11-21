# Requirements: Athena beta

**Status:** Draft for user approval, 2026-11-21.
**Scope:** everything needed for a working beta of Athena as defined in `tasks.md` "Beta definition". Kernel requirements G6 to G18 stay in `.kiro/specs/athena-batch-2/requirements.md` (sections 6 to 18) and are referenced, not repeated. This file adds the design, identity, session, form, public-page and product requirements, numbered R1 to R14 to match task groups B1 to E3.
**Users:** the everyday user (cashier, storekeeper, clerk, teacher, farm hand), a manager or approver, an owner, a builder, and the root administrator. Visitors reach public pages only.

## Glossary

- **Desk:** the signed-in Vue application, Build and View modes.
- **Everyday words:** the labels in `web/src/copy/glossary.ts` that View mode shows instead of builder words.
- **Preset:** a named accent (`ink` or `red`) applied with `data-accent` on `<html>`.
- **Art registry:** `web/src/art/registry.ts`, the only source of hairline figures and ASCII art.
- **Page registry:** `web/src/pages/registry.ts`, the only list of Desk pages and their navigation.
- **Public page:** a server-rendered page served to visitors without sign-in.

## R1 Design tokens (task B1)

1. THE Desk SHALL define color, status, chart, spacing, size, radius and motion tokens in `web/src/theme.css` for light and dark themes, keeping every color value that existed on 2026-11-21.
2. THE Desk SHALL let only `--primary`, `--primary-foreground`, `--ring`, `--chart-1` to `--chart-5`, `--sidebar-primary` and `--sidebar-ring` vary between Presets.
3. WHEN a `.vue` or `.css` file under `web/src` other than `theme.css` contains a hex, `rgb()` or named color, THE web test suite SHALL fail.
4. THE Desk SHALL theme text selection, caret, scrollbars and the focus ring from tokens.

## R2 Shell and components (task B2)

1. THE Desk SHALL show a header with a neutral background and a hairline bottom border, containing the Workspace logo and name, search, notifications, theme switch and user menu, and SHALL NOT use a filled color or black header.
2. THE Desk SHALL show navigation as a 240 px sidebar at 1024 px and wider, a 56 px icon rail from 640 to 1023 px, and a bottom navigation with at most four destinations plus More below 640 px.
3. THE Desk SHALL build every page and its navigation entry from the Page registry, and a page whose permission the user lacks SHALL NOT appear.
4. EVERY interactive component SHALL have rest, hover, focus-visible, pressed, disabled, loading and error states, and a disabled action SHALL show its reason when the user may learn it.
5. THE Desk SHALL provide a component gallery in Build mode that shows every component in every state.
6. WHEN the viewport is below 640 px, dialogs SHALL open as bottom drawers and data tables SHALL render as stacked rows showing two or three key fields.

## R3 Typography and icons (task B3)

1. THE Desk SHALL use Satori only for h1 and h2, Inter for all other text and JetBrains Mono for codes, IDs and expressions.
2. THE Desk SHALL show numbers in tables with tabular figures, right-aligned.
3. THE Desk SHALL resolve icons only through `web/src/icons.ts` with a fallback icon, and every icon-only control SHALL have an accessible name.

## R4 Accent presets and charts (task B4)

1. THE Workspace SHALL store one Preset, `ink` by default, changeable in Settings by a user with the settings permission, with a live preview.
2. WHEN a Preset is applied, text on `--primary` SHALL have a contrast ratio of at least 4.5:1 in both themes.

## R5 Workspace identity and logo (task B5)

1. THE Workspace SHALL store a display name and an optional logo (SVG or PNG, at most 512 KB), settable from the CLI (`athena new --logo`, `athena workspace set --name --logo`) and the Desk.
2. THE Desk SHALL show the Workspace logo and name in the header, the sign-in page, the browser tab title and the favicon, and on printed documents and public pages.
3. WHERE a Workspace has no logo, THE Desk SHALL show the Athena mark: `athena-black-logo` on the light theme and `athena-white-logo` on the dark theme.
4. THE server SHALL serve the logo same-origin at a content-hashed URL and SHALL reject a file that is not a valid SVG or PNG. SVG SHALL be sanitized (no scripts, no external references).

## R6 Everyday words and naming (task B6)

1. View mode and task screens SHALL show labels and status names only from `web/src/copy/glossary.ts`, each with a one-line plain explanation reachable from a help icon.
2. THE project SHALL keep one naming registry, `docs/design/naming.md`, mapping each product name to its CLI command, proto service, Go package and UI label. A rename of a code identifier SHALL require an ADR and a compatibility alias.

## R7 Sessions, rate limits and tabs (task B7)

1. THE Workspace SHALL store idle and absolute session lifetimes within bounds (idle 5 to 480 min, absolute 1 to 168 h), with defaults of idle 30 min for everyday users, idle 15 min for Build mode and root, and absolute 12 h.
2. WHERE the user chooses "Keep me signed in on this device" and holds no build permission, THE absolute lifetime SHALL be 7 days.
3. WHEN root or a Build-mode user starts a destructive action after 15 minutes without entering their password, THE Desk SHALL ask for the password again before the action runs.
4. THE server SHALL issue a new session ID at sign-in, on a permission change and on step-up, and SHALL send `Clear-Site-Data` on sign-out and password change.
5. THE server SHALL limit `Login`, `CompleteChallenge` and `StartReset` per IP (default 10 per minute) and per username (default 5 per 15 minutes), limit write procedures per session (default 60 per minute) and limit Report and export procedures per session, answering `ResourceExhausted` with a retry time. Limits SHALL be Workspace settings and denials SHALL be written to the Access Log without request values.
6. WHEN a session ends, the Workspace changes or the user's permissions change in one browser tab, ALL tabs of that browser SHALL update within two seconds.
7. WHEN a session has two minutes left before idle expiry, THE Desk SHALL warn the user with a "Stay signed in" action. Activity in any tab SHALL count as activity.

## R8 Root administrator and Build/View (task B8)

1. THE root administrator SHALL NOT be suspendable, deletable, renamable, or have Roles or permissions changed through the API, CLI or Desk. Each attempt SHALL be refused and logged.
2. THE Desk SHALL mark the root account with a caption and a lock badge, with its row actions disabled and the reason shown.
3. THE Desk SHALL switch Build and View with one segmented control shown only to users who can build. Build SHALL show a persistent "Building" caption. View SHALL show no Build navigation.
4. WHERE a user can build, THE Desk SHALL offer "Preview as" a chosen Role, read-only.

## R9 Form builder (task B9)

1. A Record Type SHALL carry a form layout: sections, one or two columns, field order and width, help text, conditional visibility as a CEL expression, and static text and image blocks.
2. THE Record Type SHALL support an Image field stored as Workspace files, with alt text required and thumbnails generated by the server.
3. THE Desk SHALL let a builder rearrange the layout by drag and drop and by keyboard (move up, move down, move to section), with a live preview at mobile and desktop widths.
4. A layout change SHALL save without changing the Record Type's table, through the journaled publisher, and SHALL be versioned with the Record Type revision.
5. View mode SHALL render forms from the layout. Without a layout it SHALL render fields in definition order in one column.

## R10 Ornament (task B10)

1. THE Desk SHALL render hairline figures and ASCII art only through the Art registry, and a page SHALL show at most one.
2. Figures SHALL appear only on sign-in, first-run and setup, empty states, the not-found page and the offline page, at the sizes in design-system §9, and SHALL rest under reduced motion.
3. THE CLI SHALL print the ASCII mark in `athena version`, `athena init` and `athena new` when output is a terminal, and SHALL NOT print it with `--json` or when piped.

## R11 Install and first run (task B11)

1. THE path install → `athena init` → `athena new "Name" [--logo]` → `athena run` → sign in as root → Setup → readiness SHALL work from a clean machine with MariaDB, and each step SHALL have a plain-language error for its failure cases.
2. THE root administrator SHALL be created with the database password at `athena new` or first boot (ADR-0018), and THE Desk SHALL ask root to change it on first sign-in.
3. `docs/guides/getting-started.md` SHALL describe exactly the tested path.

## R12 Roles, pages and processes (task D1, D2)

1. `docs/design/pages-by-role.md` SHALL list every route with its audience, permission, everyday name, icon and empty state, and a test SHALL fail when the Page registry and this list disagree.
2. `docs/design/processes.md` SHALL describe each beta process step by step in everyday words: who acts, on which screen, what is recorded, what the user sees next and what can go wrong.

## R13 Public pages and SEO (task D3)

1. THE server SHALL render public pages as HTML from approved public projections, without exposing `RecordService` to visitors.
2. EACH public page SHALL have a unique title and description, a canonical URL, Open Graph and Twitter tags with a 1200x630 image, and JSON-LD for its entity type.
3. EACH Workspace with public pages SHALL serve `sitemap.xml` and `robots.txt`. Carts, checkout, order tracking, verification results, sign-in and every Desk route SHALL send `noindex`.
4. Public pages SHALL meet LCP ≤ 2.5 s, INP ≤ 200 ms and CLS ≤ 0.1 at the 75th percentile in a lab run on a mid-range mobile profile.

## R14 Pack component kit (task D6)

1. THE Desk SHALL provide, in the component gallery, task-screen components for POS basket and tender, customer display, attendant queue, product card and listing table, cart and checkout, inbox thread, quiz runner, gradebook, count sheet, kitchen ticket and collection board, each with all states.
2. A Pack SHALL register a task screen through its manifest (route, audience, commands, Queries and durable states), following `docs/revamp/Athena-Checkout-Commerce-and-Learning.md` §7.

## Kernel and Pack requirements

Batch-2 requirements 6 to 18 apply unchanged to tasks C1 to C10, D4, D5 and D7, with the migration and ADR numbers assigned at dispatch (tasks.md "Conventions").

## Non-functional

- Accessibility: WCAG 2.2 AA for every Desk and public page.
- Security: no password, Challenge value, session token, CSRF token, secret or Record value in logs, spans or responses. Server stays on loopback until TLS and deployment are decided.
- Performance: Desk first load under 2 s on the local server. Lists page on the server. No full-page spinner after first load.
- Compatibility: no applied migration is edited, and no public proto field is removed or renumbered.
