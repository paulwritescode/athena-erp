# Review: B11 slice 2, Setup wizard + readiness view

Branch: `feat/b11-install` in `../athena-erp-b11`, commit `f0870ce`.
Scope: slice 2 only (reviewed diff `git diff 172c749...HEAD`: new
`web/src/components/setup/*`, `SetupPage.vue` rewrite, one-line
`registry.ts` props addition, 8 test files, README + getting-started +
`e2e-install.sh` follow-ups). All gates, servers and the browser pass
ran in that tree at that commit; servers were throwaway (`athena run`
from this tree's `bin/athena` on 127.0.0.1:3222/3223 with fake HOMEs,
`test_athena_b11_*` databases only, never the dev database).

Verdict (round 1): NEEDS_CHANGES

One blocker: the period step wedges after a failed save (broken retry
flow, reproduced twice on fresh tree/binary/database). Everything else
passes, including the full six-step walk, the denied matrix, invite
delivery, load-error recovery and the whole gate suite.

## Blocker

**B1. Period step cannot succeed in-place after a failed save: the
`years` list goes stale.**
`web/src/components/setup/SetupPeriodStep.vue:105-111` — after
`saveFiscalYear` succeeds, the saved year is never added to the
`years` list the next attempt searches. Reproduction (throwaway
workspace, FY `legacy` + open period `early` Nov 1–15 pre-seeded, FY
`2026` missing):

1. Submit defaults (Nov 1–30): FY 2026 is saved, then the period save
   correctly fails with `ledger: period "2026-11" overlaps "early"`.
2. Retry unchanged: fails with `ledger: already exists: fiscal year
   "2026"` — the step re-saves the year it just created.
3. Change to clean December dates and submit: STILL `already exists:
   fiscal year "2026"`. No in-place input can succeed; the error
   misleads (the user never knowingly created FY 2026).

Screenshots: `trap-period-overlap-error.png`,
`trap-period-retry.png`, `trap-period-december.png`. Escape exists
(Back + Continue remounts and refreshes the list — verified working),
but nothing points the user at it. Fix is a one-liner: push the saved
year into `years.value` (or re-run `refresh()`) after `saveFiscalYear`
succeeds, and treat a duplicate-year conflict as "exists".

## Checks run (all in the b11 tree at f0870ce)

- `gofmt -l cmd internal` → empty. `go build ./...`, `go vet ./...` → ok.
- `CI=true pnpm --dir web run typecheck` → ok; `... run build` → ok
  (`.gitkeep` restored, tree clean).
- `make test` with MariaDB → exit 0, zero FAIL lines, 63 web files /
  611 tests pass (includes the new setup suites).
- `make generate` twice → both exit 0, no diff, tree clean.
- `./scripts/e2e-install.sh` with `SLUG=test_athena_b11_rev2` →
  28 passed, 0 failed (SLUG-override follow-up verified).
- No edits outside the slice diff; `AccessPage`, `App.vue`, shell and
  mode files untouched (verified via diff file list). The one-line
  `registry.ts` props addition follows the existing entries' pattern
  and changes no gate, mode or nav behavior.

## Browser pass (49 screenshots in `.cache/screens/b11-slice2/`)

Headless Chrome driven over CDP, all interaction through real DOM
events, two throwaway workspaces:

- Fresh six-step walk at 1280 dark: company create (Back hidden on
  step 1), logo upload incl. `.txt` and 600 KB rejections, USD create,
  period open, owner invite with one-time challenge capture, readiness
  with all checks ready + 3 books checks missing + toasts. Every
  inline validation error exercised and screenshotted.
- Back through all steps, forward via step-indicator buttons, landing
  on first-incomplete for fresh/complete/partial workspaces.
- Light theme: all six steps at 1280. Size matrix 375/768/1440 ×
  dark/light on company + readiness (mobile "Step N of 6" caption +
  progress bar asserted); logo/period/currency/invite at 1280 both
  themes. One primary action per step, same place; disabled-with-reason
  (e.g. "Choose a file first, or skip for now"), loading skeletons,
  `role=alert` errors, empty→create states all observed.
- Skipped logo + invite → readiness `· skipped` rows with Review
  actions (1280 + 375 dark).
- Invite delivery: alice's challenge redeems at First login → desk
  access confirmed.
- Denied states: bob (no grants) → switch-level denied; dave (custom
  `build`-only role) → the slice's own SetupPage denied branch; carol
  (custom `build` + `access:users` role) → full per-step matrix E2E
  (ledger steps explain + Continue, logo read-only, working invite
  list, readiness missing rows + "Books checks need the books
  permission").
- Load-error states: server killed mid-session → step-level "Cannot
  reach the Athena server" + Try again, page-level "Setup did not
  load" + Try again; server restarted → retry recovers to the wizard.
- Navigations: Open overview, Open Ledger setup, Edit in Settings,
  logo Change revealing pickers, readiness Review fix action.

## Verified by reading against server code

- Field checks mirror the server exactly: `namePattern`,
  `usernamePattern`, ISO currency regex, exponent 0–12, `validDate`
  bounds + date-order message, lowercase-trim username normalization,
  logo 512 KB + png/svg (messages match `brand.ValidateLogo`).
- Readiness merge: the four `wizardCoveredChecks` names match
  `GetReadiness` verbatim; unknown server checks pass through (unit
  test + E2E books list agree); UTC-today matches the server.
- `IdentitySection` reused read-only; `InviteUser` called directly
  with `Instance Owner`; glossary terms used exist; tokens, `h-18`
  (Tailwind v4) and icons check out.

## Follow-ups (non-blocking)

1. getting-started's period command omits required `--label`:
   `period save --company ... --name ... --start ... --end ...` fails
   with `ledger: the label is required...` (reproduced on the CLI).
   Add `--label ...` to keep the B11 copy-paste bar.
2. Same stale-list class as B1, narrower: if `setBase` fails after a
   fresh currency save (revision race), retry re-saves the existing
   currency instead of reusing it (`SetupCurrencyStep.vue`, `submit`
   create path). Refresh the list / flip to select mode on failure.
3. `checkLabel`/`checkDisplayName` count characters, the server counts
   bytes (`len`): a 255-multibyte-char label passes the wizard and
   fails server-side. Failure surfaces inline (graceful); align or
   accept.
4. Pre-existing server gap (out of slice): `GetReadiness` "Today falls
   in an open period" uses `PeriodFor`, which ignores status — a
   closed period covering today reports ready. The wizard's own check
   correctly requires `open`.
5. At 375 px scrolled content passes under the fixed bottom tab bar.
   Shell-wide (only `RecordTypeEditorPage` handles safe-area), not
   wizard-owned; needs a shell pattern, not a wizard fix.
6. `gotoStep` never updates `furthest` and always animates backwards
   (cosmetic; reachable only via readiness fix actions where furthest
   is already max).
7. The first owner the wizard invites cannot open Setup: Instance
   Owner lacks `build`, so the (pre-existing) registry gate denies
   them — verified E2E for alice. The wizard's per-step denied design
   therefore serves only build-holders with partial grants (verified
   E2E via custom roles). Confirm intended with the access-model
   owners; not this slice's bug (gate predates it, B8 owns access).
8. Skips persist only in memory: revisiting Setup later shows skipped
   steps as missing with "Go to" actions. Reasonable either way;
   confirm intended.

## Re-review (fix loop 1, commit bdff521)

Verdict (round 2): PASS

Scope: B1 fix only (F1–F8 skipped per master instruction, not
blockers). Tree verified clean at `bdff521` before any gate; all work
below ran in `../athena-erp-b11` at that commit on a rebuilt binary.

The fix does both halves: after `saveFiscalYear` succeeds the saved
year is pushed into the step list so a retry reuses it, and an
`AlreadyExists` from the save (race/stale list) reloads the list and
carries on to the period save. The catch is live: duplicate key →
`ErrExists` → `CodeAlreadyExists` verified in
`internal/ledger/ledger.go` + `internal/server/ledger.go`. Two
regression unit tests added (in-place retry after overlap asserting
one FY save + two period saves, duplicate-year adoption).

Trap E2E redo on fresh throwaway server/database (FY `legacy` +
period `early` Nov 1–15 reseeded): attempt 1 fails with the overlap
error as before; attempt 2 with clean December dates IN THE SAME MOUNT
now succeeds — lands on the invite step with "Opened period December
2026", no FY conflict anywhere (`fix-trap-overlap-error.png`,
`fix-trap-retry-success.png`). The wedge is gone.

No regressions: full six-step fresh walk with all inline error states
plus back/forward, light-theme pass and the 375/768/1440 × dark/light
matrix all re-ran green (32 shots). Gates: gofmt empty, build/vet
clean, web typecheck + build clean, `make test` exit 0 with zero FAIL
lines (63 files / 613 web tests, +2 new), `make generate` twice with
no diff, tree left clean.

B1 is closed. No new blockers introduced.
