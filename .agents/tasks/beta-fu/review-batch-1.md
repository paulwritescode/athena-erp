# Review: FU batch 1 — Desk polish (12 fixes)

Verdict: PASS

- Tree: `/Users/iampaulking/projects/duchess/athena-erp-fu-r` detached at 425791b, verified clean on arrival and before gates.
- Range: `main..HEAD`, oldest db6a7fd → 425791b, 12 commits, 11 files.
- No blockers. Three non-blocking follow-ups at the end.

## 1. One fix per commit (all do what they claim)

| Commit | Bullet | Check |
| --- | --- | --- |
| db6a7fd | B9a icons breathing room | Action column 62→72px at both breakpoints, `.field-actions` gap 2px→`var(--space-2)` (`web/src/style.css`). Spacing token, 4px base. Shot shows spaced icons. |
| 99d9811 | B9a dead `.records-layout` | Zero `records-layout` refs remain in `web/src`; narrow-screen rule now targets `.record-meta` alone (a live class, `RecordTypeEditorPage.vue:289`). |
| 90c82ef | B4 Selected page accent | `bg-sidebar-accent`→`bg-sidebar-primary` + `font-medium` (`AppearancePreview.vue:69`). Matches the copy promise in `SettingsPage.vue:19` ("worn by … the selected page"); `sidebar-primary` is an allowed accent carrier. Visible in both preview panes, both themes. |
| bf006c3 | B4b dots follow page theme | `dotTheme` class on row dot, option swatches, match dots (`AppearanceSection.vue`). Dark shot: ink dot renders light and visible. Tokens only. |
| f509a46 | B4b dark ramp toward black | `.dark` + new `.dark[data-accent]` ramps mix toward black (`theme.css`); `MATCH_RAMP_VARS_DARK` + per-pane pick in `paneVars`; parity test pins the dark twin. Dark preview bars show a real tonal range. |
| 32886c4 | B4b CLI usage | Two string literals only (`cmd/athena/appearance.go`): usage lists all nine presets + custom, `--preset` help no longer says "ink or red". |
| 6198d0d | B4b empty slots show draft-accent ramp | `rampFallbackHex` mixes the draft accent toward white in oklab at the theme.css ratios; `PRESET_ACCENTS` mirror kept in step by test; test pins the ink ramp byte-exact to the old fallback. Hex lives in `appearance.ts`, not components (token guard honored). |
| ad8f664 | B4b Custom wheel until picked | Option swatch shows a `conic-gradient(var(--chart-1)…(--chart-5))` wheel while `!customPicked`; `customPicked` set on color-input and on loading a saved custom. Tokens only. |
| e77204e | B4b caption token | `text-[11px]`→`text-caption` on month + legend labels; `--text-caption` exists (`style.css:90-91`). |
| 2662371 | B9b stale draft comments | Four comment sites only in `useDeskRoute.ts`; no code lines changed. New wording matches the dialog flow. |
| 22b2aaa | B9b Change↔Auto toggle | `toggleKeyManual` freezes the derived key on reveal, returns to auto-derive (clearing key-touched) on second press; link label follows state. Browser log: "key field back to auto". |
| 425791b | B9b Enter surfaces inline errors | `TextField` (single-line `Input` only) gains an `enter` emit; dialog submits on Enter from all three fields and marks touched before the validity check. Test + browser log ("inline error after Enter", "duplicate error reads: That name is taken. Try another.") + both dialog shots (error shown, focus retained). |

Design-system posture: tokens not literals, no new hex in components, no typography/scope/icon/cursor/theme violations in the diff.

## 2. Skips (all verified)

- B9a ×3 satisfied by B9b, checked in the tree: Delete lives in the header "More actions" menu, not next to Save (`RecordTypeEditorPage.vue:248-270`); the editor title is an `h1` (`:241`); the Required checkbox sits in the centered `field-row` grid and the editor shots show it aligned.
- B4 Security copy already plain on main: `SettingsPage.vue:22` ("How long sign-ins last and how sign-in is protected."); file untouched by this batch. (The B4 black-swatch item was already handled pre-batch by the ink fallback and is improved by 6198d0d.)
- `--chart-palette` flag correctly left open: it needs new flag wiring, outside the usage-text-only scope.
- B2a slice 1 deferred, none half-built: `DataTable.vue` and friends untouched (`git diff --name-only` confirms); note `pressTimer` is already cleared on unmount in the tree (pre-existing).

## 3. Scope (clean)

11 files: `appearance.ts`/tests, `AppearanceSection`/`AppearancePreview`, `TextField`, `RecordTypeCreateDialog`, `useDeskRoute`, `fields.test`, `style.css`, `theme.css`, `cmd/athena/appearance.go` (two string literals). No server logic, proto, gate, migration, `RecordTypesPage`, `FieldSettingsDialog`, `fieldRules.ts`, `App.vue`, or `api.ts`.

## 4. Shots (all 6 eyeballed at 1280px, both themes)

- `fu-settings-light/dark`: Selected-page accent bar, visible ink dot on dark, match-dot ramp, tonal dark chart bars, readable month labels, plain Security copy. No regression; dark coherent.
- `fu-editor-light/dark`: h1 title, Save + `…` header, aligned Required checkbox, spaced action icons, Sonner toast bottom-right in light. No regression.
- `fu-dialog-light/dark`: duplicate name shows the inline error with focus still in the field; "Saved as widget_2 Change" auto-derive intact. Dark coherent.
- Not in frame (drawer/picker states): Custom wheel, draft-accent picker ramp, Auto-link state. Covered instead by code review, the new Vitest cases, and the browser script's functional checks.

## 5. Theme-menu note: harness artifact, not a Desk bug (follow-up, non-blocking)

The pick handler explicitly closes the menu (`AppearanceSection.vue:393`: `@click="theme!.setMode(item.value); close()"`), that code is untouched by this batch, no shot shows a stuck menu, and the same commit passed the same browser check three consecutive times after one timeout (`suite.jsonl`: exit 2 at 22:38, then exit 0 at 22:54, 23:10, 23:20). The predicate also watches `role="menuitem"` while the theme options are `role="radio"`. Repro for the follow-up: Settings → Your theme → pick a mode via the CDP script and watch the popover; harden the predicate to the radio group.

## 6. Gates (all green)

- Tree clean before gates; `internal/webui/dist/.gitkeep` restored after the build.
- `CI=true pnpm --dir web run typecheck`: exit 0 (ran here).
- Touched Vitest files `src/appearance.test.ts` + `src/fields.test.ts`: 35/35 pass (ran here).
- `vite build`: exit 0 (ran here).
- `.agents/bin/affected` (absolute main-repo path): reused the worker's green run on clean 425791b (exit 0, log `1123-223047-w-fu.log`, read: `ok cmd/athena` + 63 files / 616 web tests; the single "FAIL" grep hit is a test name, not a failure).
- No full browser re-run per the brief: worker's exit-0 pass on 425791b (`1123-232032-w-fu.log`, `PAGE_ERRORS: 0`, all six shots) + this review's shot eyeball is the evidence.

## Follow-ups (non-blocking)

1. `AppearanceSection.vue:283` — the Accent row indicator still shows the ink `accentHex` when `preset === 'custom'` but nothing is picked yet; consider the wheel there too.
2. Harden the browser script's theme-menu-closed predicate (see §5; flaked 1 in 4).
3. Consider capturing the Custom wheel, draft-accent picker ramp, and Auto-link states in a future browser pass.
