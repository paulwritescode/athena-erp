# Brief: L5 follow-ups batch 1, Desk polish (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-fu, branch feat/fu-batch-1 (from latest main).
Read: tasks.md "Follow-ups from merged slices"; .kiro/steering/athena-design.md; docs/design/athena-design-system.md (the Desk rules that decide each fix).

Scope: Desk-only fixes, one small fix per commit. Work these bullets, in this order, skipping anything that needs a server change (report the skip):
1. B9a: Delete next to Save in the editor header; editor title not an h1; field-row action icons cramped; Required checkbox alignment; dead `.records-layout` rule (style.css:178).
2. B4: unset custom chart swatches show black; "Selected page" preview lacks the accent; Security section intro copy too playful.
3. B4b: accent dots render light primaries on dark pages; dark ramp washes toward white for near-white primaries; CLI usage still says `--preset ink|red` with no chart-palette flag; ramp-fallback/Custom-swatch ink dots and `text-[11px]` month labels in preview.
4. B9b: stale "draft" comments in `useDeskRoute.ts`; one-way Change (no return to auto-derive key); duplicate-name Enter shows no inline error until blur.
5. B2a slice 1 (Desk items only): toolbar search text starts under the icon (`pl-9` loses to Input `px-3`); dead `.entries-grid`/`.entries-resizer` CSS; hardcoded Draft/Submitted/Cancelled in table (use the glossary); clearing filters fires a duplicate list request; `pressTimer` not cleared on unmount; search width 448 px vs 480 px in §21. The API gaps (totals, free-text search, sort params, bulk-delete RPC) are server work — do NOT build them.

Files allowed: web/src/** EXCEPT `web/src/pages/RecordTypesPage.vue`, `web/src/components/FieldSettingsDialog.vue`, `web/src/fieldRules.ts` (lane L3 owns these right now — do not touch), `web/src/App.vue` (ask master first), `web/src/api.ts` (append-only; you should not need it); plus `cmd/athena/*` usage-text only for the B4b CLI wording fix. No migrations, no proto, no gate.go, no server logic.

Rules: one fix per commit with a message naming the bullet (e.g. "fix(desk): B9a editor title is an h1"); restore internal/webui/dist/.gitkeep before committing. Screenshot each changed screen at 1280 px in both themes into /tmp (paths in your report); full 4-width matrix only if a layout shifts. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server. Full suites and browser passes only through .agents/bin/suite (it queues; never bypass the lock). Package tests (one Vitest file) may run directly. Commit on your own branch feat/fu-batch-1; you never merge, push or start agents. Rebase on main before reporting PASS and say if the rebase changed anything.
Checks: `CI=true pnpm --dir web run typecheck`, the touched Vitest files, `vite build`; one full `make test` through suite at the end.
Report: say master "FU BATCH 1: PASS or BLOCKED <one line, fixes done + skipped>" + the list of commits + screenshot paths. Then wait for instructions.
