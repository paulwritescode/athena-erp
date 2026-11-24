# Review: F1 records-table rhythm (r-f1)

Verdict: PASS

Tree: athena-erp-f1-r, detached at 6f1b1e3, clean before and after review.
Diff a6dec5c..HEAD: 3 commits (507ffb5, 6b29ed1, 6f1b1e3), 5 files only —
DataTable.vue, dataTableModel.ts, style.css, recordColumns.test.ts,
dataTableModel.test.ts. DISJOINT holds: RecordsPage.vue, RecordsPage.test.ts,
PageSwitch.vue, pageSwitchBuild.test.ts untouched.

## 1. Rhythm (all verified in code)

- Footer always counts on selecting tables: `footerLeftText(hasBulk, …)` in
  dataTableModel.ts, template renders `{{ footerLeft }}` (DataTable.vue:686).
  Non-selecting tables keep the range. Matches §21 ("N of M selected").
- Actions column narrow + right-aligned: `w-12` head/body/skeleton with
  `pr-4 text-right`; colgroup fixes it at 48 px (§21). No content flush right.
- Select/rownum padding explicit: select `px-3`, rownum `px-4`, in header,
  skeleton and body rows. Nothing flush to any edge.
- Label headers: `buildRecordColumns` heads from field labels ("Image" not
  "image"); pinned by the new 'heads every column with its field label' test.
- Shared alignment incl. explicit left cells: header (:555) and body (:636)
  use the same `numericColumn`/`centerColumn` meta helpers; body adds explicit
  `text-left` default (header gets it from the TableHead primitive base —
  same effect). Money/Number right tabular, Checkbox center, Text/Date/Image
  left — pinned by tests old + new.
- Select-cell symmetry vs checkbox pr-0: REAL mechanism. The table primitive
  (`ui/table/TableCell.vue:13`, `TableHead.vue:13`) carries
  `[&:has([role=checkbox])]:pr-0`; the unlayered
  `.datatable-select-cell{padding-right:var(--space-3)}` in style.css restores
  12 px right, symmetric with the `px-3` left. Token, not literal.
- Hairlines: `border-b` on TableRow (pre-existing, confirmed). No filler
  column: colgroup lists select/rownum/data/actions only, none otherwise.
- 48 px thumbnails: `.table-thumbnail` uses `--space-12` = 48 px (theme.css:32).
- `.datatable-select-cell` defined only in style.css, used only in DataTable.vue.

## 2. Shots (8/8 eyeballed, no breakage)

table-{light,dark}-{375,768,1280,1440}.png all hold the rhythm: label
headers incl. "Image" and "Price (USD)"; Price/Qty right tabular; Due left;
Done centered header+cells; checkbox column padded; ellipsis narrow right;
hairlines; no filler; footer "0 selected" at rest; 48 px thumbnails; stacked
cards with #N prefix at 375; 768 scrolls horizontally per §21 tablet rule.

## 3. Tests and gates

- 26/26 slice vitest are REAL asserts (exact `toBe` strings, `toEqual` ids,
  `toMatchObject` meta/props, component-type `toBe`): 8 in dataTableModel.test.ts
  + 18 in recordColumns.test.ts. Ran green myself in this tree (1.4 s).
- live-footer-0 + 0-of-5-shape verified in code: RecordsPage.vue:130 passes
  `total: null` ("API returns opaque forward tokens with no total"), so
  `selectionText(0, null)` = "0 selected" — matches the shots. (Worker cited
  "RecordsPage:55"; the mechanism sits at :121–130 in this tree — line drift
  only, claim confirmed.)
- `CI=true pnpm --dir web run typecheck` green; `vite build` green (1.0 s).
- affected: `Go: none | web tests: yes`; suite reused w-f1's green run on the
  same clean 6f1b1e3 (log 1124-063421-w-f1.log read: 66 files / 636 tests
  passed). Not rerun per one-green-run-per-commit.
- `make generate` stable: tree clean after (no proto/schema touch).

## 4. Notes

- Skills-unavailable/design-system-applied claim sane: comms show both
  read_skill calls failed unknown-skill and master authorized the design
  system (§§6+12) as authority. Not a Z decision. No motion added; tokens
  only (`--space-3`, `--space-12`).
- dist gitkeep: `internal/webui/dist/.gitkeep` present; my own vite build
  deleted it and I restored it via git checkout — tree `git status` clean.
- Nits (no action): the RecordsPage:55 line ref (see §3); headers rely on
  the primitive's base `text-left` instead of an explicit class — same
  rendering, and the primitive owns that default.

F1 spec tasks.md:162 fully covered. No scope creep, no server/proto/API change.
