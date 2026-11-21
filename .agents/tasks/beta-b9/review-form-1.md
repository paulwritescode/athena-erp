# Review: B9 form sub-slice 1, View renderer + Image field

Verdict: PASS

Branch `feat/b9-form-builder` at dcaef0f in `../athena-erp-b9form`, diffed against
`main` (`f7f4b5d`). Reviewed per `review-brief-form-1.md`, the sub-slice 1 text of
`form-brief.md`, `design.md` section 5, the design system (§4, §7, §13, §14),
`AGENTS.md` and `.kiro/steering/athena-design.md`. Read-only on product code; all
probes ran on a throwaway Workspace (`test_athena_b9form_r1`, fake HOME, port 3847).
No blockers. Follow-ups are numbered below; none stops the merge.

## What was verified

View renderer (`web/src/forms/layout.ts`, `RecordsPage.vue`, both read-only reviewed).
Sections with title/description, 2-column collapsing to 1 below 1024 px, field order,
half/full widths, help text wired via `aria-describedby`, static text blocks, static
image blocks (full `GetFile` original, 512 px proven in-page), and a quiet art
placeholder ("Artwork appears here"). Per-section visibility CEL is display-only:
evaluated once on open against the saved record, fail-open on any doubt, and a
`doc.on_show`-gated section hid for the `false` record and showed for the `true` one.
Unplaced fields append to the last section at full width (server and desk agree).
No `web/src/art` changes: the B10 registry was not built. On "inline type-check
errors" in the brief: the builder owns those in sub-slice 2; the server already
rejects a bad visibility CEL at layout save, so View can only meet valid ones and
staying visible on evaluation failure is correct here.

Image field (`ImageField.vue`, `WorkspaceImage.vue`, `images.ts`). Upload of a
512 px PNG rendered a 256 px thumbnail preview (naturalWidth read in-page, so the
`GetFile(thumbnail=true)` path is proven, not just called). Alt text is required:
saving without it blocks with an inline error; with alt text the record saves and
the value persists across reopen. Replace stores a new path (two distinct
`images/exhibit-photo-*.png` paths captured from `UpdateRecord` traffic) and keeps
the alt text; Remove clears to null and the null persists (confirmed via
`GetRecord`). The Build editor Fields tab lists the Image type (screenshot) and the
settings dialog hides unique/index/default/formula for it (code-verified).

Rules mirror. `field_rules.json` gains 14 image rows; the TS `validateImageValue` /
`isImagePath` mirror of `imageArgument` / `IsImagePath` was checked line by line
(path shape, `^[0-9a-f]{64}$` hash, trim rules, 1000-rune alt limit, required-null
handling, message wording) and both sides pass together. Layout round-trip tests
(`layout.test.ts`, 8 tests) and image helper tests are present.

Scope. `RecordsTable.vue`, `DataTable`, proto and `gen/` untouched; every
`RecordsPage.vue` hunk is editor-only (imports, editor state, `open`/`locked`/
planning/preview/`check`/`toValue`, editor form template). Layout logic lives in new
`web/src/forms/` files. No hex/`rgb()` in the diff; added `px` is hairline borders
plus the 1023 px tablet/desktop breakpoint from §3.1. No production Go changed.

## Commands observed (exit 0 unless noted)

- `CI=true pnpm --dir web run typecheck` — clean.
- `CI=true pnpm --dir web run test` — 38 files, 451 tests pass (incl. fieldRules
  parity, layout, images, token guard).
- `CI=true pnpm --dir web run build` — builds; `internal/webui/dist/.gitkeep`
  restored afterwards, `git status` clean.
- `gofmt -l cmd internal` — empty. `go build ./...`, `go vet ./...` — clean.
- `go test ./internal/server/ ./internal/files/ ./internal/store/` with MariaDB —
  all ok (server 139 s, files 49 s). `TestFieldRulesSharedTable`,
  `TestEveryProcedureHasAPolicy`, `TestOldVocabularyIsGoneOutsideLegacyFiles` pass.
- Browser matrix on the throwaway Workspace at 375/768/1280/1440 px in light and
  dark (12 screenshots in `/tmp/b9review/screens/`): single column at 375/768,
  two columns at 1280/1440, sections/blocks/visibility correct in both themes.
  Keyboard: Tab order Title, On show, Choose an image, Summary, Delete, Cancel,
  Save is logical; focus-visible ring confirmed by screenshot.

## Follow-ups (non-blocking)

1. Alt-text error renders twice: the same "needs alt text" message shows under the
   Alt text input and again under the action row (`ImageField.vue` passes the error
   to both the inner `TextField` and the outer `FieldShell`). Show it once.
2. A required-but-empty field inside a visibility-hidden section blocks save with no
   visible error: `submit()` skips locked and field-hidden values but not
   section-hidden ones. Narrow (new records never hide; values still submit), but
   align the two or surface the error when sub-slice 2 lands.
3. Empty `ImageField` has no `id="entry-<field>"` element, so the shell label's
   `for` points nowhere until an image is chosen. Give the empty state an
   anchor or drop the `for` there.
4. Section list uses the filtered index as `:key` and in the "Section N" fallback
   label; use the stable layout index so numbering and DOM reuse survive hiding.
5. `WorkspaceImage.vue` header comment says "in lists and forms" but sub-slice 1
   uses it in forms only (lists are post-B2a). Adjust the comment or leave for
   sub-slice 3.
6. Visibility is evaluated at open against saved values only; draft edits do not
   re-evaluate it. Fine as display-only for this slice; decide in the builder slice
   whether typing should toggle sections live.

## Provenance

Server: `../athena-erp-b9form` binary on `127.0.0.1:3847`, Workspace
`test_athena_b9form_r1` (database of the same name), seeded Record Type `exhibit`
with a 2-section layout plus an Image field. Screenshots and CDP scripts kept at
`/tmp/b9review/` (not committed). Dev database untouched; no other agent server
touched.
