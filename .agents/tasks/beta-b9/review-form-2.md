# Review: B9 form sub-slice 2, layout builder

Verdict: PASS

Branch `feat/b9-form-builder` at 698f20f in `../athena-erp-b9form`, sub-slice-2 diff
`4dca9c0...HEAD` only (sub-slice 1 already merged; not re-reviewed). Reviewed per
`review-brief-form-2.md`, the sub-slice 2 text of `form-brief.md`, `design.md`
section 5, the design system (§4, §7, §13, §14, §19), `AGENTS.md` and
`.kiro/steering/athena-design.md`. Read-only on product code; all probes ran on a
throwaway Workspace (`test_athena_b9form_r2`, fake HOME, port 3847). No blockers.
Follow-ups are numbered below; none stops the merge.

## What was verified

Builder (`web/src/forms/FormBuilder.vue`, `editing.ts`, `LayoutItemSheet.vue`,
`FormPreview.vue`, `ImagePickerDialog.vue`, `ArtPlaceholder.vue`). In a live
browser: arrange-7-fields from the empty state; section add/rename (`Basics`,
`Details`)/reorder; 1|2 column toggle; item drag within a section and across
sections by real mouse (native drop lands, order changes, "moved in Section 2"
announced); section reorder, item up/down, and move-to-section by keyboard only
(Space/arrows, announcements, focus retention); field sheet width (half shows in
row caption and preview) and help text (live in preview); section sheet title,
description, columns, and visibility CEL — `doc.on_show` clean, `doc.nope +`
shows the server type-check error inline; text block with content; image block
via the picker over recorded `images/` files (thumbnail grid, current highlight,
upload path present); art block with free-text key and placeholder preview; no
`web/src/art` changes, so B10 was not built. Preview toggles phone (375 px
frame, measured) and desktop and renders every field disabled with the
visibility caption. Coarse-pointer handle hiding (`style.css`) and always-visible
move buttons are code-verified; headless reports a fine pointer, so handles
showing there is correct.

Saves and round-trip. Layout-only save through the existing `library.saveCurrent`
path: no migration/data-loss dialog, "Prop saved." toast, revision bumped, and
all 10 rows plus text/image/art blocks identical after reload. The View form for
a seeded record matches the built layout exactly (2 sections, order, widths,
help, text, image, art placeholder). History shows the fresh save after reload
(see follow-up 1 for the pre-reload staleness, which is pre-existing shared code).

Scope. Builder + Form tab + `forms/` only: no records-list, B2a, proto, `gen/`,
or Go production changes. `RecordsPage.vue` hunks only alias the shared span
rule and reuse `ArtPlaceholder`; `ImageField.vue` only moves upload helpers to
`images.ts` (same behavior, newly unit-tested). One primary action per surface;
no hex/`rgb()`; added `px` is hairline borders, the 1023/639 px breakpoints, the
375 px phone frame, and a 2 px drop indicator. Token guard green.

## Commands observed (exit 0 unless noted)

- `CI=true pnpm --dir web run typecheck` — clean.
- `CI=true pnpm --dir web run test` — 43 files, 494 tests pass (incl. new
  `editing.test.ts`, `FormBuilder.test.ts` keyboard-reorder tests, image-upload
  tests, token guard).
- `CI=true pnpm --dir web run build` — builds; `internal/webui/dist/.gitkeep`
  restored afterwards, `git status` clean.
- `gofmt -l cmd internal` — empty. `go build ./...`, `go vet ./...` — clean
  (no Go changes in this slice).
- Browser matrix on the throwaway Workspace at 375/768/1280/1440 px in light and
  dark (19 screenshots in `/tmp/b9review2/screens/`): builder stacks below
  1024 px, sheet opens as a vaul bottom drawer with grab handle on phones,
  focus ring and announcements verified, View matches at both extremes.

## Follow-ups (non-blocking)

1. History "Last saved / Changed by" is stale until reload: `commitSave`
   (`web/src/composables/useRecordTypes.ts:131`) refreshes only `revision`, so
   the History tab keeps the previous save's time and author. Pre-existing
   shared-code behavior affecting every save kind (out of this slice's scope),
   and the save itself is journaled (revision bump + reload verified). Refresh
   `updated_at`/`updated_by` from the save response.
2. Focus falls to `<body>` when its control unmounts: the Arrange button
   (EmptyState swaps to sections), section Remove after confirm, and item
   detach (x). Nudge/section-move focus handling is already good; extend the
   pattern — first section title after arrange, neighboring row/head after
   remove.
3. Drop-indicator `<div>`s sit directly inside `<ul class="builder-items">`
   (`web/src/forms/FormBuilder.vue:369,413`); `ul` may only hold `li`. Use an
   `li` or render indicators outside the list.
4. The picker grid claims `listbox`/`option` roles
   (`web/src/forms/ImagePickerDialog.vue:89-102`) but offers no arrow-key
   navigation (options are Tab-reachable buttons). Add roving tabindex or drop
   the roles.
5. Picker thumbnails use `alt=file.path` (`ImagePickerDialog.vue:100`) while the
   same path renders as text below; `alt=""` would avoid the duplication.
6. Icon-only `inline` row/section buttons are 28 px wide; the 44 px touch floor
   sets `min-height` only. Desk-wide `ActionButton` pattern, needs a
   design-system decision rather than a slice fix.
7. Section cards use `:key="sectionIndex"` (`FormBuilder.vue:335`). Benign with
   controlled inputs today, but a stable key would be safer for reorderable
   cards.

## Verification notes (method, not defects)

- CDP-synthesized Enter delivers `keydown` but no native `click` in headless
  Chrome (proven with per-event listeners: `keydown:Enter` arrives, `click`
  never fires), so button activation was verified via Space (same native
  path — moves, announcements, focus retention all observed), Tab reachability,
  Reka-menu keyboard (arrows+Enter, which handles `keydown` directly), unit
  tests, and the absence of any app `keydown` interception. Enter-to-click on a
  native `<button>` is platform-guaranteed.
- Section-card mouse drops are accepted by composition: native item drops are
  proven (within-section and cross-section, with events, order change, and
  announcements), `moveSection` is unit-tested, section keyboard reorder is
  proven in-browser, and `dropSectionAt`/`overSectionCard` plus the nested-
  draggable `stopPropagation` were reviewed line by line with no defect found.

## Provenance

Server: `../athena-erp-b9form` binary on `127.0.0.1:3847`, Workspace
`test_athena_b9form_r2` (database of the same name), seeded Record Types `prop`
(7 fields) and `tag` (3 fields) plus `images/prop-banner.png`. Scripts and
screenshots kept at `/tmp/b9review2/` (not committed). Dev database untouched;
no other agent server touched.
