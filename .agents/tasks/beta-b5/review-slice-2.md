# Review: B5 slice 2, Workspace identity Desk

Branch `feat/b5-identity` in `../athena-erp-b5`, slice-2 diff `4698f96...HEAD` (all under `web/`).
First review at `2e3f5b7` ended NEEDS_CHANGES (blockers B1-B2 below); re-review at `5465cfe`.

Verdict: PASS

## Re-review at `5465cfe` (fix loop 1)

The worker typed the `setIdentity` mock parameter (B1) and switched pre-save previews from `blob:` to `data:` URLs via `FileReader`, leaving the global CSP untouched (B2), plus a test asserting the preview `src` starts with `data:`. I verified: web typecheck exits 0, web tests pass 41 files / 465 tests (token guard included), web build exits 0. Live browser check on a throwaway `test_athena_*` Workspace (fake HOME, dev database untouched, test database dropped): chose both logo files through the real Settings UI and both previews render as decoded `data:` images (`naturalWidth > 0`), zero Content-Security-Policy console errors in the session, save applies name + logos + favicon, and a pixel-inspected screenshot shows both previews rendered where the broken-image glyphs were. The fix commit touches only `IdentitySection.vue` and `identitySection.test.ts`; no regressions found. Both blockers closed; follow-ups F1-F5 stand as recorded.

## Blockers (closed in `5465cfe`)

### B1. Web typecheck is red on the branch

`web/src/identitySection.test.ts:90` — `mockImplementation(async (req: never) => ...)` fails with `error TS2345: Type 'MessageInit<SetIdentityRequest>' is not assignable to type 'never'`, so `CI=true pnpm --dir web run typecheck` exits 2. The project's own merge checks (design.md section 5) require typecheck green, so this stops the merge as-is. Runtime tests pass (vitest does not typecheck). Fix is one line: type the mock parameter as the request instead of `never`.

### B2. Pre-save logo previews are blocked by the page CSP (broken images)

`web/src/components/settings/IdentitySection.vue:128` previews a chosen file with `URL.createObjectURL(file)`, but the served Desk sends `Content-Security-Policy: ... img-src 'self' data: ...` (`internal/server/gate.go`, unchanged by this slice), which does not allow `blob:`. Choosing a file therefore renders a broken image with alt text instead of a preview — I reproduced it live in the browser (console: `Loading the image 'blob:...' violates ... img-src 'self' data:. The action has been blocked`) and confirmed it visually in screenshots at 1280 px: both preview boxes show the broken-image glyph. Brief item 2 requires "light and dark logo upload with preview"; the preview half is 100% broken. Post-save previews (same-origin `/brand/` URLs) render fine. Suggested fix: read the file as a `data:` URL (`FileReader.readAsDataURL`, allowed by `img-src`, ≤512 KB so size is fine, still scriptless inside `<img>`); do not loosen the global CSP for this.

## What passed (brief items 1-5, except the blockers)

1. Header (sidebar 24 px, mobile top bar 24 px), sign-in (48 px), tab title (`<name> - Athena`) and favicon all follow the Workspace identity per theme; measured heights exactly 24/48 in the browser at every width. Mark fallback is black-on-light / white-on-dark everywhere. "Powered by Athena" appears in the user-menu footer (desktop dropdown and mobile drawer) only once a logo is set, and is absent before. Logos never stretch (`object-contain`, letterboxed).
2. Settings → Workspace identity: name edit, light/dark upload with preview boxes at 48 px plus quarter-height clear space, staged Remove with "Keep logo" undo, dirty-tracked Save with success toast, type/size validation inline (`role="alert"`), read-only copy plus disabled-with-reason buttons without `settings:workspace` (unit-covered). Works with no logo (mark fallback asserted in every state). Set → reload → verify → remove → verify-fallback was driven end-to-end through the real UI.
3. No `athena-black-logo` / `athena-white-logo` root copies; B2's `web/src/assets/brand/athena-black.png` + `athena-white.png` are reused, nothing duplicated.
4. Token guard (`tokens.test.ts`) green — it passed in the suite, and the fixed `bg-white`/`bg-zinc-900` preview swatches are plain utilities (the guard scans arbitrary values), functionally required so each logo previews on its own theme. Favicons correct per state (brand URLs when set, `favicon-mark-*` otherwise, single `link[rel="icon"]`, no duplicates). Manifest name is `"Athena"`, consistent with the new tab title (the slice fixed the old "Athena ERP" title mismatch). `theme-color` stays static light in dark theme — pre-existing, filed as F4.
5. `pnpm test`: 41 files / 465 tests pass (includes the new `identity.test.ts` + `identitySection.test.ts` and the token guard). `pnpm build` exits 0 (`.gitkeep` restored afterwards). Browser pass: headless Chrome against a throwaway `test_athena_*` Workspace on a fake HOME (dev database untouched, test databases dropped, server stopped): 100+ scripted DOM assertions green across 375/768/1280/1440 × light/dark covering sign-in fresh, desk + sign-in with identity set, removal, and fallback — plus pixel-inspected screenshots of the settings section (empty/filled/saved/error), sign-in (mark + logo, both themes, mobile + desktop), desktop header, mobile header/drawer, and both "Powered by Athena" footers. No page errors besides the B2 CSP blocks and routine `GetSession` 401s on stale-cookie boots (proven pre-existing: cleared-cookie boots 401-free, and slice 2 adds only the public `getIdentity` call).

## Follow-ups (non-blocking)

- F1. "No logo" empty text is unreadable in the mismatched swatch/theme combos (theme-colored title text inside the fixed white light-swatch in dark theme, and inside the fixed dark swatch in light theme). Give the swatch empty state fixed-contrast text.
- F2. Logo inline errors render bare red text without the `CircleAlert` icon that §7 and the shared `FieldShell` errors use (`IdentitySection.vue:235`). Reuse the `field-error` pattern.
- F3. Clearing the display name cannot work (server treats `""` as unchanged) but the UI shows dirty, toasts success, and snaps the old name back. Consider a client-side nonempty guard.
- F4. `theme-color` is static `#f7f7f7` in both themes (`web/index.html:6`, pre-existing). Consider updating it on theme change.
- F5. The fallback favicon is the black mark in both themes (merged slice-1 server design), low-contrast on dark browser chrome. Options: server white-mark variant with a theme-aware pick, or document as accepted.
