# Brief: B9a Desk, Record Types list and editor page (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b9a-desk, branch feat/b9a-desk. Work only there. Scope: web/src/pages/RecordTypesPage.vue, new web/src/pages/RecordTypeEditorPage.vue and components under web/src/components/record-types/, plus web/src/pages/registry.ts and routePaths for the new route. Do not touch Settings, notifications, the session code or the user menu: another worker owns them.
Read: AGENTS.md, .kiro/steering/athena-design.md, docs/design/athena-design-system.md sections 7, 13, 14, 16 and 19 (19 is this task), tasks.md B9a. The server data is merged: RecordType.record_count, count_hidden, updated_at, updated_by, pack_key.

1. **List** at /build/record-types:
   - Desktop and tablet: a DataTable with Name (with icon), Description, Kind badge, Fields, Records ("Hidden" with a tooltip when count_hidden), Pack ("None" until Packs exist), Last changed (readable time), Changed by.
   - Search, Kind and Pack filters, sortable columns. Primary action: New Record Type.
   - Phones: stacked rows with Name, Kind and Fields. A tap opens a bottom drawer with the summary and two actions, Open editor and View records.
   - No form on this page.
2. **Editor** at /build/record-types/<key> and /build/record-types/new:
   - Header: name, Kind, saved state. Save is primary; Delete goes in the destructive slot.
   - Tabs: Form (placeholder explaining that the form builder arrives in B9), Fields, Settings, Rules (list the Rules attached to this Record Type, linking to the Rules page), History (the saves from the journal; use what the API provides, and if there is no history procedure, show updated_at and updated_by and note the gap).
   - Move all current editing onto it with no behavior change: fields, settings, label field, kind, save, destructive-save planning with the MigrationDialog, delete, parent and child pair save.
   - On phones the page is full screen with the tabs as a segmented control, and the actions sit in the sticky bottom bar.
   - Reload and back and forward work. An unknown key shows the not-found state.
3. Remove the old inline form path completely.

Checks: typecheck, test (exit 0), build. Vitest for list filtering and sorting and the editor route parsing. Browser pass with a throwaway Workspace only, at 320, 375, 768, 1280 and 1440 in both themes. Run every Record Type flow end to end: create, add fields, save, destructive change with the plan, delete, pair save. Screenshots in .cache/screens/b9a-*.
Do not commit, merge, push or start agents. Report with hc send t6 "B9A DESK: PASS or BLOCKED".
