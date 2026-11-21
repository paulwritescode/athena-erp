# Frontend theme and typography

**Last updated:** 2026-11-21  
**Status:** Shared semantic color tokens, chart series, status tokens, spacing and size tokens, motion tokens, accent presets and the supplied typography scale are implemented. No Settings UI chooses the accent yet (B4).

## Palette and buttons

`web/src/theme.css` is the palette source for light and dark themes. It defines semantic surface, foreground, primary, secondary, accent, border, ring, destructive, and desk alias tokens. The small `@theme` mapping in `web/src/style.css` exposes those values to Tailwind utility classes; it does not define a second palette. Component and app CSS should use the semantic variables rather than literal color values.

The shadcn-vue Button component defines default, outline, secondary, ghost, destructive, and link variants using semantic color utilities. The global base styles must not force `color: inherit` onto buttons, because that overrides each variant's foreground and can make primary button text unreadable in dark mode. The current primary button pairs `--primary` with `--primary-foreground`; light and dark values are set in `theme.css`.

## Chart series, status, spacing, sizes and motion

`theme.css` also carries the design-system §2.4 chart series (`--chart-1` to `--chart-5`, both themes), the §2.5 status tokens (`--success`, `--warning`, `--info`, each with a `-foreground` verified ≥ 4.5:1 on the solid color and a `-soft` 12% badge tint), the §3 spacing scale (`--space-1` to `--space-16`, 4 px base), control/icon/row sizes (`--size-*`, desktop and touch, including `--size-icon-touch`), and the §6 motion tokens (`--duration-fast/base/slow`, `--ease-out/in-out`). Spacing, sizes and motion are theme-invariant and live in `:root` only. The `@theme` map exposes the colors, the spacing scale, every size token under a distinct `--spacing-*` alias, easings under distinct `--ease-athena-*` aliases, and durations under distinct `--transition-duration-athena-*` aliases (verified: `duration-athena-fast` emits `transition-duration: var(--duration-fast)`). Same-name mappings such as `--ease-out: var(--ease-out)` are forbidden: they emit circular CSS that poisons the token (verified in the built bundle during review).

## Accent presets

Two presets ship as `[data-accent]` blocks on `<html>`, per design-system §2.3: `ink` (default, current values) and `red` (one warm primary on actions, with `--chart-1` and `--sidebar-primary` following it). Only the §2.1 accent tokens change between presets; `--ring` and `--sidebar-ring` keep their neutral values until specified. Dark `--sidebar-primary` is `var(--primary)`; every other existing color value is unchanged. Text on `--primary` passes ≥ 4.5:1 in both presets and themes (8.67/5.80 for red, verified at authoring; B4 re-checks with the Settings preview).

## Browser surfaces

Selection, caret, scrollbars and the focus ring come from tokens in the `style.css` base layer: a subtle primary selection tint, `caret-color: var(--primary)` in fields, thin neutral scrollbars, and a 2 px `--ring` focus outline. `prefers-reduced-motion` collapses transitions and animations.

## Token guard

`web/src/tokens.test.ts` fails the web suite if any `.vue` or `.css` file under `web/src` other than `web/src/theme.css` holds a hex literal, `rgb(` or named color in a value position: CSS declarations, `.vue` style blocks, `style`/`:style`/`v-bind` values, Tailwind arbitrary values and `fill`/`stroke`/`color` attributes. Comments, selectors, `data-*` attribute values, `transparent`, `currentColor` and `inherit` are allowed. Literal colors belong in `theme.css` only.

The check is structural since B2: `@vue/compiler-sfc` parses each `.vue` file into blocks and the template block into the real template AST, which is walked for paint attributes and bindings by exact name. Bare `v-bind` objects scan only style, class and paint keys; ternary conditions never count (only branch values reach paint); `url()` contents never count. String literals in `<script>` and `<script setup>` count as well, since a palette object reaches paint through `:style`; files with a legitimate reason list in `SCRIPT_EXCEPTIONS` (empty today). A brace/string-aware CSS scanner reads every declaration value in every rule, nested or not. The B1 regex limits are closed and covered by fixtures (backtick bindings, `data-*` keys inside `v-bind` objects, colors beside nested rules), as are conditional `v-bind` objects, `stop-color`, `title` strings, `url(#fragment)` references and script palette objects. `@vue/compiler-dom` and `postcss` are transitive-only, so expression and CSS scanning are purpose-built in the test file.

Remaining limits:
- a `//` inside a JavaScript regex literal reads as a comment start to the script scanner, so code after it on that line goes unscanned;
- a regex literal holding braces next to a `data-*` key in one `v-bind` object could mis-split the pair.
Neither shape appears in this tree; both are covered by staying out of it rather than by handling it.

## Shell breakpoints and overlays

B2 adapts the shadcn-vue sidebar to the R2 breakpoints: full sidebar at 1024 px and up, a 56 px icon rail from 640 to 1023, and the bottom navigation below 640 (`SIDEBAR_MOBILE_BREAKPOINT` 640, widths 15rem/3.5rem in `sidebar/utils.ts`, `min-[640px]:` visibility in `Sidebar.vue`). Sheet, dialog, drawer and alert-dialog overlays are a plain `bg-black/30` dim with no backdrop blur. Sheets carry `--space-4` side padding at their usages.


## Type scale

`web/src/typography.css` is the single source for typography values extracted from the supplied design specification. It defines Cal Sans display styles; Inter title, body, caption, button, and navigation styles; and JetBrains Mono code styles. The control desk uses display-sm for page headings, title-md/title-sm for section/card headings, body-sm for normal text, caption for dense metadata, and the specified button and nav styles. Display-xl through display-md remain available for future larger surfaces.

Only typography values were extracted from the supplied specification; its spacing, radius, and component layout sections were not applied through this typography file.

## Maintenance

- Change light/dark palette values in `theme.css`.
- Change type family, size, weight, line height, or tracking in `typography.css`.
- Keep button labels legible against each variant background in both themes.
- Keep the shadcn-vue component variant classes semantic so theme changes flow through tokens.
