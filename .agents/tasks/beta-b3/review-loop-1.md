# B3 review loop 1 (Codex): fix all blockers and follow-ups, same worktree

## Decisions by the master (apply them, do not re-ask)

- **Record editing stays a full page** on every size. That was decided in step 2 and is now written into design-system §16 as an exception. Row detail sheets apply to read-and-act panels (people, roles, sessions, Rule Runs, files, ledger masters), not to the main Record editor.
- **Tablet row detail:** §16 wins on tablets (640 to 1023 px), so it is a right sheet. Bottom drawers are for dialogs, forms, pickers and menus (§13.2). On phones a row detail is a bottom drawer.
- **Drawer width on tablet:** the drawer surface spans the viewport and its content is capped at 640 px, centered.

## Blockers

1. **Nested drawer stacking.** Assign Role inside the user sheet cannot be tapped at 375 and 768, because the parent backdrop paints over the child drawer: both are z-50 and the parent is painted last (BottomDrawer.vue:37, style.css:456, AccessPage.vue:1065). A child drawer or dialog must sit above its parent's overlay. Use one stacking scale: sheet 50, child 60, toast 70. Test it with a real tap at 375 and 768.
2. **Hard close needs a confirmation** (LedgerSetupPage.vue:608). Ask with a ConfirmDialog that names the period and the consequence: "Hard close May 2026. Nobody can record or change anything in this period again, and it cannot be reopened." Use the destructive kind, and step-up when the server asks for it. Check the other irreversible actions on every page the same way: delete, revoke, end session, hard close, and cancelling a submitted document.
3. **Migration backfill lost on Esc** (MigrationDialog.vue:31,64). Pass the dirty state to the guard, and stop the plan watcher from resetting a draft the user has typed into. Reopening keeps the values until the save succeeds.
4. **An optional Enum cannot be cleared** (SelectField.vue:85). Optional selects offer a "None" option, or a clear button, that sets the value to empty. Required selects do not. Test both.
5. **Stale responses overwrite another user's sheet** (AccessPage.vue:118,125,132). Every async load for a selected item checks that the selection is still the same when the response arrives, using a request token or AbortController. Apply this in Access, and look for the same pattern in Records, Rules, Files and Ledger. Add a Vitest for the race.
6. **Overflow at 320 px.** The Record Types editor measures 365 px and its Delete control is off screen (RecordTypesPage.vue:181). The Gallery skeleton makes the page 338 px (GalleryPage.vue:181). Add 320 to the regress sweep for every page.
7. **Detail panels B3a left unfinished.** Files opens the inline editor (FilesPage.vue:333). Rule Run rows have no detail action (RulesPage.vue:301). Ledger company editing is inline on phones (LedgerSetupPage.vue:500). Apply §16:
   - File details (path, size, versions, backing Record Type or Rule) open in a sheet with Open in editor, Rename, Download and Delete. The editor workspace itself can stay a page area.
   - Rule Runs open a sheet with the outcome, the diff and the input hash.
   - Ledger masters (company, period, currency, account) open a sheet to view and edit, and a bottom drawer on phones.
8. **A toast is clipped at 768 next to the wide sheet:** the toast starts at x=-114 (style.css:467). The toast lane is the free area beside the sheet. When there is less free area than the toast needs, place the toast at the top center over everything. It must never run off screen. Test at 768, 1023 and 1280 with the 640 px sheet open.

## Follow-ups (do them in this loop)

9. Make the drawer X 44x44 at touch sizes (BottomDrawer.vue:39).
10. Put the 640 px content cap on the drawer content at tablet sizes (BottomDrawer.vue:61).
11. The Role selector in the Assign drawer still floats a SelectContent on small screens (AccessPage.vue:1069). Use SelectField, which is native or a drawer below 1024.
12. Pin `vaul-vue` exactly to 0.4.1 in web/package.json.
13. Take screenshots for every page at 375, 768, 1280 and 1440 in light and dark (b3l1-*), named page-width-theme.

Rerun typecheck, test, build and the regress and dialog scripts (add the new cases). Report with `hc send t6 "B3 LOOP1: PASS or BLOCKED"`.
