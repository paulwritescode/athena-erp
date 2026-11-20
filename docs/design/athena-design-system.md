# Athena design system

**Status:** Proposed, 2026-11-21. Tokens marked *Proposed* are not in `web/src/theme.css` or `web/src/typography.css` yet; the beta task list (`.kiro/specs/athena-beta/tasks.md`, tasks B1 to B4) implements them. Existing color values and the Satori, Inter and JetBrains Mono stacks are kept as they are (D-010).
**Applies to:** every Desk page, every Pack task screen, every public page.
**Who we design for:** the person doing the work: a cashier, a storekeeper, a clerk, a teacher, a farm hand. Not the developer, not the product owner, not the business analyst. If a screen needs a manual, it is not finished.

## 1. Principles (MUJI, applied to software)

MUJI's art director Kenya Hara describes the brand through *emptiness*: a plain form that leaves room for the user to bring their own use to it, rather than a form that tells them what to feel ([MUJI, Kenya Hara](https://www.muji.com/us/flagship/huaihai755/archive/hara.html); [Ryohin Keikaku](https://www.ryohin-keikaku.jp/en/about-muji)). Athena takes these working rules from that:

1. **Emptiness is room for work.** Space is the default; content earns its place. A screen shows the one task in front of the user and what they need to finish it.
2. **Quiet surfaces, one voice of color.** Surfaces are neutral. Color appears only on the primary action and in charts. No colored headers, no black top bars, no tinted cards.
3. **Material honesty.** A control looks like what it does. A button is a button, a link is a link, a status is written in words as well as shown.
4. **One way to do one thing.** The same action looks and sits in the same place on every page. Uniformity is a feature.
5. **Explain, don't decorate.** Every field, status and disabled action can tell the user what it means and why, in plain words.
6. **Calm feedback.** The system answers every action at once and quietly: a toast, a state change, never a surprise.
7. **Restraint in ornament.** Hairline figures and ASCII art appear rarely and only where the user waits or starts (section 9).

Review question for every screen: *could a new cashier finish this without asking anyone?*

## 2. Color

### 2.1 Rule

Following shadcn-vue's theming model, the base palette is neutral (Zinc) and only these tokens may carry the accent hue: `--primary`, `--primary-foreground`, `--ring`, `--chart-1` to `--chart-5`, `--sidebar-primary`, `--sidebar-ring`. Everything else (background, card, popover, sidebar, header, muted, border, input) stays neutral.

Semantic status colors are separate and fixed: `--destructive`, and the *Proposed* `--success`, `--warning`, `--info`. They are used only for state (error, done, attention), always with text or an icon, never as decoration.

### 2.2 Kept values

The current light and dark values in `web/src/theme.css` stay unchanged as the neutral base. One existing value breaks the rule: dark `--sidebar-primary: oklch(0.488 0.243 264.376)` is blue. Task B4 sets it to `var(--primary)`.

### 2.3 Accent (decided 2026-11-21: both, chosen per Workspace)

Athena ships both accents as named presets, and each Workspace picks one in Settings (stored as a Workspace setting, applied as `data-accent="ink"` or `data-accent="red"` on `<html>`). Ink is the default for a new Workspace. Only the tokens in §2.1 change between presets.

| Preset | Light `--primary` | Dark `--primary` | Character |
| --- | --- | --- | --- |
| `ink` (default) | `oklch(0.21 0.006 285.885)` (Zinc 900) | `oklch(0.92 0.004 286.32)` | Ink on paper. No hue anywhere except charts. |
| `red` | `oklch(0.42 0.13 22)` (close to MUJI's brand red `#7f0019`, lifted for contrast) | `oklch(0.68 0.13 22)` | One warm signature color on actions only. |

The Workspace can override the accent in Settings (playbook §8), within contrast limits checked at save: text on `--primary` ≥ 4.5:1.

### 2.4 Chart series (*Proposed*)

Muted, earthy and distinguishable, ordered so series 1 and 2 differ in lightness as well as hue (readable in grayscale):

| Token | Light | Dark | Name |
| --- | --- | --- | --- |
| `--chart-1` | `oklch(0.45 0.03 260)` | `oklch(0.78 0.03 260)` | sumi (ink) |
| `--chart-2` | `oklch(0.62 0.09 55)` | `oklch(0.72 0.09 55)` | clay |
| `--chart-3` | `oklch(0.55 0.07 150)` | `oklch(0.70 0.07 150)` | moss |
| `--chart-4` | `oklch(0.50 0.08 240)` | `oklch(0.70 0.08 240)` | indigo |
| `--chart-5` | `oklch(0.75 0.06 85)` | `oklch(0.55 0.06 85)` | kinari (unbleached) |

With the `red` preset, `--chart-1` becomes the accent.

### 2.5 Status tokens (*Proposed*)

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--success` | `oklch(0.52 0.09 150)` | `oklch(0.72 0.09 150)` | Saved, posted, paid |
| `--warning` | `oklch(0.62 0.12 75)` | `oklch(0.78 0.12 75)` | Needs attention, draft, expiring |
| `--info` | `oklch(0.50 0.06 240)` | `oklch(0.72 0.06 240)` | Neutral notice |

Each has a `-foreground` and a `-soft` (`color-mix(in oklab, <token> 12%, var(--background))`) for badges.

### 2.6 Forbidden

Hex, `rgb()` or named colors in components. Full-color header, sidebar or card backgrounds. Gradients. Colored shadows or glows. Color as the only carrier of meaning.

## 3. Spacing, size and layout (*Proposed* tokens)

Base unit 4 px. Spacing tokens in `web/src/theme.css`:

| Token | px | Typical use |
| --- | --- | --- |
| `--space-1` | 4 | icon to label |
| `--space-2` | 8 | inside a control group, chip gap |
| `--space-3` | 12 | label to field, list row padding |
| `--space-4` | 16 | field to field, mobile page padding, card padding (mobile) |
| `--space-6` | 24 | card padding, tablet page padding, form section gap |
| `--space-8` | 32 | desktop page padding, between page sections |
| `--space-12` | 48 | above a page heading, empty-state padding |
| `--space-16` | 64 | sign-in and first-run vertical rhythm |

Rules: more space above a heading than below it. Related things are close, separate things are far. Never invent an in-between value; pick a token.

Sizes:

| Item | Desktop | Touch (mobile, tablet, POS) |
| --- | --- | --- |
| Control height (button, input, select) | 36 px | 44 px minimum target |
| Small control | 32 px | 40 px |
| Icon | 16 px inline, 20 px in nav | 20 px, 24 px in bottom nav |
| Table row | 40 px | 48 px |

Radius: keep `--radius: 0.625rem` for cards, dialogs and drawers; controls use `calc(var(--radius) - 2px)`; badges and chips are fully rounded. No other radii.

Borders over shadows: surfaces are separated by a 1 px `--border` hairline. Shadows only on things that float (popover, dropdown, dialog, drawer, toast), using `--app-shadow` with a real offset and blur.

Widths: reading text 65 to 72 ch. Forms 720 to 960 px with an optional 320 px right rail on desktop. Lists and tables use the full content width. Dialogs 440 px (confirm), 560 px (form), 800 px (wide).

### 3.1 Breakpoints

| Name | Width | Shell |
| --- | --- | --- |
| mobile | < 640 px | Top bar with logo and user menu; bottom navigation with up to 4 destinations plus More (sheet). Page padding 16. One column. Dialogs become bottom drawers. Tables become stacked rows with the 2 or 3 key fields. |
| tablet | 640 to 1023 px | Collapsible sidebar rail (56 px, icons with tooltips), expands over content. Page padding 24. Forms one column; right rail moves below. |
| desktop | 1024 to 1439 px | Fixed sidebar 240 px. Page padding 32. Forms with optional right rail. |
| wide | ≥ 1440 px | Same as desktop; content capped at 1280 px for forms and dashboards; tables may use full width. |

## 4. Typography

Fonts are kept: **Satori** for headings only, **Inter** for everything else, **JetBrains Mono** for codes, IDs and expressions. Satori is a trial font loaded from the local machine (`web/src/typography.css`); bundling it waits for web embedding rights (OPEN, owner: user).

| Role | Token | Font | Where |
| --- | --- | --- | --- |
| Page title (h1) | display-sm 28/1.2, 500 | Satori | One per page, top left of content |
| Section title (h2) | title-md 18/1.4, 400 | Satori | Card and form section headings |
| Group title (h3) | title-sm 16/1.4, 600 | Inter | Field groups, drawer sections |
| Body | body-sm 14/1.5 | Inter | All running text, field values |
| Large body | body-md 16/1.5 | Inter | Sign-in, empty states, mobile forms |
| Label | caption 13/1.4, 500 | Inter | Field labels, table headers |
| Supporting text | caption 13/1.4, 400, `--app-text-muted` | Inter | Help under a field, metadata |
| Numbers | body-sm with `font-variant-numeric: tabular-nums` | Inter | Amounts, quantities, dates in tables |
| Code | code 14/1.5 | JetBrains Mono | Record IDs, keys, Rule source |

Emphasis comes from size, weight and space, in that order; color is the last resort and only `--foreground` versus `--app-text-muted`. No italics for emphasis, no all-caps except short badges, no underline except links.

Wrapping: headings use `text-wrap: balance`; body uses `text-wrap: pretty`. Labels never truncate; values truncate with an ellipsis and a tooltip only in tables. Long words in values (emails, IDs) use `overflow-wrap: anywhere`. Numbers never wrap.

**Typography inside drawers and dialogs.** The title is title-md Satori; one line of supporting text below it in muted caption; then content in body-sm. Section breaks are h3 Inter 600 with `--space-6` above. The primary action sits bottom right, secondary to its left, destructive far left. The title says what will happen ("Invite a person", not "User").

## 5. Icons

Lucide only (`@lucide/vue`, pinned). Stroke width 1.5 (2 at 16 px and below). Sizes from section 3. An icon never stands alone without an accessible name; in navigation and on primary actions it pairs with a word.

| Place | Icon |
| --- | --- |
| Overview / Home | `house` |
| Record Types | `layout-template` |
| Records (View) | per Record Type, chosen from the safe icon registry; fallback `file-text` |
| Packs | `package` |
| Rules | `scroll-text` |
| Files | `folder` |
| Access (people and roles) | `users` |
| Ledger / Books | `book-open` |
| Reports | `chart-column` |
| Jobs and queues | `list-checks` |
| Settings | `settings` |
| Search / command | `search` |
| Notifications | `bell` |
| Create | `plus` |
| Edit | `pencil` |
| Delete | `trash-2` |
| More actions | `ellipsis` |
| Filter | `list-filter` |
| Sort | `arrow-up-down` |
| Close | `x` |
| Back | `arrow-left` |
| Help and explanation | `circle-help` |
| Success / Warning / Error / Info | `circle-check` / `triangle-alert` / `circle-alert` / `info` |
| Locked / permission | `lock` |
| Offline | `wifi-off` |
| Theme | `sun` / `moon` |
| Sign out | `log-out` |

The safe icon registry (`web/src/icons.ts`, task B3) maps names to imported components so a Record Type definition can name an icon without loading arbitrary code.

## 6. Motion

One motion language, after Emil Kowalski's guidance: short, purposeful, transform and opacity only, and reduced motion respected.

| Token (*Proposed*) | Value | Use |
| --- | --- | --- |
| `--duration-fast` | 120 ms | hover, press, toggle |
| `--duration-base` | 180 ms | popover, dropdown, tooltip, toast |
| `--duration-slow` | 240 ms | dialog, drawer, sheet, page content |
| `--ease-out` | `cubic-bezier(0.23, 1, 0.32, 1)` | anything entering |
| `--ease-in-out` | `cubic-bezier(0.77, 0, 0.175, 1)` | things moving on screen |

Rules: enter with ease-out, leave faster than you enter. Buttons scale to 0.97 on press. Popovers grow from their trigger (`transform-origin`). No animation on keyboard-driven repeated actions (typing, arrow keys in a table). No looping decoration. `prefers-reduced-motion: reduce` swaps movement for a plain fade or nothing.

## 7. Components

Sources: shadcn-vue (Reka UI) for every primitive, TanStack Table for data grids, Lucide for icons. Components missing today and needed: sidebar, sheet, drawer, tooltip, context-menu, skeleton, avatar, breadcrumb, scroll-area, radio-group, toggle-group, calendar and date picker, pagination, progress, accordion, collapsible, hover-card, form. They are added with the shadcn-vue CLI into `web/src/components/ui`, never hand-copied from elsewhere. App-level compositions live in `web/src/components/` with one file per component.

Every interactive component has these states, styled from tokens: rest, hover, focus-visible (2 px `--ring` outline, 2 px offset), active/pressed, disabled (50 % opacity, `not-allowed` cursor, and a reason on hover or focus where the user may learn it), loading (spinner or skeleton, label kept, width fixed), error.

| Component | Built on | Rules | Mobile / tablet / desktop |
| --- | --- | --- | --- |
| **App shell** | sidebar, header | Header is `--background` with a bottom hairline: logo and Workspace name left; search, notifications, theme, user menu right. Never a filled color bar. | bottom nav / rail / fixed sidebar (3.1) |
| **Sidebar** | shadcn sidebar | Groups with caption labels; items icon + word; selected item `--sidebar-accent` background and `--foreground` text, no colored fill; keyboard navigable; collapse keeps labels as tooltips. Build and View are two groups switched by the mode control, not two apps. | sheet from More / 56 px rail / 240 px |
| **Mode switch (Build / View)** | toggle-group | Two-option segmented control in the header, with words and icons (`hammer`, `eye`). Shown only to people who can build. The current mode is also shown as a quiet caption under the page title in Build ("Building"), so it is never ambiguous. | in user menu / header / header |
| **Page header** | layout | h1, one line of supporting text, primary action top right, secondary actions in a `ellipsis` menu. Breadcrumb above h1 when depth > 1. | actions collapse into one menu / same / same |
| **Button** | button | Variants: primary (one per view), secondary, outline, ghost, destructive, link. Label is a verb ("Save invoice"). Icon left of label. Loading keeps the width and disables the button. | full width in forms and drawers / auto / auto |
| **Input, textarea, select, combobox, date** | input, textarea, select, command, calendar | Label above, help below in muted caption, error below in destructive with `circle-alert`, replacing the help. Required is the default and unmarked; optional fields say "(optional)". Placeholder shows an example, never the label. | 44 px, native date and select where better / 40 / 36 |
| **Checkbox, radio, switch** | checkbox, radio-group, switch | Switch only for settings that apply at once; checkbox for choices saved with a form. Label clickable. | 44 px row / 40 / 32 |
| **Card** | card | Neutral `--card`, hairline border, `--space-6` padding (16 on mobile), no shadow, title h2. Cards group; they do not decorate. Never nest cards. | stacked / 2 columns / grid |
| **Data table** | TanStack Table + table | Header caption style, sticky; numbers right-aligned tabular; row hover `--app-surface-hover`; row click opens the Record; selection checkboxes left; row actions in `ellipsis` and context menu; server paging with page size choice; resizable columns (existing `athena-entry-columns:` key); empty, loading (skeleton rows), and error states inside the table frame. | stacked cards with 2 or 3 key fields / horizontal scroll with first column pinned / full |
| **Filters and search** | input-group, popover, badge | Search box first, filters as chips that show their value, "Clear all" as a link. | in a drawer / popover / popover |
| **Dialog** | dialog, alert-dialog | For short, focused decisions. Alert-dialog for anything destructive, which names the thing and the consequence ("Delete 3 Records. This cannot be undone."). Esc closes non-destructive dialogs; focus returns to the trigger. | bottom drawer / centered / centered |
| **Drawer and sheet** | drawer (vaul-vue), sheet | Sheet from the right for detail and editing alongside a list; drawer from the bottom on mobile. Width 480 (form) or 640 (detail). Header, scrolling body, fixed footer with actions. Dismissing never loses work: unsaved changes ask first. | full-height bottom drawer / right sheet / right sheet |
| **Floating panels** | popover, hover-card, tooltip | Tooltip for names of icon-only controls and for truncated text, 500 ms delay, never essential information. Popover for small pickers. Hover-card for previews of linked Records. | tap-to-open popover only / same / same |
| **Context menu** | context-menu, dropdown-menu | Same items as the row `ellipsis` menu, in the same order; destructive last, separated. | long-press / right-click / right-click |
| **User menu** | dropdown-menu + avatar | Avatar with initials, name, role caption; items: Profile, Change password, Sessions, Theme, Sign out. | sheet / dropdown / dropdown |
| **Command palette** | command | `⌘K` / `Ctrl K`; shows only readable Records, Reports and permitted actions. | full-screen sheet / dialog / dialog |
| **Tabs** | tabs | Underline style, words only, no more than 6; beyond that use a sidebar section. | scrollable / same / same |
| **Badge and status** | badge | Text always ("Draft", "Submitted · Partly paid"); tone from status tokens, soft background. | same |
| **Toast** | sonner | Bottom right (bottom center on mobile); one line plus optional action ("Undo", "View"); success 4 s, error stays until dismissed; never the only place a durable state is shown. | bottom center / bottom right / bottom right |
| **Empty state** | layout + art registry | Optional hairline figure 160 to 240 px, h2 that says what is missing, one sentence why, one primary action. | same, figure 120 px |
| **Loading** | skeleton, progress | Skeleton in the shape of the content after 200 ms; progress bar for jobs with a known total; never a full-page spinner after first load. | same |
| **Error states** | layout + alert | Inline field errors; an error summary at the top of long forms linking to each field; page-level error card with what happened, what to try, and the request reference (`X-Request-Id`) in mono caption; permission-denied card that names the missing permission in plain words and who to ask. | same |
| **Confirmation and conflict** | dialog | Conflict (stale revision) keeps the user's changes, shows what changed, offers Reload or Apply mine. | same |
| **Form layout** | form + form builder (B9) | Sections with h2, fields in 1 or 2 columns, logical order top to bottom; actions in a sticky footer on long forms. | 1 column / 1 column / 1 or 2 columns + rail |
| **Sign-in and first-run** | card + art registry | Centered column 400 px on `--background`, logo, h1, form, one quiet hairline or ASCII mark. | full width, padding 16 / same / same |

## 8. Words

Two vocabularies exist and must not mix:

- **Builder vocabulary** (ADR-0008): Record Type, Record, Pack, Workspace, Rule, Grant. Used in Build mode, the CLI, the API and the docs.
- **Everyday vocabulary:** what a user sees in View mode and in task screens. It names the thing in the user's own work ("Invoice", "Stock count", "Customer") and the action they take ("Confirm sale", not "Submit document"). Financial and technical terms get a plain label and a one-line explanation behind `circle-help`: for example "Posted" shows as "Recorded in the books"; "Period closed" as "This month is closed for changes". The glossary lives in `web/src/copy/glossary.ts` (task B6) and is the only source for these labels.

Copy rules: sentences, not title case. Controls name their action. Errors say what happened and what to do next. Never blame the user. Never show raw codes without a plain sentence first.

## 9. Ornament: hairline figures and ASCII art

Source: [Hairline](https://hairline.lucasmarkes.com/) (`@lucasmarkes/hairline`, MIT, SVG, no dependencies, vanilla API works in Vue). Adding it as a dependency needs approval (task B10).

Rules:

1. **Where:** sign-in, first-run and setup, empty states, the not-found and offline pages, and the installer's CLI banner (ASCII). Nowhere on data-dense pages, forms in progress, tables, dialogs or task screens such as the POS lane.
2. **How many:** at most one figure per screen.
3. **Size:** empty state 160 to 240 px wide (120 px on mobile); sign-in 200 to 280 px; never larger than the content it accompanies.
4. **Look:** line color from `--app-text-subtle`, theme follows the Desk (`theme: "auto"`), `intensity` 0.3 or less, `play` off. Reduced motion rests the figure.
5. **Meaning:** the figure matches the page (Sieve for "no results", Padlock for sign-in, Drawer for empty Files, Plot for empty Reports, Stack for empty Records). Decorative figures are `aria-hidden`; meaningful ones get a label.
6. **ASCII:** set in JetBrains Mono at caption size in `--app-text-subtle`, `aria-hidden`, at most 8 lines, used for the CLI banner, the sign-in footer mark and the 404 page.
7. **One place to change:** every use goes through `web/src/art/registry.ts` (placement key → figure or ASCII block, size, label). Pages never import a figure directly. Custom Athena figures made with the `hairline-create` skill live in `web/src/art/figures/` and register there.

## 10. Brand and Workspace logo

The Athena mark comes in two files at the repository root, `athena-black-logo` and `athena-white-logo` (PNG, 1254 x 1254, RGBA; added 2026-11-21 without a file extension). Use the black mark on the light theme and the white mark on the dark theme. Task B5 renames them to `.png`, makes web sizes (favicon 32 and 180 px, header 48 px, sign-in 96 px) and an SVG if one is supplied. It is the platform logo for the installer, the CLI banner, the default favicon and the sign-in page when a Workspace has no logo.

Each Workspace can set its own logo and name (task B5): stored as a Workspace file and referenced from Workspace settings, served same-origin, shown in the header, the sign-in page, the browser tab title and favicon, printed documents and public pages. Rules: SVG or PNG, up to 512 KB, shown at 24 px height in the header and 48 px on sign-in, never stretched, with the Workspace name as text beside or below it. The platform mark appears only as "Powered by Athena" in the footer once a Workspace logo is set.

## 11. Pages by audience

| Audience | Sees | Indexed by search engines |
| --- | --- | --- |
| Visitor (public) | Public pages a Pack publishes: storefront, product pages, collection and order tracking (by token), course catalogue, transcript verification. | Yes, except carts, checkout, tracking and verification results. Server-rendered HTML with title, description, canonical, Open Graph and JSON-LD (task B14). |
| Signed-out user | Sign-in, invite completion, password reset. | No (`noindex`). |
| Everyday user | View mode: their home with tasks and recent Records, the Record Types their Roles allow, their Reports, their profile. | No. |
| Manager / approver | The above plus approval queues, team Reports, assignment of work. | No. |
| Builder | Build mode: Record Types, Rules, Packs, Files, Access, Ledger setup, Settings. | No. |
| Root administrator | Everything, always (ADR-0018). Cannot be changed or suspended. | No. |

## 12. Checks before a screen is done

- Opened in a browser at 375, 768, 1280 and 1440 px, light and dark.
- Keyboard only: every action reachable, focus visible, order logical, Esc and Enter behave.
- Contrast: body ≥ 4.5:1, large text ≥ 3:1, focus ring visible on every surface.
- All states seen: empty, loading, error, permission denied, long content, disabled with reason.
- No hard-coded color, spacing or font value (grep for `#`, `rgb(`, `px` outside tokens).
- Copy read aloud: does a new user understand it?
