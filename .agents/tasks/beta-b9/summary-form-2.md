# Summary: B9 form sub-slice 2, layout builder (merged 4113b23)

## What shipped

The form builder in the Record Type editor's Form tab: sections add/rename/
reorder, 1|2 columns, drag-and-drop ordering (within and across sections,
announced) plus full keyboard operation (move up/down, move-to-section,
Space/arrows, focus retention), field settings in a right sheet (width,
help, visibility CEL with inline server type-check errors), static text and
image blocks (image picker over recorded images/ files), free-text art-
header picker with placeholder preview (no B10 registry built), and a live
phone/desktop preview rendering every field disabled with visibility
captions. Saves are layout-only (no DDL, no confirm dialog, journaled,
visible in History after reload). Move buttons replace drag on phones;
sheets become bottom drawers.

## History

- Worker w-b9form committed 698f20f (16 files, new web/src/forms/ builder).
- Review round 1 PASS: builder verified end to end in a live browser
  (7-field arrange from empty state, drag by real mouse incl. cross-
  section, keyboard-only flows, sheet/blocks/picker/art, save with no
  migration dialog + toast + revision bump, reload identical, View matches),
  494 web tests, matrix 375/768/1280/1440 × light/dark, 7 follow-ups.
- Rebased cleanly onto main 2714a12; all checks green; merged --ff-only,
  pushed, recorded slice line (follow-up 2.3 open).

## Follow-ups (in tasks.md)

Stale History stamp until reload; focus-to-body on unmount; drop divs in
ul; picker listbox roles; duplicated picker alt; 28 px icon buttons (needs
a design decision); section index keys.

## Try it

Build → Record Types → open a type → Form tab: arrange fields by drag and
by keyboard, add sections/blocks, set visibility, Save, reload, open a
record in View.
