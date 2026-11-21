# Review: B9b name-first New Record Type flow

Verdict: **PASS**

Branch `feat/b9b-new-type`, commit `0596b2e`, worktree `../athena-erp-b9b`.
Reviewer read-only on product code; no product files changed by this review.

## Blockers

None.

## Brief checklist (all met)

- List header and empty state open a `ResponsiveDialog` titled "New Record Type" with Name (required, autofocused — verified `document.activeElement` is `new-type-name`), "What is it for?" (optional), and Kind radios with one-line explanations defaulting to Master (`RecordTypeCreateDialog.vue`, `recordTypeKeys.ts` `KIND_OPTIONS` from glossary).
- Key derived from the name with the existing `snakeKey` rules (moved unchanged into `recordTypeKeys.ts`), shown as "Saved as `<key>`" with a Change link that reveals a Key field. Verified `Customer` → `customer`, duplicate → `customer_2`, custom key `vendor` saves and opens `/build/record-types/vendor`.
- Create disabled until the name is valid and unique (`disabled-reason` carries the message); errors render inline under the field with an alert icon after blur. Verified empty-name disabled and duplicate-name inline error "That name is taken. Try another." with Create disabled.
- Create saves a real Record Type with one required Name text field (`useRecordTypes.ts` `createRecordType`: `name`/`Name`/`Text`/`required: true`, revision from the server, draft rolled back on failure) and opens `/build/record-types/<key>` on the Fields tab (verified URL, active tab state, and field values `label=Name key=name type=Text required=true`).
- Editor title is an `h1`, editable in place by clicking the title, by the quiet pencil button, or by Rename in the header menu; blank commits keep the old name. Verified rename Customer-flow via title click (Vendor → Vendors) with save toast.
- Old unnamed-draft path removed: `addRecordType`/`renameRecordType`/`savedAs`/`ensureDraft`/`isNew` have zero references left in `web/src`; `/build/record-types/new` now renders the list with the create dialog over it (verified: dialog open, list visible, zero editor tabs).
- B9a follow-ups in these files: Delete separated from Save (header `...` menu, Rename + divider + destructive Delete; mobile keeps the separated full-width Delete); title is the `h1`; field-row action icons fit in the widened 62 px column (2 buttons, measured 62 px wide, inspected in screenshot); Required checkbox sits aligned under its left-aligned column header and vertically centered (inspected in screenshot; geometry `inputX=0 cellW=157`).
- Scope kept: 8 files under `web/src` only, no Go changes.

## Checks run (all green)

- `go build ./...`, `go vet ./...`, `gofmt -l cmd internal` (empty).
- `CI=true pnpm --dir web run typecheck` exit 0.
- `CI=true pnpm --dir web run test`: 36 files, 419 tests passed, including new `recordTypeKeys.test.ts` (4 tests: derivation, uniqueness, reserved words, create validation); re-ran that file alone, 4/4.
- `CI=true pnpm --dir web run build` exit 0; restored `internal/webui/dist/.gitkeep` afterwards (`git status` clean).
- `make test` exit 0 (Go suite with `ATHENA_TEST_DATABASE_URL` set plus the web suite).

## Browser pass (throwaway workspace, dev database untouched)

Server: branch binary + freshly built `webui/dist`, `test_athena_b9b_review` database on port 3110 (the install lock was held by the B2A review server on 3100, so this server ran with an isolated config home; B2A's server verified still healthy afterwards). Signed in as `administrator`. Scripted pass: 29/30 assertions green plus 1 follow-up check green (the single FAIL was a script bug — it read `textContent` instead of input values; the corrected check confirmed the default field).

- 1440 (dark/system): empty list, dialog empty/filled/duplicate, create → editor on Fields tab, custom key, rename with toast, `/new` dialog-over-list, header menu, field-row geometry.
- 1440 light (via the header theme button): list and filled dialog render on neutral surfaces with the ink primary.
- 375 (dark): drawer create flow (`data-vaul-drawer` present), editor with sticky View records/Save/Delete bar, duplicate flagged inline, title tap-to-rename.
- 375 light: drawer renders correctly.
- Screenshots (18) archived in the worktree at `.cache/screens/b9b/`; inspected: dialog filled/duplicate, editor fields/menu, mobile dialog/editor, light dialog 1440/375.

## Non-blocking nits (follow-ups, do not hold the merge)

1. Stale "draft" comments in `web/src/composables/useDeskRoute.ts` (~lines 32-34, 166-168): `/new` is no longer a draft.
2. One-way Change: once the Key field is revealed there is no return to auto-derive.
3. Duplicate-name error appears on blur; typing a duplicate and pressing Enter while Create is disabled shows no inline error until blur. Standard pattern, acceptable.
4. B9a follow-up "dead `.records-layout` rule (`style.css:178`)" still present; outside the B9b brief's listed files.

## Cleanup notes for the master

- Dropped `test_athena_b9b_review` after the pass. `test_b9b_review` (first attempt, before the Stylus policy arrived) is left in place: the policy forbids dropping databases that do not start with `test_athena_`.
- Review server on 3110 stopped; no files changed outside the worktree except this review file and `/tmp` scratch.
