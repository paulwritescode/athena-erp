# Brief: C10 G15 integration surface (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c2, branch feat/c10-integrations (fresh from latest main — cut it after secrets slice 5 merges; verify `git log main..HEAD` empty before starting).
Read: tasks.md C10 (beta line: attachments reuse B9 Image storage); .kiro/specs/athena-batch-2/tasks.md G15 (15.1–15.6, the source scope); batch-2 design.md G15 + Requirements 15.1–15.10; internal/secrets (merged, use for provider keys); internal/files + B9 Image storage (read which one attachments reuse — beta says B9 Image).

Numbers assigned by the master: migration **25** (`integrations` tables; reserved in .agents/stylus/migrations.md); ADR-**0035** "Integration surface" (Proposed — YOU write it). The batch-2 text names migration 23 + ADR-0032 — both TAKEN (F4/C7); use 25/0035. Never pick a number yourself.

Acceptance→slice mapping (every G15 line covered):
- 15.1 migration+ADR → slice 1.
- 15.2 provider inbox (HMAC key window, dedupe, evidence, Operation_ID mapping, secrets for keys) → slice 2.
- 15.3 Property 26 (provider event verification) → slice 3.
- 15.4 attachments/search/comments/import (+ store Search dialect, abandoned-upload job) → slice 4. Attachments reuse B9 Image storage per the beta C10 line (not files/path.go blob rules unless B9 sits on them — verify and say which).
- 15.5 integrations.proto + server + gate + CLI inbox/attachments + RecordsPage attachments/comments → slice 5. GATE: do NOT touch RecordsPage.vue until master says GO (another lane works nearby Desk code; backend + CLI first).
- 15.6 docs → slice 6 with the checkpoint (generate stable, full suite, typecheck/build, browser pass of inbox/attachments in both themes).

Slices, each merging on its own (report after each with say master "C10 SLICE n: PASS or BLOCKED <one line>"; one worker, sequential; rebase on main before reporting PASS):
1. Migration 25 + ADR-0035 (Proposed) + decision-log/README rows.
2. `internal/integrations/` inbox: HMAC verification with key window, event dedupe, evidence records, Operation_ID mapping; keys via internal/secrets.
3. Property 26 in `internal/integrations/inbox_test.go` (new). Real properties, not example lists.
4. `internal/integrations/{attachments,search,comments,import}.go` + store Search dialect (mariadb.go) + abandoned-upload job in internal/jobs.
5. Proto + generate twice no diff + server + gate (append entries at end) + CLI + Desk (after GO).
6. Docs + checkpoint.

Files allowed: the files above + their tests + the ADR + docs. NOT allowed: any other lane's files (events.proto/jobs L2-secrets done but merged — do not touch; ledger/stock protos; record_types.proto), App.vue (ask master), RecordTypesPage/FieldSettingsDialog/fieldRules (L3-owned), api.ts (append-only; ask first).

Rules: batch-2 conventions (gate policies for new procedures, generate twice no diff, property tests where demanded, spans without values, no secret material in spans/logs/errors). Telemetry off unless set. Throwaway Workspaces/test_athena_* DBs only; fake HOME for any server. Full suites/browser only through the MAIN-REPO .agents/bin/suite (absolute path); package tests direct. Suite -p default applies, no manual flag. Commit on feat/c10-integrations; never merge/push/spawn.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test through suite for slice 1+ (affected where it applies), make generate stable; web typecheck/test/build for slice 5.
