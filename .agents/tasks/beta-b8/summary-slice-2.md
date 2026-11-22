# Summary: B8 slice 2, root presentation + Build/View (merged d81473f; B8 done)

## What shipped

Root sees a quiet "Root administrator" caption (user menu) and a `lock`
"Root" badge on its own Access row with the reason; row menu and sheet
actions visible but disabled with reasons. `?mode=build` fresh loads land
in Build (the B2 session guard + regression tests through the real startup
path — no composable changed). §7 mode switch (segmented toggle + words,
builders only; header ≥640 px, user-menu radios below) with a persistent
"Building" caption and hairline-separated Preview-as sidebar group. View
hides ALL Build navigation (sidebar, palette group, phone bottom nav,
Records empty-state gate, direct `/build/*` URLs deny viewers). Read-only
"Preview as" renders View as a chosen Role (navigation only, zero inputs,
identity unchanged, auto-exit on leaving Build or vanishing Role).

## History

- Worker w-b8 committed 3489f84 (19 files), then fix d81473f in loop 1.
- Review round 1 PASS: 38/38 browser checks (root/viewer/width×theme/
  gap-closure), 555 web tests, 1 follow-up.
- Merge blocked once: the preview test failed only on rebased main —
  B10 lazy art observes intersection, absent in jsdom, so the Packs
  remount threw and latched the error state. Fix stubs the observer in
  the test (production browsers have the API). Re-review PASS (563/563).
- Rebased cleanly onto main 35061c0; all checks green; merged --ff-only,
  pushed, ticked B8 complete.

## Follow-ups (in tasks.md)

Roles column "No roles" despite live assignments (pre-existing data
behavior). The slice-1 isRootSelected nit was resolved in this slice.

## Try it

Sign in as root: caption + badge + disabled actions with reasons. Builders:
mode switch both ways, `?mode=build` fresh load, View with zero Build nav,
Preview as Clerk/Reader, exit restores the page.
