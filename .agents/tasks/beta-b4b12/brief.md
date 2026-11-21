# Brief: B4 and B12 server (Muse Spark, taking over from Codex)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b4b12-server, branch feat/b4b12-server. Work only there.
**Handover:** Codex started this task and hit its usage limit at 12:50 on 2026-11-21. The worktree holds its partial, uncommitted, unverified work. Start with `git status` and `git diff`. Keep what is correct and matches this brief, fix or remove what is not, then finish. Note in your report what you kept and what you changed.

Read first: AGENTS.md; .kiro/specs/athena-beta/requirements.md (R4); design.md sections 3 and 5; tasks.md B4 and B12; docs/design/athena-design-system.md sections 2, 17 and 18.

Numbers assigned by the master: migration 15 `notifications`; ADR-0024 Notifications; ADR-0025 Workspace appearance (only if the storage is new durable data; if it reuses the G5 Settings chain, record that in ADR-0024's sibling note instead and say so).

## B12 server: notifications
- Migration 15: `athena_notifications` (id UUIDv7, recipient_user_id, kind, subject_type, subject_id, subject_label, actor_user_id nullable, params JSON with no Record values, created_at, read_at nullable), with indexes for recipient plus unread and recipient plus created.
- `proto/athena/v1/notifications.proto`: `NotificationService` with List (keyset paging, unread filter), UnreadCount, MarkRead (ids), MarkAllRead. Every procedure gets a `procedurePolicies` entry in gate.go: signed-in, own notifications only. Run `make generate`.
- Write notifications in the same transaction as their event, from the owning service:
  - auth: invite accepted (to the inviter), Role or Assignment changed (to the affected user), sign-in failed 3 or more times or locked (to the user), session ended by someone else (to the user);
  - rules: a save stopped by a validation or guard Rule (to the acting user);
  - lifecycle: a document submitted that needs your approval, if an approval concept exists, otherwise skip and say so;
  - ledger: a period closed (to users holding ledger:write).
- The text is built in Go from kind and params into everyday words (design-system section 8). Never include passwords, tokens or Record field values.
- CLI: `athena notifications list|read|read-all [--user name]`, operator mode.
- Tests: one per source, plus paging, permission (cannot read another user's), mark read.

## B4 server: appearance
- Store the Workspace accent: preset `ink`/`red`, or custom accent plus up to five chart colors as OKLCH strings. Reuse the G5 Settings chain (`internal/ledger/settings.go`) at Workspace scope if it fits; otherwise a small Workspace settings table in migration 15.
- `WorkspaceService` (new proto `workspace.proto`): GetAppearance (no sign-in needed, so the sign-in page can theme itself; returns only appearance), SetAppearance (permission `settings:appearance`, added to `auth.SystemPermissions`). Validate the OKLCH format and contrast: text on the accent at least 4.5:1 against the light and dark foregrounds. On failure, return the nearest passing lightness in the error detail.
- CLI: `athena workspace appearance show|set --preset ink|red | --accent "oklch(...)" [--chart1 ...]`.
- Tests: storage round trip, permission, contrast pass and fail with the suggested value.

## Rules
- Follow the batch-2 conventions: policies in gate.go, `make generate` twice with no diff, spans without values, ADR Proposed, decision log and docs/README rows, README commands.
- Do not touch web/src except the generated `web/src/gen/` files.
- Use throwaway test databases only (make test creates `test_athena_*`). Do not write to the dev database in .env.local except through tests.
- Checks: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test (sets ATHENA_TEST_DATABASE_URL), make generate twice with no diff.
- Do not commit, merge, push or start agents.
- When done or stuck: write the report to `/Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/beta-b4b12/report.md` (PASS or BLOCKED, files changed, command output), then run `hc send t6 "B4B12 SERVER: <PASS|BLOCKED>, report at .agents/tasks/beta-b4b12/report.md"`.
