# Review: B8 slice 2, root presentation + Build/View (LAST slice)

Branch `feat/b8-root-buildview` (3489f84) in `../athena-erp-b8`,
`git diff 0afe93c...HEAD` (one commit, 19 files, all `web/src`).

Verdict: PASS

No blockers. One follow-up (F1, pre-existing, outside the slice).

## Fix loop 1 re-review (d81473f)

Verdict (fix loop 1): PASS

Worker commit `d81473f` ("stub IntersectionObserver in the PageSwitch
build test") is test-only: 11 added lines in
`web/src/pageSwitchBuild.test.ts`, no product change. The diagnosis is
confirmed from source, and no product change is needed:

- The branch was rebased onto `main` 35061c0, which includes B10 slice 2:
  `PacksPage.vue:11` now renders `art="empty-packs"` on its empty state.
- `ArtFigure.onMounted` (`web/src/art/ArtFigure.vue:21-27`) calls the
  figure's `create()`, and the hairline library constructs an
  `IntersectionObserver` with no feature guard
  (`@lucasmarkes/hairline/dist/index.js:336`).
- jsdom has no `IntersectionObserver` (verified: `typeof` is `undefined`
  with the repo's jsdom), so mounting Packs throws inside the async hook;
  Vue routes it to `PageSwitch`'s `onErrorCaptured`, which latches
  `loadError=true` (`PageSwitch.vue:97-104`). The reset watch fires only
  on mode/page/route change, and preview enter/exit changes none of
  those — so after a preview exit the latched `ErrorState` stayed instead
  of the Packs page. Exactly the reported failure.
- The no-op stub (observe/unobserve/disconnect on `window` + `globalThis`)
  keeps the art mount quiet; figures simply never receive visibility
  callbacks in tests. `IntersectionObserver` is Baseline in all modern
  browsers, so production is unaffected.

Re-runs on `d81473f`: `pageSwitchBuild.test.ts` alone 3/3 pass; full web
suite 55 files / 563 tests pass (555 before + 8 from the rebased main).
Branch diff vs the new base is still the same 19 slice-2 files. The F1
follow-up stands; nothing new found.

## Checks run (all green)

- `CI=true pnpm --dir web run typecheck` — exit 0.
- `CI=true pnpm --dir web run test` — 55 files / 555 tests pass, including
  the 6 new slice-2 files (`startupMode`, `pageSwitchBuild`, `preview`,
  `shellPreview`, `userMenuCaption`, `viewHidesBuild`) and the rewritten
  `rootAccess` tests.
- `CI=true pnpm --dir web run build` — ok.
- Browser pass on a throwaway Workspace (fake HOME, `test_athena_b8_slice2`
  database, loopback 127.0.0.1:3213; seeded 2 Record Types, Clerk/Reader
  roles, 2 invited users; server stopped, database dropped and scratch
  removed afterwards; dev database untouched): 18/18 root checks,
  6/6 viewer checks, 10/10 width×theme matrix checks (375/768/1280/1440 ×
  light/dark, theme explicitly set and verified via the `dark` class),
  4/4 gap-closure checks. 40+ screenshots captured; 12 inspected at the
  pixel level (caption, badge, disabled row menu + sheet, preview panel,
  View at all widths, denied page, light theme).

## Item-by-item (review brief)

1. Root presentation — verified live. User menu caption "Root administrator"
   (`UserMenu.vue`, via `gloss('rootAdministrator')`); Access row subtitle
   + outline `lock` "Root" badge with the reason as `title`
   (`AccessPage.vue:302,313`); row menu items and sheet footer buttons
   visible but disabled, each carrying the reason (`userActionsFor`,
   `disabledReason` through the shared Action components). Consistent with
   the slice-1 server refusals; `rootUser.ts` notes the server re-checks.
   Bonus: `isRootUsername` (trim + lowercase) resolves the recorded slice-1
   follow-up "Desk isRootSelected exact-matches lowercase" (`tasks.md:172`).
2. `?mode=build` fresh load — verified live (lands in Build with the
   caption). No composable changed in this slice: the guard is the B2 line
   (`useSession.ts:63`, only flips to View when `signed-in`), and the new
   `startupMode.test.ts` pins it through the real startup path — test 2
   holds `getSession` pending while the database connects, which is exactly
   the `canBuild` true→false transition (`useSession.ts:39`) the unguarded
   watch used to act on. Not dead code: without the guard that test fails.
3. Mode switch + View-hides-Build — verified live both ways at all widths.
   Segmented toggle-group with `hammer`/`eye` + words, `v-if="canBuildDesk"`,
   header ≥640px and user-menu radios below (matches §7 incl. the
   user-menu/header/header placements). Persistent "Building" caption in
   Build (`PageSwitch.vue`); hairline-separated Preview-as sidebar group;
   §15 sidebar groups. View has zero Build nav: sidebar (root and viewers),
   palette Build group (gated by `mode`), phone bottom nav
   (Invoice/Note/My work/Count only), Records empty-state gate
   (`canBuild`), direct `/build/*` URLs render PermissionDenied for
   viewers (explicit-address-keeps-mode is pre-existing intended behavior).
4. Preview-as read-only — verified live for Clerk (Invoice read/create/write
   + "1 field stays hidden", Note read) and Reader (Note only). The panel
   exposes a single Exit control and zero inputs/selects/textareas; no
   Record contents; identity unchanged while previewing; leaving Build
   auto-exits (`App.vue` watch, verified live); a previewed Role that
   vanishes ends the preview. Cross-checked against reality: clerk's and
   reader's actual View nav match their previews exactly. Previews need
   `build` + `access:roles`, so `viewRecordTypes` is the full type universe
   (server: "a builder sees every Record Type", `record_types.go:191`) and
   the denied reason renders otherwise (seen live as clerk).
5. Scope — presentation + mode only. Diff touches no permission, gate or
   auth logic (no `internal/`, proto, migration; existing composables and
   `registry.ts` untouched — only new `usePreview.ts` added). New CSS uses
   tokens (`--space-*`, `--border`, `--radius`, `text-caption`,
   `text-muted-foreground`/`destructive`); icons via the registry
   (`lock`, `FileX2`); no custom motion.
6. Gates + browser matrix — as listed above. Worktree left clean at HEAD
   (build-deleted `internal/webui/dist/.gitkeep` restored).

## B8 task coverage (last slice)

Slice 1 (merged 4d03467, not re-reviewed) + this slice cover every B8 task
bullet: server/CLI root confirmation; quiet caption + lock badge + disabled
actions with reasons; `?mode=build` fix pinned by regression tests; §7 mode
switch with persistent caption and hairline-separated group; View hides all
Build navigation; read-only "Preview as". B8 is complete on merge.

## Follow-ups (non-blocking)

- F1: the Access Users table Roles column shows "No roles" for clerk/reader
  although their assignments are live (their View nav matches their Roles'
  grants exactly; `user list` carries no roles field). Pre-existing Desk
  data behavior — slice 2 touched only the root branch of that cell
  (`AccessPage.vue:324`, same output as before). Suggests the column reads
  a source that omits assignments; owner: whoever owns the Access table.

## Environment notes (not slice defects)

- A bare shell here has no `node` on `PATH`; all pnpm/Node runs used
  `~/.nvm/versions/node/v26.7.0/bin` with the committed lockfile.
- No Playwright/Puppeteer is installed (and lockfile-only installs are
  required), so the browser pass drove headless Chrome over CDP with a
  dependency-free Node script (real mouse events for Reka primitives).
- The server's auth rate limiter works: repeated harness logins earned
  `resource_exhausted` + "Too many tries. Wait 105 seconds and try again."
  Passes were paced (~150 s between logins); no product issue.
