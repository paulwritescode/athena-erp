# Review: B9 follow-up 2.3, list-image thumbnails

Verdict: PASS

Branch `feat/b9-form-builder` at de2b4a1 in `../athena-erp-b9form`, 2.3 diff
`9fdad29...HEAD` only (slices 1–2 already merged; not re-reviewed). Reviewed per
`review-brief-form-2-3.md`, the follow-up 2.3 text of `form-brief.md`,
`design.md` section 5, the design system (§7, §13, §14, §21), `AGENTS.md` and
`.kiro/steering/athena-design.md`. Read-only on product code; all probes ran on
a throwaway Workspace (`test_athena_b923_r1`, fake HOME, port 3847). No
blockers, no follow-ups.

## What was verified

Thumbnails (brief 1). An Image primary column tiles `PrimaryCell` with the
`GetFile` thumbnail and reads the alt text as the title (empty reads
`Untitled`, no tile); non-primary Image columns render a 48 px thumbnail box
with alt text as both `alt` and tooltip; empty values render an empty span —
no broken tiles anywhere. Alt text also feeds `accessorFn`, so sorting,
tooltips, and stacked phone rows stay meaningful. The primary tile is
`aria-hidden` with the alt text duplicated as the visible (announced) title;
list cells expose the alt text directly. Missing files degrade to the quiet
muted box, text silenced in tables.

Proven in a live browser against a Record Type with a primary Image column
plus a second Image column, over three records (primary filled / primary
empty + spare filled / both empty): both rendered tiles measured 256 px wide
with correct alt text; primary cell text reads "A wooden stool" + timestamp;
empties read "Untitled"; tooltip reads "Spare part"; zero broken images; 375 px
stacked rows show the tiled primary + second column with row numbers, the
third column correctly hidden per §13.

Scope (brief 2). Four files: `recordColumns.ts` (+42/−9), `PrimaryCell.vue`
(+2 props, one tile branch), `recordColumns.test.ts` (+5 tests), `style.css`
(+3 token-only lines). No other list, editor, `forms/`, or server changes.
Token guard green (suite passes, no literals in the diff).

## Commands observed (exit 0 unless noted)

- `CI=true pnpm --dir web run typecheck` — clean.
- `CI=true pnpm --dir web run test` — 48 files, 530 tests pass (incl. the 5
  new thumbnail/alt tests).
- `CI=true pnpm --dir web run build` — builds; `internal/webui/dist/.gitkeep`
  restored afterwards, `git status` clean.
- Browser matrix on the throwaway Workspace at 375 and 1440 px in light and
  dark (4 screenshots in `/tmp/b9review3/screens/`), all visually inspected.

## B9 coverage (last slice)

The B9 task text (`tasks.md:86-90`) is fully covered once this merges: layout
in definitions (sections, 1|2 columns, order, widths, help, CEL visibility,
Image type with thumbnails + required alt, static text/image blocks) — server
slice 1; View renderer — 2.1; drag/keyboard builder with phone/desktop preview
in the Form tab, journaled non-destructive saves — 2.2; list thumbnails — this
slice. The art header stays a placeholder until B10, which owns the registry.
Acceptance lines (round-trip tests; upload/display/delete; keyboard-only
reorder) were each proven in their slice's review. The B9 tick itself is the
master's merge step.

## Provenance

Server: `../athena-erp-b9form` binary on `127.0.0.1:3847`, Workspace
`test_athena_b923_r1` (database of the same name), seeded Record Type `item`
with primary + spare Image fields and three records. Scripts and screenshots
kept at `/tmp/b9review3/` (not committed). Dev database untouched; no other
agent server touched.
