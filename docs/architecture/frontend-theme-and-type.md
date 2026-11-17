# Frontend theme and typography

**Last updated:** 2026-11-17  
**Status:** Shared semantic color tokens and the supplied typography scale are implemented.

## Palette and buttons

`web/src/theme.css` is the palette source for light and dark themes. It defines semantic surface, foreground, primary, secondary, accent, border, ring, destructive, and desk alias tokens. The small `@theme` mapping in `web/src/style.css` exposes those values to Tailwind utility classes; it does not define a second palette. Component and app CSS should use the semantic variables rather than literal color values.

The shadcn-vue Button component defines default, outline, secondary, ghost, destructive, and link variants using semantic color utilities. The global base styles must not force `color: inherit` onto buttons, because that overrides each variant's foreground and can make primary button text unreadable in dark mode. The current primary button pairs `--primary` with `--primary-foreground`; light and dark values are set in `theme.css`.

## Type scale

`web/src/typography.css` is the single source for typography values extracted from the supplied design specification. It defines Cal Sans display styles; Inter title, body, caption, button, and navigation styles; and JetBrains Mono code styles. The control desk uses display-sm for page headings, title-md/title-sm for section/card headings, body-sm for normal text, caption for dense metadata, and the specified button and nav styles. Display-xl through display-md remain available for future larger surfaces.

Only typography values were extracted from the supplied specification; its spacing, radius, and component layout sections were not applied through this typography file.

## Maintenance

- Change light/dark palette values in `theme.css`.
- Change type family, size, weight, line height, or tracking in `typography.css`.
- Keep button labels legible against each variant background in both themes.
- Keep the shadcn-vue component variant classes semantic so theme changes flow through tokens.
