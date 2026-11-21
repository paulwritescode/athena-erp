# Summary: B4b Appearance page redesign (merged fb603a6)

## What shipped

Settings → Appearance is now a left column of choices (Accent with nine
named presets — Ink default, Slate, Blue, Sky, Green, Amber, Orange, Brown,
Red — plus Custom with a color picker; Charts palettes incl. Match accent;
Your theme per user) beside one realistic preview card (invoice rows with
status badges, focused search, selected sidebar item, six-month bar chart)
in light and dark. Choices open in popovers on desktop, bottom drawers on
touch. Save enables only when changed; Reset restores defaults. The server
validates presets and palettes against one shared list; every preset passes
text contrast in both themes, enforced by a vitest that parses theme.css.

## History

- Worker w-b4b committed d91ad3b (922+/198- across server, proto, Desk).
- Review round 1 NEEDS_CHANGES: preview Match ramp showed the saved accent
  instead of the draft; pre-B4b custom chart saves rendered the ramp.
- Fix loop 1 (52557d0 → rebased fb603a6): ramp declared on scoped
  `[data-accent]` + inline vars for custom drafts; empty-palette + stored
  colors treated as custom in applyAppearance and paint.js; 2 regression
  tests. Re-review PASS with behavioral confirmation, no regressions.
- Rebased cleanly onto main 9260d04 (zero file overlap); master reran all
  checks green (go build/vet/gofmt, make test incl. 422 vitest, generate
  stable ×2, typecheck, build); merged --ff-only, pushed, ticked tasks.md.

## Follow-ups (in tasks.md)

Accent dots ignore dark page mode; dark ramp washes toward white; CLI usage
stale (`--preset ink|red`, no palette flag); preview nits (fallback dots,
`text-[11px]` month labels).

## Try it

Sign in → Settings → Appearance: pick Blue accent + Ocean charts, watch the
live preview in both themes, Save, reload — the whole Desk follows.
