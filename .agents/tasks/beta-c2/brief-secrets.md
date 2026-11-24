# Brief: C2 secrets and retention, 8.8–8.10 + rotate (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-c2, branch feat/c2-events-jobs (REBASE on latest main first — the 7 split commits are merged and should drop; verify `git log main..HEAD` empty before starting).
Read: tasks.md C2 (Open: secrets line); .kiro/specs/athena-batch-2/tasks.md G8.8–8.10 (this task); batch-2 "Conventions"; design.md G8 secrets/retention + Data Models rows; Requirements 8.13–8.19, 8.22; ADR-0028 (merged); internal/events + internal/jobs (merged split code — read what exists); internal/auth/service.go (delivery paths).

Numbers assigned by the master: migration **22** (secret refs/keys/disposal/holds tables; reserved in .agents/stylus/migrations.md); ADR-**0033** "Secrets and retention" (Proposed — YOU write it; L5 records only the Z4 row pointing here). No other migration/ADR. Never pick a number yourself.

Z4 RULE (binding, decided 2026-11-24 — bake it in exactly, do not redesign): retention is 7 years ledger/invoices/stock/audit (NO beta auto-delete of this set), guest personal data anonymised 90 days after the event (consent model for dietary data), Access Log 1 year, outbox/events 30 days, job runs 90 days. Workspace key comes from env or OS keychain, is owner-held, and is NEVER stored in the database or backups. `athena secrets rotate` rotates keys.

Slices, each merging on its own (report after each with say master "C2 SECRETS SLICE n: PASS or BLOCKED <one line>"; one worker, sequential; rebase on main before reporting PASS):
1. Migration 22 + ADR-0033 (Proposed, with the Z4 text above) + decision-log/README rows.
2. 8.8: `internal/secrets/` (new): references, provider interface with a local file provider, key versions, AES-256-GCM with authenticated context. Redaction in logs and spans (no secret material anywhere observable).
3. 8.9: Property 16 (field encryption round trip + binding) in `internal/secrets/keys_test.go` (new). Real properties, not example lists.
4. 8.10: lifetimes, holds, disposal: carry lifetime attributes in `internal/server/attributes.go` + `internal/server/record_types.go` (append-only edits; L3-C4 owns these files but has no live worker — rebase carefully); new `internal/jobs/disposal.go` (preview, holds, disposal, restore reapply) honoring the Z4 schedule (7yr set never auto-deleted in beta).
5. CLI + docs: `cmd/athena/secrets.go` (new) + `athena secrets rotate` (per Z4 custody); overview/README/AGENTS.md/log rows ("secrets are references only"). No Desk in this task (G18 owns secret-reference bindings later).

Files allowed: the files above + their tests + the ADR + docs. NOT allowed: any other lane's files (ledger.proto L1, record_types.proto L3-C4, stock.proto L3-C7 — do not touch), App.vue (ask master), RecordTypesPage/FieldSettingsDialog/fieldRules (L3-owned Attrs exception above covers attributes.go/record_types.go ONLY for lifetime carries), RecordsPage.vue (6.10 owns it now), api.ts (append-only; you should not need it).

Rules: batch-2 conventions (gate policies for new procedures, generate twice no diff, property tests where demanded, spans without values, NO secret material in spans/logs/errors). Telemetry off unless the endpoint is set. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server. Full suites and browser passes only through .agents/bin/suite (it queues; never bypass). Package tests may run directly. Commit on feat/c2-events-jobs (restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents. If the remaining work splits into disjoint pieces or a slice runs over about 90 minutes, ask master about splitting (AGENT_RULES.md).
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test through suite for slice 1+ (affected for the rest where it applies), make generate stable; web typecheck/test/build only if web/ changes (not expected).
