# Frontend theme and typography

**Last updated:** 2026-11-22
**Status:** Shared semantic color tokens, chart series, status tokens, spacing and size tokens, motion tokens, accent presets and the supplied typography scale are implemented. Settings → Appearance (B4b) chooses the accent, the charts palette and the theme.

## Palette and buttons

`web/src/theme.css` is the palette source for light and dark themes. It defines semantic surface, foreground, primary, secondary, accent, border, ring, destructive, and desk alias tokens. The small `@theme` mapping in `web/src/style.css` exposes those values to Tailwind utility classes; it does not define a second palette. Component and app CSS should use the semantic variables rather than literal color values.

The shadcn-vue Button component defines default, outline, secondary, ghost, destructive, and link variants using semantic color utilities. The global base styles must not force `color: inherit` onto buttons, because that overrides each variant's foreground and can make primary button text unreadable in dark mode. The current primary button pairs `--primary` with `--primary-foreground`; light and dark values are set in `theme.css`.

The design-system §14 action system (`ActionButton`, `ActionGroup`) builds its six kinds on the default, outline and ghost variants plus token-colored outlines. The destructive kind is an outline at rest and fills `--destructive` on hover, with `--destructive-foreground` text (near-white on light, near-black on dark; 4.71/6.18 verified at authoring, so both themes pass 4.5:1). The §16 detail sheet (`DetailSheet`) is a full-height right sheet, 480 px wide or 640 px with `wide`, with a dim overlay and no backdrop blur.

## Chart series, status, spacing, sizes and motion

`theme.css` also carries the design-system §2.4 chart series (`--chart-1` to `--chart-5`, both themes), the §2.5 status tokens (`--success`, `--warning`, `--info`, each with a `-foreground` verified ≥ 4.5:1 on the solid color and a `-soft` 12% badge tint), the §3 spacing scale (`--space-1` to `--space-16`, 4 px base), control/icon/row sizes (`--size-*`, desktop and touch, including `--size-icon-touch`), and the §6 motion tokens (`--duration-fast/base/slow`, `--ease-out/in-out`). Spacing, sizes and motion are theme-invariant and live in `:root` only. The `@theme` map exposes the colors, the spacing scale, every size token under a distinct `--spacing-*` alias, easings under distinct `--ease-athena-*` aliases, and durations under distinct `--transition-duration-athena-*` aliases (verified: `duration-athena-fast` emits `transition-duration: var(--duration-fast)`). Same-name mappings such as `--ease-out: var(--ease-out)` are forbidden: they emit circular CSS that poisons the token (verified in the built bundle during review).

## Accent presets

Nine named presets ship as `[data-accent]` blocks on `<html>`, per design-system §2.3: `ink` (default, current values), `slate`, `blue`, `sky`, `green`, `amber`, `orange`, `brown` and `red` (one warm primary on actions, with `--chart-1` and `--sidebar-primary` following it), plus Custom. Only the §2.1 accent tokens change between presets; `--ring` and `--sidebar-ring` keep their neutral values until specified. Dark `--sidebar-primary` is `var(--primary)`; every other existing color value is unchanged. Text on `--primary` passes ≥ 4.5:1 in every preset and theme (8.67/5.80 for red, verified at authoring; a test parses `theme.css` and re-checks all presets, see below).

## Workspace appearance (B4b)

Settings → Appearance is a 280 px column of choices beside a realistic invoices preview in light and dark: Accent (nine named presets plus Custom, §18), Charts (Match accent — a tonal ramp — Neutral, Earth, Ocean, Forest, or Custom), and Your theme (Light, Dark, System, per user). Each choice opens a popover on desktop and a bottom drawer on touch. The server owns validation and contrast through `internal/appearance`'s one shared preset and palette list; on failure it returns a suggested accent the UI offers one click away, and `SetAppearance` rejects presets carrying colors.

Application lives in `web/src/appearance.ts`. Named presets set `data-accent` on `<html>` and named palettes set `data-charts`; custom writes one `#athena-appearance` `<style>` block with `:root` and `.dark` rules. Every preset is one `[data-accent]` block in `theme.css` (light and dark) and the ramp derives globally from `--primary` through `color-mix`, so custom accents ramp without extra rules. The picker's hex converts to the `oklch()` strings the server parses (`hexToOklch`, reference-checked in `appearance.test.ts`); a client mirror of the server's resolver derives the dark twin for the preview and warns before saving, but the server stays authoritative. A test parses `theme.css` and checks text contrast on every preset in both themes, plus the `.preview-light` scope that forces the light pane true inside a dark page.

The choice applies at load (the public `GetAppearance` call in `useSession`, so the sign-in page wears it) and caches per Workspace in `localStorage`. `web/public/paint.js` — a separate file because the Content-Security-Policy allows scripts from `'self'` only — applies the cache synchronously in `<head>`, so the first paint already wears the accent. The accent selectors omit `:root` so the preview can scope a preset to one pane; on `<html>` they behave exactly as before.

Light/dark/system is per user in `useTheme` (`localStorage`, operating-system default with a live listener), chosen in the user menu, on the Appearance page (provided by `App.vue`) and by the cycling header button. The preset, palette and fallback-hex names live in `appearance.ts`, not in the `.vue`, because the token guard below reads `red` as a color.

## Browser surfaces

Selection, caret, scrollbars and the focus ring come from tokens in the `style.css` base layer: a subtle primary selection tint, `caret-color: var(--primary)` in fields, thin neutral scrollbars, and a 2 px `--ring` focus outline. `prefers-reduced-motion` collapses transitions and animations.

## Token guard

`web/src/tokens.test.ts` fails the web suite if any `.vue` or `.css` file under `web/src` other than `web/src/theme.css` holds a hex literal, `rgb(` or named color in a value position: CSS declarations, `.vue` style blocks, `style`/`:style`/`v-bind` values, Tailwind arbitrary values and `fill`/`stroke`/`color` attributes. Comments, selectors, `data-*` attribute values, `transparent`, `currentColor` and `inherit` are allowed. Literal colors belong in `theme.css` only.

The check is structural since B2: `@vue/compiler-sfc` parses each `.vue` file into blocks and the template block into the real template AST, which is walked for paint attributes and bindings by exact name. Bare `v-bind` objects scan only style, class and paint keys; ternary conditions never count (only branch values reach paint); `url()` contents never count. String literals in `<script>` and `<script setup>` count as well, since a palette object reaches paint through `:style`; files with a legitimate reason list in `SCRIPT_EXCEPTIONS` (empty today). A brace/string-aware CSS scanner reads every declaration value in every rule, nested or not. The B1 regex limits are closed and covered by fixtures (backtick bindings, `data-*` keys inside `v-bind` objects, colors beside nested rules), as are conditional `v-bind` objects, `stop-color`, `title` strings, `url(#fragment)` references and script palette objects. `@vue/compiler-dom` and `postcss` are transitive-only, so expression and CSS scanning are purpose-built in the test file.

JavaScript first passes a lexer that blanks comments and regex literals (regex versus division by the previous significant token, with escapes and character classes), so neither confuses the scanners; the two B2 residual limits are closed with fixtures. There are no known limits.

## Shell breakpoints and overlays

B2 adapts the shadcn-vue sidebar to the R2 breakpoints: full sidebar at 1024 px and up, a 56 px icon rail from 640 to 1023, and the bottom navigation below 640 (`SIDEBAR_MOBILE_BREAKPOINT` 640, widths 15rem/3.5rem in `sidebar/utils.ts`, `min-[640px]:` visibility in `Sidebar.vue`). Sheet, dialog, drawer and alert-dialog overlays are a plain `bg-black/30` dim with no backdrop blur. Sheets carry `--space-4` side padding at their usages.


## Type scale

`web/src/typography.css` is the single source for typography values extracted from the supplied design specification. It defines Satori display styles (trial font, loaded from the local machine); Inter title, body, caption, button, and navigation styles; and JetBrains Mono code styles. The control desk uses display-sm for page headings, title-md/title-sm for section/card headings, body-sm for normal text, caption for dense metadata, and the specified button and nav styles. Display-xl through display-md remain available for future larger surfaces.

Only typography values were extracted from the supplied specification; its spacing, radius, and component layout sections were not applied through this typography file.

B3 applies the section 4 roles: headings balance, body copy renders pretty, table cells use tabular figures, and component titles carrying the h2 role (dialog, sheet, alert, drawer and card titles) resolve through `.cn-font-heading` to Satori title-md. Labels and table headers use the `text-caption` utility mapped from the caption tokens. Sign-in, empty states and phone form controls run at body-md. Icons come only from `web/src/icons.ts` (kebab-case names, `FileText` fallback) or `components/ui`; no other file imports `@lucide/vue`.

## Maintenance

- Change light/dark palette values in `theme.css`.
- Change type family, size, weight, line height, or tracking in `typography.css`.
- Keep button labels legible against each variant background in both themes.
- Keep the shadcn-vue component variant classes semantic so theme changes flow through tokens.
