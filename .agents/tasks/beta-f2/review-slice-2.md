# Review: F2 slice 2 Desk search (r-f2s2)

Verdict: PASS

Tree: `/Users/iampaulking/projects/duchess/athena-erp-f2-s2-r`, detached at
1087724, clean. `git log main..HEAD` shows one commit only
(`feat(f2): desk type-to-filter on Records (slice 2)`) on main tip 96acb15.
Slice 1 confirmed merged (57e04c4 on main). Delta is 7 web files, +320/-42;
no proto, Go, CLI, or F4 files touched.

## 1. Search component + helpers + scope prop

- `web/src/components/DataTableSearch.vue` (new): v-model box, debounced
  emit at `SEARCH_DEBOUNCE_MS = 150` (`dataTableSearch.ts`), immediate
  synchronous `clear()`, `onUnmounted(() => clearTimeout(timer))` for
  timer cleanup. Caller-value-wins watcher cancels a pending burst.
- `pluralScope` / `searchPlaceholderFor`: "Diet concern" -> "Find diet
  concerns…". Endings covered (-y, sibilants, -ch, vowel+y) with tests.
- `searchScope` prop on DataTable is additive: explicit `searchPlaceholder`
  wins, else scope-derived, else no box. `showSearch` semantics preserved
  for existing consumers (all 17+ shipped `search-placeholder="Find …"`
  call sites keep working; override-wins covered by test).
- Find-verb derivation matches design-system §21 ("Find customers…") and
  the shipped placeholders. VARIANCE vs task-text "Search" is per ruling,
  not flagged.

## 2. RecordsPage wiring (granted scope only)

Diff is exactly the four granted items, nothing else:

1. `appliedQuery` ref + `query: appliedQuery.value` on `listRecords`.
2. 500 ms `filterTimer` debounce removed; `watch([filterText,
   statusSelected], applyListFilter)` applies at once (debounce now lives
   in DataTableSearch). Dead `onClearFilters` + `@clear-filters` binding
   removed with the timer it served; DataTable still emits `clearFilters`
   (line 353) for F4.
3. Template `search-placeholder='Filter records, …'` replaced with
   `:search-scope="recordType.label"` (derived placeholder).
4. `currentExpression()` takes `''` + status states (status-only override;
   `combineRecordFilter('', …)` is valid — empty text is skipped), with a
   no-change skip guard so the watcher + direct `onFiltersChanged` call
   list once per burst.

No other RecordsPage logic changed. No `filterTimer` / `onClearFilters`
references remain.

## 3. Glossary

`find` / `clearSearch` added and used (`gloss('clearSearch').label` for the
clear button, `gloss('find').label` in the derived placeholder). The old
hardcoded `aria-label="Clear search"` in DataTable.vue is gone; no
hardcoded Search/Clear/Find user words remain in the search block (only
identifiers and icon names).

## 4. First-char proof + browser E2E

- Unit: `dataTableSearch.test.ts` "emits the first character after the
  pause" types `z` through the real DOM input event (Reka Input ->
  onInput path, not a programmatic set), asserts instant display, silence
  before 150 ms, emit at 150 ms. Burst/clear/caller-wins/unmount cases
  covered (`DataTable.test.ts` covers scope placeholder wiring).
- E2E (`/tmp/f2-records-search.js` via suite `browser`, exit 0, head
  df2b80d + uncommitted slice 2): seed 1 type + 5 rows, `z`->2,
  clear->5, `apples`->1, clear->5, `ac`->2, placeholder
  `input[placeholder="Find notes…"]` asserted, 4 shots (1280+375 x
  light/dark), `PAGE_ERRORS: 0`. Full step log verified in
  `suite-logs/1124-081842-w-f2.log`.
- Transplant check: the 3 modified files are byte-identical between
  df2b80d and 96acb15, so the rebased 1087724 carries the identical
  slice-2 content the browser passed on (new files have no base).
- All 4 shots eyeballed: `ac` -> Acme + Acorn rows, clear X in box,
  Clear filters / Filters-1 badge, paging footer; no defects at either
  width or theme.

## 5. Gates

- `affected`: dedup green on clean 1087724 (worker's run
  `1124-102307-w-f2.log`); log read: 70 files / 660 tests passed — claim
  verified.
- `vue-tsc`: clean, run in this tree (20.9 s, pnpm Done).
- `gofmt -l cmd internal`: empty; `go build ./...`: ok (both run here).
- No proto change: delta is web-only (7 files under `web/src/`).
- Web build: `make build` incl. vite build exit 0 inside the browser
  harness (`/tmp/f2-shots/run.log`).

## 6. Scope

- No Advanced filter / F4 builder code: only two disclosed comments
  naming the affordance point (`appliedFilter`/`currentExpression`
  override; "F4's Advanced filter builds on this block"). Deferred item
  correctly left unbuilt.
- No CLI search flag (nothing outside `web/src/` changed) — correctly
  absent per brief.

## Notes (non-blocking)

- `onFiltersChanged` calls `applyListFilter()` directly while the watcher
  fires too; the no-change guard makes the second a no-op. Harmless.
- All other DataTable consumers now get 150 ms debounced search for free;
  consistent with §21, no consumer relied on per-keystroke emits.
