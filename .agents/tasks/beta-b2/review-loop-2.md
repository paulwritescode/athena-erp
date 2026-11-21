# B2 review loop 2: findings to fix (last loop before the user decides)

Source: Codex re-review plus the master's look at l1-records-375-light, l1-access-375-dark and l1-gallery-1440-light. Same worktree and scope. Worst first.

## Routing (useDeskRoute.ts:43,48,76-80,101,108; useSession.ts:65-69,96-97)

1. From Build, switching to View and opening New record leaves the address at `/view`. Refreshing lands on `/build/overview`. The URL writer must include the active Record Type, plus the record ID or `new`. The reader must treat bare `/view` as View.
2. A saved Record Type keyed `queue` or `count` shares its URL with the View stubs. Put stubs under a reserved segment (for example `/view/-/queue`) or Record Types under `/view/r/<key>`, and test both.
3. Distinguish an explicitly requested page from the default landing:
   - A files:read-only user at explicit `/build/overview` gets PermissionDenied (or Overview, if Overview needs no permission). They are not redirected.
   - A user with no Build access at `/build/ledger` gets PermissionDenied, not Records.
   - Only a bare or default URL redirects.
4. Mode flips must not push intermediate history entries. Restoring the last page is one `replaceState` (or one `pushState` for the final page). Back must return only to pages the user actually visited.

## Phones (AccessPage.vue:662,709,744,767; RecordsTable.vue:126)

5. Roles, Permissions and Sessions in Access still use wide tables (Roles measured 546 px at 375). Stack them too.
6. Stacked rows show only 2 or 3 key fields: name or title, then the status, then one detail. Other fields move into the row's detail view. This applies to Records and every Access tab.
7. Access Users cards fill only about half the width at 375 px. Stacked rows use the full content width.
8. The Build bottom nav truncates "Record ..." to fit. Use short labels from the registry (for example "Types") or icons with full labels in the More sheet. No label may truncate.

## Shell and dialogs (ui/sidebar/Sidebar.vue:66; App.vue:79; AlertDialogContent.vue:45; ConfirmDialog.vue:25; GalleryPage.vue)

9. Expanding the 56 px rail at 640 to 1023 px pushes the layout (main moves 56 to 240 px, page width grows 768 to 869 px, and the right edge clips). On tablet the expanded sidebar overlays content, with a dim overlay and focus trapped. It closes on Esc, on an outside click and on navigation.
10. The phone confirmation drawer is 320 px wide at 375 px. Bottom drawers are full width.
11. While ConfirmDialog is busy, Esc and outside clicks must not close it.
12. The Gallery heading sits about 10 px under the top bar at 1440 px. Every page has the same top padding (--space-8 desktop, --space-6 tablet, --space-4 phone) from one place, the page layout, not per page.
13. Check that the stacked-row labels ("Title", "Detail") use Inter caption, not Satori.

## Token guard (tokens.test.ts:389,523; frontend-theme-and-type.md:28)

14. Wrongly fails: comparing `accent === 'red'` inside a v-bind conditional where the painted value is `currentColor`. Only flag literals that reach a paint position.
15. Missed: a `<script setup>` object with a literal `#123456` used through `:style`. Scan string literals in `<script>` and `<script setup>` that hold a color, and allow them only in a file listed as an exception.
16. Fix the doc claim. If any limit remains, list it, as in B1.

Rerun typecheck, test and build. Retake 375, 768 and 1440 shots of Records, Access (all tabs), Gallery and the expanded rail at 768, in both themes. Report with `hc send t6`.
