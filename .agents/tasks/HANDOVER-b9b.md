# Handover: B9b name-first New Record Type flow

## Worktree and branch

- Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b9b`
- Branch: `feat/b9b-new-type`, rebased onto main at `ec98c5f` (B6 glossary + B7 Desk sessions slice).
- Nothing committed (per brief: never commit/merge/push). Working tree holds only the change:
  - Modified: `web/src/components/PageSwitch.vue`, `web/src/composables/useRecordTypes.ts`,
    `web/src/pages/RecordTypeEditorPage.vue`, `web/src/pages/RecordTypesPage.vue`, `web/src/style.css`
  - New: `web/src/components/record-types/RecordTypeCreateDialog.vue`,
    `web/src/components/record-types/recordTypeKeys.ts`,
    `web/src/components/record-types/recordTypeKeys.test.ts`

## What is done and verified

All B9b implementation is complete and every required check passed on the final code:

- Name-first dialog on `/build/record-types/new` (dialog over the list): focused Name, derived
  `Saved as` key with Change, optional purpose, four Kinds with glossary one-liners + HelpHint,
  conditional parent picker for Child. Create disabled until valid; type saves with one required
  Name text field (Document class `non_posting`); draft rolls back on failure; `/new` replaced
  with the saved key.
- Unnamed-draft path removed (draft creation, key auto-follow, saved-as replacement, Discard
  wording, save-first gating). Editor title is the h1 (click / pencil / header-menu Rename);
  header is View + Save + ⋯ menu (Rename, divider, destructive Delete). Kind/key/parent fixed
  after save; ConfirmDialog is Delete-only.
- Four B9a follow-ups: delete off the primary row, view-without-save, field-row action column
  widened 22/18px → 62px at both breakpoints (the cramped column overlapped the settings button
  into the Required cell — the stray glyph), Required checkbox verified centered.
- `pnpm typecheck` clean; 419/419 Vitest pass (36 files, incl. new derivation/validation tests);
  `make build` green; throwaway-workspace browser pass at 1440 and 375 (create, duplicate-name
  rejection, rename by title and by menu, menu delete, phone drawer create + title-tap rename)
  all passed with no page errors. Screenshots verified by eye under
  `.cache/screens/b9b-*.png` (gitignored).

## What is in progress (file by file)

No file has pending edits. The only undelivered item is the report: `hc send t6 "B9B: PASS"`
failed 8+ times with `cannot save terminals: registry writer stopped` (even `hc ls` fails).
The hc/Herdr daemon needs attention after the restart; the implementation itself is finished.

## Exact next steps

1. After restart, confirm the worktree state: `git -C /Users/iampaulking/projects/duchess/athena-erp-b9b status --short`
   (expect only the 5 modified + 3 new files above) and `git -C ... log --oneline -1`.
2. Confirm `hc` works (`hc ls`). If the registry error persists, the Herdr daemon needs a restart.
3. Send the report: `hc send t6 "B9B: PASS"`.
4. Optional re-verification (no code changed since green, so only if doubted): the commands below.

## Commands to verify

Use absolute paths under the worktree; Node is at `$HOME/.nvm/versions/node/v26.7.0/bin`.

- Status: `git -C /Users/iampaulking/projects/duchess/athena-erp-b9b status --short`
- Typecheck: `PATH="$HOME/.nvm/versions/node/v26.7.0/bin:$PATH" CI=true pnpm --dir /Users/iampaulking/projects/duchess/athena-erp-b9b/web typecheck`
- Tests: same with `test` (expect 36 files / 419 tests passed).
- Build: `PATH="...:$PATH" CI=true make -C /Users/iampaulking/projects/duchess/athena-erp-b9b build`,
  then `git -C ... checkout -- internal/webui/dist/.gitkeep` (the build deletes that tracked stub).
- Browser pass: reset DB + migrate + `user list` (see below), serve on 3052, then
  `ATHENA_E2E_PASSWORD=<db password from .env.local> NODE_PATH=/tmp/b9a-e2e/node_modules <node> /tmp/b9b-e2e/run.cjs`.

## Fresh-session notes

- Temp database `test_athena_b9b` on the shared MariaDB (127.0.0.1:3306, creds in the worktree
  `.env.local`); safe to drop or reuse. Recreate: `DROP/CREATE DATABASE`, then with
  `ATHENA_DATABASE_URL=<base>/test_athena_b9b ATHENA_FILES_DIR=/tmp/b9b-files HOME=/tmp/b9b-home`:
  `bin/athena migrate up` + `bin/athena user list` (the latter creates the root `administrator`;
  its password is the database password).
- Port 3052 was the test server; the server process was stopped before handover. Ports 3000 and
  3100 belong to other workers — do not touch. I did not start MariaDB itself.
- Harness: `/tmp/b9b-e2e/run.cjs` (playwright-core via `NODE_PATH=/tmp/b9a-e2e/node_modules`,
  system Chrome, storage at `/tmp/b9b-e2e/storage.json`). `/tmp` may be wiped by the restart —
  the script is also fully described by the verified flow list above.
- Disk filled to 100% mid-task (shared machine); I freed ~1GB by deleting only my own disposable
  caches (`/tmp/b9a-home`, `/tmp/b9a-files`, `/tmp/athena-gocache`). If builds fail with
  no-space errors again, look for other disposable caches before touching anyone's work.
- `hc` was already broken before the restart announcement (registry writer stopped, daemon
  processes still alive). Do not restart Herdr daemons yourself; that is shared infra.
