# Summary: B9 form sub-slice 1, View renderer + Image field (merged 99cc6ed)

## What shipped

The View record form renders from the layout: sections with title and
description, 1|2 columns (1 below 1024 px), field order, half/full widths,
help text via aria-describedby, static text and image blocks (full GetFile
originals), a quiet art placeholder ("Artwork appears here", no B10 registry
built), and display-only per-section visibility CEL (fail-open, agreed with
the server on unplaced-field appending). Image field end to end: upload with
256 px thumbnail preview, required alt text with inline error, display,
replace (new path, alt kept), remove (null persists). The Build Fields tab
lists the Image type with inapplicable attributes hidden. Image rules are
mirrored in fieldRules.ts + the shared field_rules.json table (14 rows),
both sides green together. Editor-only: no list changes (RecordsTable,
DataTable, columns untouched).

## History

- Worker w-b9form committed dcaef0f (17 files; renderer in new web/src/
  forms/). Master ordered it off RecordsTable.vue mid-slice (B2a owns the
  list); list thumbnails deferred to follow-up 2.3.
- Review round 1 PASS: behavior verified end to end over RPC (thumbnail
  naturalWidth, alt gating, replace/remove persistence), 451 web tests,
  browser matrix 375/768/1280/1440 × light/dark with keyboard pass, 6
  non-blocking follow-ups.
- Rebased onto main 183749a with one import-block conflict (B2a's
  recordColumns vs the deleted RecordsTable import; kept B2a's + the new
  WorkspaceImage import, zero remaining references); all checks green;
  merged --ff-only, pushed, recorded slice line.

## Follow-ups (in tasks.md)

Doubled alt-text error; hidden-section required blocks save invisibly;
empty ImageField label anchor; section key/label index; WorkspaceImage
comment; visibility not re-evaluated on draft edits.

## Try it

Give a Record Type a 2-section layout with an Image field (via API/CLI),
open a record in View: sections, visibility and image upload/alt/replace
all work from the layout.
