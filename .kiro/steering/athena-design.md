---
inclusion: always
---

# Athena Desk design rules

The full system is `docs/design/athena-design-system.md`. Read it before any frontend change. These rules are the minimum and apply to every agent and every task.

- **Who for:** the everyday user (cashier, storekeeper, clerk), not developers or business analysts. A screen must be usable without a manual.
- **MUJI restraint:** neutral surfaces, generous space, one primary action per view, uniform placement of the same action on every page, ornament rarely.
- **Color:** only `--primary`, `--ring`, `--chart-*` and `--sidebar-primary` carry the accent. Header, sidebar, cards and backgrounds stay neutral; never a full-color or black header. Status uses status tokens with words. No hex, `rgb()` or named colors in components. Keep the existing values in `web/src/theme.css`.
- **Type:** Satori for h1 and h2 only; Inter for everything else; JetBrains Mono for codes and IDs. Use the tokens in `web/src/typography.css`; tabular numbers in tables.
- **Space and size:** 4 px base, spacing tokens only; controls 36 px desktop, 44 px touch; hairline borders instead of shadows, shadows only on floating layers.
- **Components:** shadcn-vue (Reka UI) primitives added with the CLI, TanStack Table for grids, Lucide icons with words. Every state: rest, hover, focus-visible, pressed, disabled with reason, loading, error, empty.
- **Responsive:** mobile < 640 (bottom nav, dialogs become drawers, tables become stacked rows), tablet 640 to 1023 (icon rail), desktop ≥ 1024 (240 px sidebar). Check at 375, 768, 1280, 1440 in both themes.
- **Motion:** 120 / 180 / 240 ms, ease-out to enter, transform and opacity only, honor reduced motion. Follow `~/.kiro/skills/emilkowalski-motion/SKILL.md`.
- **Hierarchy pass:** follow `~/.kiro/skills/impeccable/SKILL.md` (Operate mode for the Desk, Persuade for public pages).
- **Words:** View mode uses everyday words from `web/src/copy/glossary.ts`; builder words (Record Type, Pack, Rule, Grant) stay in Build mode.
- **Feedback:** Sonner toasts bottom right for results; durable state on the page.
- **Ornament:** hairline figures and ASCII only through `web/src/art/registry.ts`, at most one per screen, only on sign-in, first-run, empty, 404 and offline screens.
- **Logo:** the Workspace logo and name show in the header, sign-in, tab title and favicon; the Athena mark is the fallback.
