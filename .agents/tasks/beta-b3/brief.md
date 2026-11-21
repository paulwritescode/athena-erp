# Brief: B3 and B3a, the Desk experience (Muse t4)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b3-experience, branch feat/b3-experience. Work only there. Scope: web/** and docs/architecture/frontend-theme-and-type.md. No Go or proto (Codex is doing B4 and B12 servers and B7 in parallel; their Desk parts come to you later).
Read first: AGENTS.md; .kiro/steering/athena-design.md; docs/design/athena-design-system.md (all of it, especially new sections 13 to 16); .kiro/specs/athena-beta/requirements.md R2 and R3; tasks.md B3 (including the B2 carry-overs) and B3a. Use the impeccable skill (Operate mode: layout, typeset, polish) and emilkowalski-motion.

## What the user asked for (2026-11-21, in their words, summarized)
- The phone navigation bar floats (section 13.1).
- On phones and tablets, every form and popover is a bottom drawer as tall as its content, swipe to close (13.2), and the whole logic works through drawers so it feels like a native app.
- Tables and charts are minified to fit phones (13.4, 13.5).
- Swipe on the screen to go back and forward (13.3).
- The desktop sidebar is arranged and grouped by hierarchy (section 15).
- The Build/View control and its container have square corners while the rest of the app is rounded: fix it, and every container.
- In Access, clicking a user opens a sheet from the top of the browser viewport to the bottom, not a card constrained inside the page (section 16). Today it is an inline card beside the table.
- Action buttons (Edit, Revoke, Reset, Suspend, Reactivate, End session, Delete and the rest) need their own look by kind, a proper icon, a hover behavior that shows it is a button, and clear spacing (section 14).
- "Take it to the next level and make sure it works nicely and is amazing": a page-by-page polish pass.

## Order (send a progress report after each step)
1. B3: typography roles, `web/src/icons.ts` registry, tabular numbers, and the B2 carry-overs listed under B3 in tasks.md.
2. ActionButton and ActionGroup (section 14) plus the full-height DetailSheet (section 16). Apply both on Access first (users, roles, sessions), then on every page.
3. Sidebar groups and radius consistency (section 15).
4. Native mobile and tablet layer (section 13): floating nav, drawer-everything (one `ResponsiveDialog` wrapper that is a dialog at 1024 px and wider and a vaul bottom drawer below; replace direct dialog use), swipe navigation, minified tables and charts, pull to refresh, manifest.
5. Page pass: Overview, Record Types, Records, Rules, Files, Access, Ledger, Sign-in.

## Checks and evidence
- typecheck, test, build pass. Add Vitest for pure logic: swipe gesture thresholds, drawer dirty guard, icon registry fallback.
- Browser pass with a throwaway Workspace only (as in B2), never the dev database. Shots for every page at 375, 768, 1280 and 1440 in light and dark into .cache/screens/b3-*. Include drawer-open and swipe mid-gesture shots on 375. Use touch emulation for gestures.
- A scripted check that no dialog renders centered below 1024 px, that nothing overflows horizontally at 375, 640, 768 and 1023, and that every action button has an icon and an accessible name.
- Do not commit, merge, push, start agents or touch other worktrees. Report each step with `hc send t6`.
