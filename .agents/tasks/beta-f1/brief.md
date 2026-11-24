# Brief: F1 view tables like shadcn (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-f1, branch feat/f1-tables (from latest main).
Read: tasks.md F1 (exact spec); docs/design/athena-design-system.md; the `impeccable` and `emilkowalski-motion` skills (load via read_skill — F1 explicitly follows them); the Records table components (find them: table, columns, footer).

CRITICAL ORDERING (night-shift file reservation): C1 6.10 owns `RecordsPage.vue` and the records table components until it merges — F1 rebases on it. FIRST step is recon:
1. Recon: map the exact files F1 must touch; diff them against 6.10's unmerged branch (`feat/c1-voucher`, commit d13f4e0): `git diff main...d13f4e0 --name-only` vs your file list.
2. If DISJOINT: build all of F1, say so in every report (list both file sets).
3. If OVERLAPPING: build ONLY the non-overlapping parts (tests, footer component if separate, docs); hold overlapping edits for 6.10's merge and SAY master "F1: PARTIAL <built vs held>" — do not touch 6.10's files.

Scope (from tasks.md F1): consistent cell padding (nothing flush to any edge incl. checkbox + row-actions); one alignment per column shared by header and cells (text left, numbers/money right with tabular figures, dates left, thumbnails fixed-size vertically centered); hairline row separators; narrow right-aligned actions column; no filler column after the last field; header labels from field labels ("Image" not "image"); selection count footer ("0 of 5 selected"). Check 375/768/1280/1440 in both themes (screenshots to /tmp, paths in report).

Files allowed: the table components + their tests + footer (exact set from recon). NOT allowed until 6.10 merges: `RecordsPage.vue` itself if 6.10 touches it (ask master first in ANY case); App.vue (ask master); api.ts (append-only; you should not need it); protos, server logic, migrations, gate.go.

Rules: design-system posture (tokens, no literals, Satori/Inter scope, reduced-motion); telemetry off unless the endpoint is set. Throwaway Workspaces/test_athena_* DBs only, never the dev database; fake HOME for any server. Browser ports/DB: ask master for a pair before your first browser run (parallel passes run tonight — never reuse another lane's). Full suites and browser passes only through .agents/bin/suite (it queues; never bypass the lock). Package tests (one Vitest file) may run directly. Commit on your own branch feat/f1-tables (one commit per coherent piece; restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents. Rebase on main before reporting PASS and say if the rebase changed anything.
Checks: `CI=true pnpm --dir web run typecheck`, the touched Vitest files, `vite build`; one `make test` through suite at the end; full browser pass (4 widths x 2 themes) through suite.
Report: say master "F1: PASS or PARTIAL or BLOCKED <one line>" + commits + screenshot paths + the recon file lists. Then wait for instructions.
