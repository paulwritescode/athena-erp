# B2 review loop 1: findings to fix

Source: Codex review (t14) plus the master's own look at b2-records-375-light, b2-gallery-1440-light and b2-access-768-dark. Fix in the same worktree, same scope (web/** and docs/architecture/frontend-theme-and-type.md). Worst first.

1. Navigation and permissions (useSession.ts:56,64,147; PageSwitch.vue:20; useDeskRoute.ts:24,55; App.vue:93)
   - A user with build and ledger:read who refreshes Ledger lands in View/Records. A files:read-only user opening Ledger lands in View/Records instead of PermissionDenied. Direct navigation must keep the page and mode, or show PermissionDenied. This also fixes the inherited `?mode=build` startup reset for B2's purposes; tell me if it fully closes the B8 item.
   - useDeskRoute ignores the pathname, so `/build/gallery` resolves to Overview. Read the path, and drop the separate validPages list: derive it from the registry.
   - From Work queue or Count, choosing a saved Record Type changes the selection but stays on the stub page. Selecting a Record Type must open its Records.
   - Switching from View to Build keeps the View stub. Each mode keeps its own last page.
2. Phones (RecordsPage.vue:524,531; AccessPage.vue:662; style.css:395; PageSwitch.vue:102)
   - Records and Access still render wide grids at 375 px: details clipped, columns past the viewport. Move both to the new DataTable (stacked rows under 640) or apply the same stacking. Keep the `athena-entry-columns:` key working at 640 px and wider.
   - The Records heading is right-aligned at 375 px. Headings sit top left.
   - Ledger has a 6 px gutter on phones. Use --space-4 like every page.
   - The mobile header shows the mark without the Workspace name. Show the name.
3. DataTable (DataTable.vue:28,89,91)
   - Clickable rows need keyboard activation (Enter and Space, focusable row or a real link in the first cell).
   - The phone CSS hides the only sort controls: give phones a sort select above the stacked rows.
   - Let a column declare that it hides on phones, and stack only the key fields.
   - Add loading (skeleton rows) and error states inside the table frame, and show both in the gallery.
4. Dialogs and sheets (ui/dialog/DialogContent.vue:37; ui/alert-dialog/AlertDialogContent.vue:45; DetailSheet.vue:13; ConfirmDialog.vue:6)
   - Below 640 px, dialogs and alert dialogs open as bottom drawers. DetailSheet opens from the bottom on phones.
   - ConfirmDialog needs busy and disabled-with-reason states, shown in the gallery.
5. Shell (DeskTopBar.vue:22; App.vue:71)
   - The 56 px rail has no working expand control. Add one, with keyboard access and a tooltip.
6. Visual rules (StatusBadge.vue:15; style.css:191; GalleryPage.vue)
   - StatusBadge uses the accent for success. Use --success and its soft background; the accent is only for primary actions.
   - Satori is applied to the Record Type name input. Satori is for h1 and h2 only.
   - Gallery cards have a large radius and a drop shadow, and nest cards inside cards. Cards use --radius, a hairline border, no shadow, and are never nested: make the inner examples plain bordered areas or no frame.
   - Add the date picker (Calendar plus Popover, design-system section 7) to the gallery.
7. Token guard (tokens.test.ts:348,363,369,423; frontend-theme-and-type.md:28)
   - Missed: a literal hex in a conditional v-bind object, and `stop-color`. Wrongly flagged: a `title` string, and `clip-path: url(#abc)`. Add fixtures for each.
   - The installed @vue/compiler-sfc does expose `descriptor.template.ast`: walk the real template AST instead of the hand-written scanner where it helps, and correct the doc line.
   - Line 369 emits `url(x[0])` into production CSS from a test comment (Vite warning). Remove it.
8. Dependencies (web/package.json:29,38)
   - No component imports `@vee-validate/zod` or `zod`. Remove both unless a component added in this task uses them.

Rerun typecheck, test, build; retake the 375 and 1440 shots for Records, Access, Ledger and Gallery in both themes; report with `hc send t6`.
