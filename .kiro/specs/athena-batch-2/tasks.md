# Implementation Plan: Athena batch 2

## Overview

Eighteen groups, G1 to G18, run in order. Each group extends existing packages and ends with a checkpoint. Every file path is marked (existing) or (new). A new file or package is created only where no existing file covers the job, and the task says why. The language is Go for the Core, TypeScript for the Desk, as in the design.

Conventions that apply to every group:

- Every new ConnectRPC procedure gets a `procedurePolicies` entry in `internal/server/gate.go` (existing).
- Proto edits are followed by `make generate`, which regenerates `gen/athena/v1/` and `web/src/gen/` (existing generated files).
- Every capability has an `athena <command>` in `cmd/athena/`, calling the Go package directly.
- New system tables are new numbered entries in `internal/server/sysmigrations.go` (existing), never edits to applied ones. Next free number: 12.
- ADR numbers start at 0017, the next free number after 0016. ADRs are added as Proposed with `docs/decisions/adr-template.md`, listed in `docs/README.md` (existing) and `docs/decisions/decision-log.md` (existing).
- Property tests use `pgregory.net/rapid` in Go and `fast-check` in Vitest, 100 iterations minimum, tagged `Feature: athena-batch-2, Property N: <text>`.
- Spans carry ids, counts, durations and outcome codes only.
- Each group checkpoint: `make generate` leaves no diff on a second run, `make test` is green against MariaDB, `CI=true pnpm --dir web run typecheck` and `CI=true pnpm --dir web run build` pass, and a browser pass runs in light and dark themes.

## Tasks

- [x] 1. G1 Users & Roles page and the revamp access model
  - [x] 1.1 Migration 12 and ADR-0017
    - Add migration 12 `access_model` to `internal/server/sysmigrations.go` (existing): `athena_challenges`, `athena_role_assignments`, user `state`, `generation`, `invite_delivery`, nullable `password_hash`, role `requires_scope` and `system`, session device columns. Copy `athena_user_roles` rows into assignments with empty Scope and no dates, and mark existing users `active`.
    - Create `docs/decisions/adr-0017-access-model-v2.md` (new, required by AGENTS.md for a security change), amending ADR-0014. Update `docs/README.md` and `docs/decisions/decision-log.md` (existing).
    - Add a migration test to `internal/server/sysmigrations_test.go` (existing) that a database made by the current build upgrades without losing access.
    - _Requirements: 1.8, 1.13_
  - [x] 1.2 Add the dev dependency for property tests
    - Add `pgregory.net/rapid` to `go.mod` and `go.sum` (existing); add `fast-check` to `web/package.json` and `web/pnpm-lock.yaml` (existing) with the pinned pnpm.
    - _Requirements: 1.16_
  - [x] 1.3 Implement Challenges
    - Create `internal/auth/challenge.go` (new; no existing file holds single-use credentials) for issue, digest, consume-once, attempt counting and activation codes. Edit `internal/auth/service.go` (existing) `CreateUser` to create an invited user with no password, and `Login` to refuse users that are not active.
    - Edit `internal/auth/accesslog.go` (existing) with Challenge events that never include the value.
    - _Requirements: 1.1, 1.2, 1.3, 1.5, 1.6, 1.9_
  - [x]* 1.4 Write property test for Challenge single use
    - **Property 1: Challenge single use**
    - Add to `internal/auth/auth_test.go` (existing), using concurrent completions against `internal/testdb`.
    - **Validates: Requirements 1.3, 1.4, 1.5**
  - [x] 1.5 Implement Assignments and whole-Assignment evaluation
    - Create `internal/auth/assignment.go` (new) for Assignment, Scope and dates. Edit `internal/auth/principal.go` (existing): add `Assignments`, rewrite `Can`, `Level`, `Actions`, `Scope` and `MustSeparateDuties` to test whole Assignments, and return disjuncts from `Scope`. Edit `Service.loadPrincipal` in `internal/auth/service.go` (existing).
    - Edit `internal/server/access.go` (existing) to compile Scope disjuncts into the existing row-condition SQL, and to check old and new rows on update.
    - _Requirements: 1.13, 1.14, 1.15, 1.17_
  - [x]* 1.6 Write property test for scope union
    - **Property 3: Scope union by whole Assignment**
    - Add to `internal/auth/auth_test.go` (existing), comparing `Principal.Can` with a naive model.
    - **Validates: Requirements 1.13, 1.14, 1.15, 1.16, 1.17**
  - [x] 1.7 Replace the Owner shortcut and add the last-owner guard
    - Edit `internal/auth/principal.go` (existing) to remove `Owner` short-circuits. Add permissions `access:ownership`, `access:users`, `access:roles`, `access:assign`, `access:recover`, `access:sessions`, `access:explore` to `SystemPermissions`. Edit `internal/auth/service.go` (existing): `UpdateUser`, `SaveRole`, `DeleteRole` and assignment changes run the guard in one transaction.
    - Seed system Roles Platform Administrator, Instance Owner, System Manager and the business Role templates without Grants (in `internal/auth/service.go`, existing).
    - Implement delegation checks over the whole proposed diff, and the privileged-recovery password re-check.
    - _Requirements: 1.10, 1.11, 1.12, 1.20, 1.21, 1.22, 1.27_
  - [x]* 1.8 Write property test for the last-owner guard
    - **Property 2: Last owner**
    - Add to `internal/auth/auth_test.go` (existing) with concurrent owner removals.
    - **Validates: Requirements 1.11, 1.12**
  - [x] 1.9 Extend AuthService proto and handlers
    - Edit `proto/athena/v1/auth.proto` (existing): reserve `CreateUserRequest` field 3 and name `password`, and `ResetPasswordRequest` field 2 and name `new_password`; add `InviteUser`, `CompleteChallenge`, `ResendInvite`, `StartReset`, `ListAssignments`, `SaveAssignment`, `RevokeAssignment`, `ExplainAccess`, `ListSessions`, `RevokeSession`, `TransferOwnership`. Run `make generate`.
    - Edit `internal/server/auth_rpc.go` (existing) and `internal/server/gate.go` (existing) with a policy per procedure using the new permissions.
    - _Requirements: 1.1, 1.7, 1.19, 1.23_
  - [x] 1.10 Implement the Permission Explorer
    - Create `internal/auth/explain.go` (new; no existing file explains decisions) returning the matching Assignment or the failing check per Assignment, including separation-of-duty conflicts. Wire `ExplainAccess` in `internal/server/auth_rpc.go` (existing).
    - _Requirements: 1.18, 1.25_
  - [x] 1.11 Implement session management
    - Edit `internal/auth/service.go` (existing) with list, revoke-own and revoke-other, and revoke-on-suspend and revoke-on-reset.
    - _Requirements: 1.6, 1.22, 1.23_
  - [x] 1.12 Add CLI commands
    - Edit `cmd/athena/access.go` (existing) with `user invite|resend|reset|suspend|reactivate|sessions` and `role assign|revoke|explain`; update help in `cmd/athena/main.go` (existing); extend `cmd/athena/access_test.go` (existing). Make `athena user create --owner` use the guarded bootstrap.
    - _Requirements: 1.9, 1.26_
  - [x] 1.13 Build the Users & Roles page
    - Create `web/src/pages/AccessPage.vue` (new; no existing page covers access) with Users, Roles and Sessions tabs, Role chips with Scope pickers, date inputs, preview and diff dialog and Explorer panel. Edit `web/src/App.vue`, `web/src/api.ts`, `web/src/types.ts` (existing) for the Build nav item shown with `access:users` or `access:roles`, and the first-login completion screen in `web/src/pages/SignInPage.vue` (existing).
    - Create `web/src/assignmentPreview.ts` and `web/src/assignmentPreview.test.ts` (new, pure TypeScript with Vitest).
    - _Requirements: 1.18, 1.19, 1.24_
  - [x] 1.14 Update documentation
    - Edit `AGENTS.md`, `docs/architecture/overview.md`, `README.md` (existing): owners no longer hold every permission, invite-only users, sign-in remains local only, multi-factor sign-in OPEN.
    - _Requirements: 1.10, 1.28_
  - [x] 1.15 Checkpoint - Ensure all tests pass
    - `make generate` with no diff, `make test`, web typecheck and build, browser pass in both themes for invite, first login, assignment preview and explorer. Ensure all tests pass, ask the user if questions arise.

- [x] 2. G2 PostgreSQL adapter, implemented but untested
  - [x] 2.1 Implement the PostgreSQL dialect
    - Create `internal/store/postgres.go` (new; the seam in `store.go` has one implementation) covering every `Dialect` method. Edit `internal/store/store.go` (existing) only if a method is missing for PostgreSQL.
    - _Requirements: 2.1, 2.8_
  - [x] 2.2 Wire the driver
    - Edit `internal/server/server.go` and `internal/workspace/{workspace,templates}.go` (existing) so driver `postgres` opens through pgx stdlib with `otelsql`. Add `github.com/jackc/pgx/v5` to `go.mod` and `go.sum` (existing), noting the Lambda-free single-binary fit in ADR-0019 (0018 was taken by the root Administrator).
    - _Requirements: 2.2, 2.9_
  - [x] 2.3 Write dialect unit tests
    - Create `internal/store/postgres_test.go` (new) asserting SQL strings, error classification by SQLSTATE and quoting. No database connection.
    - _Requirements: 2.3_
  - [x]* 2.4 Write property test for identifier shortening
    - **Property 4: Identifier shortening**
    - Add to `internal/store/store_test.go` (existing).
    - **Validates: Requirements 2.6**
  - [x] 2.5 Update CLI and documentation
    - Edit `cmd/athena/main.go` (existing) so `athena new --driver postgres` prints an untested warning. Create `docs/decisions/adr-0019-postgresql-adapter.md` (new; 0018 was taken by the root Administrator), amending ADR-0005 and ADR-0002. Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/adr-0005-database-dialect-layer.md`, `docs/decisions/decision-log.md` (existing) to say "implemented but untested" and resolve O-001 only as "adapter exists".
    - _Requirements: 2.4, 2.5, 2.7, 2.9_
  - [x] 2.6 Checkpoint - Ensure all tests pass
    - `make test` on MariaDB, `go vet ./...`, web typecheck and build. Ensure all tests pass, ask the user if questions arise.

- [x] 3. G3 Starlark memory guard
  - [x] 3.1 Add limits and the host guard
    - Edit `internal/rules/rules.go` (existing): extend `Limits` and `DefaultLimits`, add codes `memory_limit`, `row_limit`, `cascade_limit`. Edit `internal/rules/builtins.go` and `internal/rules/value.go` (existing) so every builtin that builds values counts them and checks sizes. Add a static AST check for the repeat operator before a run in `rules.go`.
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_
  - [x] 3.2 Apply limits on both paths
    - Edit `internal/server/rules_engine.go` and `internal/server/rules.go` (existing) so saved runs and `SimulateRule` use one limits value, and enforce row and cascade limits there. Record the code in the Rule Run log.
    - _Requirements: 3.3, 3.4, 3.6_
  - [x] 3.3 Write tests for the guard
    - Extend `internal/rules/rules_test.go` and `internal/server/rules_test.go` (existing) with the `[0] * 10**9` example, cascade depth and row examples.
    - _Requirements: 3.5, 3.6_
  - [x]* 3.4 Write property test for resource caps
    - **Property 5: Resource caps**
    - Add to `internal/rules/rules_test.go` (existing).
    - **Validates: Requirements 3.1, 3.2, 3.3, 3.4**
  - [x] 3.5 Update ADR and documentation
    - Create `docs/decisions/adr-0020-starlark-resource-guard.md` (new; 0019 was taken by the PostgreSQL adapter) amending ADR-0016. Edit `docs/decisions/adr-0016-starlark-rules-host.md`, `AGENTS.md`, `docs/architecture/overview.md`, `docs/decisions/decision-log.md` (existing). Status stays Proposed.
    - _Requirements: 3.7_
  - [x] 3.6 Checkpoint - Ensure all tests pass
    - `make test` green, web typecheck and build, browser check that the Rules page shows the new codes in both themes. Ensure all tests pass, ask the user if questions arise.

- [x] 4. G4 Atomic child and parent Record Type save
  - [x] 4.1 Add the group operation to the Publisher
    - Edit `internal/server/record_types.go` (existing): `save_group` journal operation, compensating steps, and resume or rollback in `resumeOperations`. Edit `internal/server/sysmigrations.go` only if the journal needs a column (add a numbered migration then, using the next free number).
    - _Requirements: 4.1, 4.2, 4.3_
  - [x] 4.2 Extend the API and the Desk
    - Edit `proto/athena/v1/record_types.proto` (existing) so `SaveRecordTypeRequest` carries the parent and its expected revision; run `make generate`. Edit `web/src/App.vue` and `web/src/api.ts` (existing) to send one request and show one toast.
    - _Requirements: 4.4, 4.5_
  - [x] 4.3 Write fault-injection tests
    - Extend `internal/server/record_types_test.go` (existing): fail the parent, stop between members, resume at startup.
    - _Requirements: 4.1, 4.2, 4.3_
  - [x] 4.4 Add ADR and documentation
    - Create `docs/decisions/adr-0021-atomic-record-type-group-save.md` (new; 0020 was taken by the Starlark guard) stating compensation, not one transaction, because MariaDB commits DDL implicitly. Edit `.agents/tasks/erp-kernel-roadmap.md` (existing) to remove the tracked item, and `docs/architecture/overview.md`.
    - _Requirements: 4.1_
  - [x] 4.5 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass saving a parent and child in both themes. Ensure all tests pass, ask the user if questions arise.

- [x] 5. G5 Ledger foundation
  - [x] 5.1 Migration 13 and ADR-0022 (0021 was taken by the pair-save group)
    - Add migration 13 `ledger_foundation` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0021-ledger-foundation-and-kernel-masters.md` (new) recording which masters the kernel owns and the relation to D-007 (OPEN).
    - _Requirements: 5.14_
  - [x] 5.2 Implement masters, periods and currencies
    - Create `internal/ledger/` (new package; none covers ledgers) with Company, Branch, Department, Cost Centre, Dimension Definition, Fiscal Year and Period (soft and hard close), Currency and Exchange Rate. Edit `internal/server/kinds.go` and `internal/server/kind_migration.go` (existing) so ledger-kind tables are kernel-owned, and `internal/server/records.go` (existing) to reject direct writes to them.
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.8_
  - [x]* 5.3 Write property test for period non-overlap
    - **Property 6: Period non-overlap**
    - Create `internal/ledger/periods_test.go` (new).
    - **Validates: Requirements 5.3**
  - [x] 5.4 Implement Accounts and Account Determination
    - Create `internal/ledger/{accounts,determination}.go` (new) with tree parents, leaf checks and specificity resolution that names tied mappings.
    - _Requirements: 5.6, 5.7_
  - [x] 5.5 Implement Settings with scope chains
    - Create `internal/ledger/settings.go` (new) with `Resolve`, merge rules, effective-time resolution, diff preview and prospective publish.
    - _Requirements: 5.9, 5.10, 5.11_
  - [x]* 5.6 Write property test for determination and Settings
    - **Property 7: Determination specificity and Setting resolution**
    - Create `internal/ledger/determination_test.go` (new).
    - **Validates: Requirements 5.7, 5.9, 5.10**
  - [x] 5.7 Add LedgerService setup procedures and CLI
    - Create `proto/athena/v1/ledger.proto` (new; no existing service fits), run `make generate`; create `internal/server/ledger.go` (new handler); edit `internal/server/server.go` and `gate.go` (existing); create `cmd/athena/ledger.go` (new) and edit `cmd/athena/main.go` (existing).
    - _Requirements: 5.12, 5.13_
  - [x] 5.8 Build the setup pages
    - Create `web/src/pages/LedgerSetupPage.vue` (new) for Company setup, Periods, Currencies, Accounts and the readiness view; edit `web/src/App.vue`, `web/src/api.ts`, `web/src/types.ts` (existing).
    - _Requirements: 5.12, 5.15_
  - [x] 5.9 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `AGENTS.md`, `docs/decisions/decision-log.md` (existing). Mark jurisdiction, chart and tax OPEN.
    - _Requirements: 5.14, 5.15_
  - [x] 5.10 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass of setup pages in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 6. G6 Posting engine
  - [x] 6.1 Migration 14 and ADR-0023 (0022 was taken by the ledger foundation)
    - Add migration 14 `posting_engine` to `internal/server/sysmigrations.go` (existing): `athena_operations`, `athena_posting_groups`, `athena_posting_log`, `l_gl`. Create `docs/decisions/adr-0022-posting-engine-and-operations.md` (new). OPEN: backdating window and book policy.
    - _Requirements: 6.4, 6.7_
  - [ ] 6.2 Implement plan validation and the writer
    - Create `internal/posting/` (new) with plan validation (balance by company and book, leaf accounts, periods, dimensions, control accounts), the single-transaction writer and balance deltas. Reuse `internal/types/{money,allocate}.go` and `internal/audit/audit.go` (existing).
    - _Requirements: 6.1, 6.3, 6.4, 6.5, 6.6, 6.17_
  - [ ]* 6.3 Write property test for balanced postings
    - **Property 8: Balanced postings**
    - Create `internal/posting/posting_test.go` (new).
    - **Validates: Requirements 6.1, 6.2, 6.6**
  - [ ] 6.4 Implement the Operation_ID contract
    - Add `internal/posting/operations.go` (new): canonical payload hash, replay result, conflict, and uniqueness per voucher revision, event and book.
    - _Requirements: 6.7, 6.8, 6.10_
  - [ ]* 6.5 Write property test for idempotent replay
    - **Property 9: Idempotent replay**
    - Add to `internal/posting/posting_test.go` (new in 6.3), with concurrent replays.
    - **Validates: Requirements 6.7, 6.8, 6.9, 6.10**
  - [ ] 6.6 Implement reversal and closed-period correction
    - Add `internal/posting/reverse.go` (new): mirror stored rows, flag `is_reversal`, correction in an allowed period.
    - _Requirements: 6.11, 6.13_
  - [ ]* 6.7 Write property test for reversal mirrors
    - **Property 10: Reversal mirrors**
    - Add to `internal/posting/posting_test.go`.
    - **Validates: Requirements 6.11, 6.12**
  - [ ] 6.8 Hook into the lifecycle and the Rule host
    - Edit `internal/lifecycle/lifecycle.go` and `internal/server/lifecycle.go` (existing) to call `Post` and the mirror. Edit `internal/rules/rules.go` and `internal/server/rules_engine.go` (existing) to add the Posting rule type with the plan API, fixture replay, plan diff and named approval in `internal/server/rules.go` (existing).
    - _Requirements: 6.4, 6.14, 6.15, 6.16_
  - [ ] 6.9 Add preview, status, log procedures and CLI
    - Edit `proto/athena/v1/ledger.proto` (new in 5.7): `PreviewPosting`, `GetPostingStatus`, `ListPostingLog`, `ListLedgerRows`; run `make generate`; edit `internal/server/ledger.go`, `gate.go` (existing). Create `cmd/athena/post.go` (new).
    - _Requirements: 6.14, 6.18, 6.19_
  - [ ] 6.10 Add the Desk voucher view
    - Edit `web/src/pages/RecordsPage.vue` (existing): Preview posting dialog, ledger rows and Posting_Log on a submitted voucher; Sonner toasts for blocked posts.
    - _Requirements: 6.14, 6.15_
  - [ ] 6.11 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `AGENTS.md`, `docs/decisions/decision-log.md` (existing); mark Posting rules not production-ready until accepted.
    - _Requirements: 6.16_
  - [ ] 6.12 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass: preview, submit, double-click submit, cancel, in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 7. G7 Party ledger, settlements and realised FX
  - [ ] 7.1 Migration 15 and ADR-0023
    - Add migration 15 `party_ledger` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0023-party-ledger-and-settlements.md` (new).
    - _Requirements: 7.1, 7.2_
  - [ ] 7.2 Implement open items and applications
    - Create `internal/posting/settlement.go` (new file in the existing posting package) with open items, line-level applications, locks, ceiling and unapplied cash. Use `internal/types/allocate.go` (existing).
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.7_
  - [ ]* 7.3 Write property test for allocation sums
    - **Property 11: Allocation sums**
    - Add to `internal/types/types_test.go` (existing) and `internal/posting/settlement_test.go` (new).
    - **Validates: Requirements 7.3, 17.7, 17.10**
  - [ ] 7.4 Implement realised FX and settlement reversal
    - Edit `internal/posting/settlement.go` (7.2) to post gain or loss through the G6 engine and to mirror applications on cancel.
    - _Requirements: 7.5, 7.6_
  - [ ]* 7.5 Write property test for party ledger and control account
    - **Property 12: Party ledger equals control**
    - Add to `internal/posting/settlement_test.go` (new in 7.3).
    - **Validates: Requirements 7.1, 7.2, 7.6, 7.8**
  - [ ] 7.6 Add procedures, CLI and Desk view
    - Edit `proto/athena/v1/ledger.proto`: `ApplySettlement`, `ReverseSettlement`, `ListOpenItems`; `make generate`; edit `internal/server/ledger.go`, `gate.go`. Create `cmd/athena/settle.go` (new). Edit `web/src/pages/RecordsPage.vue` (existing) with an open-items panel on payment Records.
    - _Requirements: 7.9_
  - [ ] 7.7 Write the USD 100 fixture test
    - Add a golden test in `internal/posting/settlement_test.go` for the 130 to 132 receipt giving Bank 13,200, Receivable 13,000, Gain 200.
    - _Requirements: 7.5_
  - [ ] 7.8 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 7.1_
  - [ ] 7.9 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 8. G8 Jobs, outbox, events, workflows, secrets and lifetimes
  - [ ] 8.1 Migration 16 and ADRs 0024 and 0025
    - Add migration 16 `events_jobs_secrets` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0024-events-jobs-and-workflows.md` and `docs/decisions/adr-0025-secrets-key-custody-and-lifetimes.md` (new). State the reordering of P16 before P14. OPEN: retention periods and key custody.
    - _Requirements: 8.1, 8.13, 8.16, 8.23_
  - [ ] 8.2 Implement events, outbox and inbox
    - Create `internal/events/` (new): envelope, producer write in the caller's transaction, inbox with effect keys, Pack-declared subscriptions. Edit `internal/posting` (existing) and `internal/lifecycle/lifecycle.go` (existing) to emit events on commit.
    - _Requirements: 8.1, 8.2, 8.3, 8.6, 8.12_
  - [ ]* 8.3 Write property test for inbox deduplication
    - **Property 13: Inbox deduplication**
    - Create `internal/events/events_test.go` (new).
    - **Validates: Requirements 8.3, 8.5**
  - [ ] 8.4 Implement the job runner and schedules
    - Create `internal/jobs/` (new): leases with expiry and fencing, backoff, owner assignment, recovery after restart, occurrence keys with timezone and catch-up. Edit `internal/server/server.go` (existing) to start workers.
    - _Requirements: 8.4, 8.5, 8.9, 8.10, 8.19_
  - [ ]* 8.5 Write property test for occurrence uniqueness
    - **Property 15: Occurrence uniqueness**
    - Create `internal/jobs/jobs_test.go` (new).
    - **Validates: Requirements 8.9, 14.1**
  - [ ] 8.6 Implement workflows and tasks
    - Create `internal/events/workflow.go` (new file in the new package): instances, waits, gated task creation, cycle budget.
    - _Requirements: 8.7, 8.8, 8.11_
  - [ ]* 8.7 Write property test for gated task creation
    - **Property 14: Gated task creation**
    - Add to `internal/events/events_test.go`.
    - **Validates: Requirements 8.8**
  - [ ] 8.8 Implement secret references and field encryption
    - Create `internal/secrets/` (new): references, provider interface with a local file provider, key versions, AES-256-GCM with authenticated context. Ensure redaction in logs and spans.
    - _Requirements: 8.13, 8.14, 8.15_
  - [ ]* 8.9 Write property test for field encryption
    - **Property 16: Field encryption round trip and binding**
    - Create `internal/secrets/keys_test.go` (new).
    - **Validates: Requirements 8.14, 8.15**
  - [ ] 8.10 Implement lifetimes, holds and disposal
    - Edit `internal/server/attributes.go` and `internal/server/record_types.go` (existing) to carry lifetime attributes; create `internal/jobs/disposal.go` (new) for preview, holds, disposal and restore reapply.
    - _Requirements: 8.16, 8.17, 8.18, 8.19_
  - [ ] 8.11 Deliver invites through the outbox
    - Edit `internal/auth/service.go` (existing) to enqueue invite and reset delivery WHERE a destination exists, and keep the manual code path otherwise.
    - _Requirements: 8.22_
  - [ ] 8.12 Add procedures, CLI and pages
    - Create `proto/athena/v1/events.proto` and `proto/athena/v1/settings.proto` (new); `make generate`; create `internal/server/events.go` and `internal/server/settings.go` (new handlers); edit `gate.go`. Create `cmd/athena/{jobs,events,secrets}.go` (new). Create `web/src/pages/JobsPage.vue` and `web/src/pages/SettingsPage.vue` (new); edit `web/src/App.vue`.
    - _Requirements: 8.20, 8.21_
  - [ ] 8.13 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `AGENTS.md`, `docs/decisions/decision-log.md` (existing). State secrets are references only.
    - _Requirements: 8.13, 8.23_
  - [ ] 8.14 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass of Jobs and Settings in both themes, crash-between-commit-and-dispatch test. Ensure all tests pass, ask the user if questions arise.

- [ ] 9. G9 Balances, Query compiler, Reports v1 and Report Runs
  - [ ] 9.1 Migration 17 and ADR-0026
    - Add migration 17 `balances_reports` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0026-balances-queries-and-report-runs.md` (new).
    - _Requirements: 9.1, 9.7_
  - [ ] 9.2 Implement Balances with watermarks
    - Edit `internal/posting/` (existing) to update Balances in the posting transaction. Create `internal/report/balances.go` (new) for generations, rebuild and atomic switch.
    - _Requirements: 9.1, 9.2_
  - [ ]* 9.3 Write property test for Balance rebuild
    - **Property 17: Balance rebuild**
    - Create `internal/report/balances_test.go` (new).
    - **Validates: Requirements 9.1, 9.2, 9.3**
  - [ ] 9.4 Implement the Query compiler
    - Create `internal/query/` (new) with typed Queries, grain checks and Scope disjuncts compiled at source, reusing the `internal/expr` SQL translation (existing).
    - _Requirements: 9.4, 9.5, 9.11_
  - [ ]* 9.5 Write property test for scoped queries
    - **Property 18: Scoped query equals filtered model**
    - Create `internal/query/query_test.go` (new).
    - **Validates: Requirements 9.4, 9.5, 9.14**
  - [ ] 9.6 Implement Reports and Report Runs
    - Create `internal/report/{report,run}.go` (new) with the four fixture Reports, Report_Run digest and provenance, drill-down and the stored high-water mark rerun.
    - _Requirements: 9.6, 9.7, 9.9, 9.10_
  - [ ]* 9.7 Write property test for Report Run reproducibility
    - **Property 19: Report Run reproducibility**
    - Create `internal/report/run_test.go` (new).
    - **Validates: Requirements 9.7, 9.8, 17.6**
  - [ ] 9.8 Add ReportService, CLI and Reports page
    - Create `proto/athena/v1/reports.proto` (new); `make generate`; create `internal/server/reports.go` (new); edit `gate.go`. Create `cmd/athena/report.go` (new). Create `web/src/pages/ReportsPage.vue` (new); edit `web/src/App.vue` and `AGENTS.md` (existing) for the Build list.
    - _Requirements: 9.12, 9.13, 9.14_
  - [ ] 9.9 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 9.12_
  - [ ] 9.10 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass running and reproducing a Report in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 10. G10 Pack packaging
  - [ ] 10.1 Migration 18 and ADR-0027
    - Add migration 18 `packs` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0027-pack-packaging-seeds-and-overlays.md` (new), resolving O-005 as Proposed. OPEN: licence.
    - _Requirements: 10.1, 10.10_
  - [ ] 10.2 Implement manifests and planning
    - Create `internal/packs/` (new): `pack.yaml` and Module manifest parse and print with `gopkg.in/yaml.v3` (existing dependency), hash verification, dependency resolution, plan with privilege diff and Overlay conflicts. Use `internal/files` (existing) for archive paths.
    - _Requirements: 10.1, 10.2, 10.3, 10.4_
  - [ ]* 10.3 Write property test for seed rerun and manifest round trip
    - **Property 20: Seed rerun and manifest round trip**
    - Create `internal/packs/packs_test.go` (new).
    - **Validates: Requirements 10.1, 10.5, 10.6**
  - [ ] 10.4 Implement install, seeds and Overlays
    - Add `internal/packs/{install,seeds,overlay}.go` (new files in the new package): Seed_Identity, three-way diff, checkpoints, Workspace lock via `internal/workspace/lock.go` (existing), removal guard, Role and Grant templates through `internal/auth` (existing).
    - _Requirements: 10.5, 10.6, 10.7, 10.8, 10.9_
  - [ ] 10.5 Add PackService, CLI and page
    - Create `proto/athena/v1/packs.proto` (new); `make generate`; create `internal/server/packs.go` (new); edit `gate.go`. Create `cmd/athena/pack.go` (new). Edit `web/src/pages/PacksPage.vue` (existing) with plan preview and installed list.
    - _Requirements: 10.3, 10.11, 10.12_
  - [ ] 10.6 Update documentation
    - Edit `docs/architecture/record-type-and-pack-model.md`, `docs/architecture/overview.md`, `README.md`, `AGENTS.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 10.1_
  - [ ] 10.7 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass: plan, install, interrupt and resume a throwaway Pack, in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 11. G11 Stock ledger, reservations, serial and batch, valuation
  - [ ] 11.1 Migration 19 and ADR-0028
    - Add migration 19 `stock_ledger` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0028-stock-ledger-and-valuation.md` (new). OPEN: valuation method and negative stock policy.
    - _Requirements: 11.1, 11.3_
  - [ ] 11.2 Implement the stock ledger, layers and applications
    - Create `internal/stock/` (new) with ledger rows, layers, Item application entries, bins and cost methods behind a method interface. Use `internal/posting` and `internal/types/quantity.go` (existing).
    - _Requirements: 11.1, 11.2, 11.3, 11.4_
  - [ ] 11.3 Implement cost deltas and late freight
    - Add `internal/stock/delta.go` (new): append-only cost deltas with economic and posting dates, freight split between on-hand and sold, rebuild watermark and close block.
    - _Requirements: 11.5, 11.6, 11.7, 11.13_
  - [ ]* 11.4 Write property test for append-only cost correction
    - **Property 21: Append-only cost correction**
    - Create `internal/stock/stock_test.go` (new).
    - **Validates: Requirements 11.5, 11.6, 11.7**
  - [ ] 11.5 Implement reservations, serial, batch, quarantine and counts
    - Add `internal/stock/{reserve,tracking,count}.go` (new files in the new package).
    - _Requirements: 11.9, 11.10, 11.11, 11.12, 11.14_
  - [ ]* 11.6 Write property test for availability
    - **Property 23: Availability**
    - Add to `internal/stock/stock_test.go`.
    - **Validates: Requirements 11.9, 11.10**
  - [ ] 11.7 Implement the stock-to-GL check
    - Add `internal/stock/check.go` (new): per control account comparison and Integrity Log entries.
    - _Requirements: 11.8, 11.16_
  - [ ]* 11.8 Write property test for stock value equals GL
    - **Property 22: Stock value equals GL**
    - Add to `internal/stock/stock_test.go`.
    - **Validates: Requirements 11.2, 11.8, 17.5**
  - [ ] 11.9 Add StockService, CLI and Desk view
    - Create `proto/athena/v1/stock.proto` (new); `make generate`; create `internal/server/stock.go` (new); edit `gate.go`. Create `cmd/athena/stock.go` (new). Edit `web/src/pages/RecordsPage.vue` (existing) and create `web/src/pages/StockCheckPage.vue` (new) for the check.
    - _Requirements: 11.15_
  - [ ] 11.10 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 11.3_
  - [ ] 11.11 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass of the stock check in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 12. G12 Remaining ledgers
  - [ ] 12.1 Migration 20 and ADR-0029
    - Add migration 20 `remaining_ledgers` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0029-remaining-ledgers.md` (new).
    - _Requirements: 12.1, 12.8_
  - [ ] 12.2 Implement tax, asset and budget ledgers
    - Create `internal/ledger/{tax,asset,budget}.go` (new files in the G5 package).
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.9_
  - [ ] 12.3 Implement project, leave, meter and intercompany ledgers
    - Create `internal/ledger/{project,leave,meter,intercompany}.go` (new files).
    - _Requirements: 12.5, 12.6, 12.8_
  - [ ]* 12.4 Write property test for ledger invariants
    - **Property 24: Ledger invariants**
    - Create `internal/ledger/ledgers_test.go` (new).
    - **Validates: Requirements 12.4, 12.6, 12.7**
  - [ ] 12.5 Extend read procedures and CLI
    - Edit `proto/athena/v1/ledger.proto` and `internal/server/ledger.go` (existing) so `ListLedgerRows` names each ledger; edit `cmd/athena/ledger.go` (existing) with `ledger show <ledger>`.
    - _Requirements: 12.10_
  - [ ] 12.6 Update documentation
    - Edit `docs/architecture/overview.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 12.9_
  - [ ] 12.7 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass of a ledger view in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 13. G13 Trees, effective dating, determination specificity and UoM
  - [ ] 13.1 Migration 21 and ADR-0030
    - Add migration 21 `trees_effective_uom` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0030-trees-effective-dating-and-uom.md` (new).
    - _Requirements: 13.1, 13.4_
  - [ ] 13.2 Implement tree and effective-date attributes
    - Edit `internal/server/attributes.go`, `internal/server/records.go`, `internal/server/record_types.go`, `internal/server/migrations.go` (existing) and `internal/store/store.go` (existing) for path, depth, cycle and range checks. Edit `proto/athena/v1/record_types.proto` (existing); `make generate`.
    - _Requirements: 13.1, 13.2_
  - [ ] 13.3 Implement UoM families and conversions
    - Edit `internal/types/quantity.go` (existing) and create `internal/ledger/uom.go` (new) for factors, overrides and cross-family rejection. Edit `internal/ledger/determination.go` (G5) to use effective ranges.
    - _Requirements: 13.3, 13.4, 13.5_
  - [ ]* 13.4 Write property test for effective ranges and UoM
    - **Property 25: Effective ranges and UoM round trip**
    - Add to `internal/ledger/ledgers_test.go` (new in 12.4) and `internal/types/types_test.go` (existing).
    - **Validates: Requirements 13.2, 13.4, 13.6**
  - [ ] 13.5 Add CLI and Desk support
    - Edit `cmd/athena/ledger.go` (existing) with `tree`, `uom` and `determination` subcommands. Edit `web/src/pages/RecordTypesPage.vue` and `web/src/components/FieldSettingsDialog.vue` (existing) for the new attributes, and `web/src/fieldRules.ts` with `internal/server/testdata/field_rules.json` (existing) in step.
    - _Requirements: 13.7_
  - [ ] 13.6 Update documentation
    - Edit `docs/architecture/record-type-and-pack-model.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 13.1_
  - [ ] 13.7 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 14. G14 Complete report engine
  - [ ] 14.1 Migration 22 and ADR-0031
    - Add migration 22 `report_engine` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0031-report-delivery-closing-pack-and-explain.md` (new).
    - _Requirements: 14.1, 14.4_
  - [ ] 14.2 Implement schedules and delivery
    - Create `internal/report/{schedule,delivery}.go` (new) on `internal/jobs` (G8) with Grant and Scope recheck at run and delivery, private links and logs.
    - _Requirements: 14.1, 14.2, 14.3_
  - [ ] 14.3 Implement the Closing Pack and control Reports
    - Create `internal/report/closingpack.go` (new): tie-outs, exceptions, sign-offs, lock and close block; add the two control Reports in `internal/report/report.go` (existing from G9).
    - _Requirements: 14.4, 14.5, 14.6_
  - [ ] 14.4 Implement Explain this movement
    - Create `internal/report/explain.go` (new) walking ledger row, voucher, version, Rule Run, request and actor, using `internal/audit` (existing).
    - _Requirements: 14.7_
  - [ ] 14.5 Add procedures and CLI
    - Edit `proto/athena/v1/reports.proto` (existing from G9): schedules, delivery, Closing Pack, `ExplainMovement`; `make generate`; edit `internal/server/reports.go`, `gate.go`. Edit `cmd/athena/report.go` (existing).
    - _Requirements: 14.10_
  - [ ] 14.6 Build work queues and the Activity timeline
    - Create `web/src/pages/WorkQueuePage.vue` (new). Edit `web/src/pages/RecordsPage.vue` and `web/src/pages/ReportsPage.vue` (existing) for the Activity timeline and Explain action; edit `web/src/App.vue`.
    - _Requirements: 14.7, 14.8, 14.9_
  - [ ]* 14.7 Write tests for delivery recheck and closing lock
    - Add example tests to `internal/report/run_test.go` (existing from 9.7): Scope removed while queued, closing pack not regenerated.
    - _Requirements: 14.2, 14.4_
  - [ ] 14.8 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 14.4_
  - [ ] 14.9 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass of schedules, Closing Pack and Explain in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 15. G15 Integration surface
  - [ ] 15.1 Migration 23 and ADR-0032
    - Add migration 23 `integrations` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0032-integration-surface.md` (new).
    - _Requirements: 15.1, 15.4_
  - [ ] 15.2 Implement the verified provider inbox
    - Create `internal/integrations/` (new): HMAC verification with the key window, event dedupe, evidence records, mapping to Operation_ID. Use `internal/secrets` (G8, existing by now) for keys.
    - _Requirements: 15.1, 15.2, 15.3, 15.9_
  - [ ]* 15.3 Write property test for provider event verification
    - **Property 26: Provider event verification**
    - Create `internal/integrations/inbox_test.go` (new).
    - **Validates: Requirements 15.1, 15.2, 15.3**
  - [ ] 15.4 Implement attachments, search, comments, import and export
    - Add `internal/integrations/{attachments,search,comments,import}.go` (new files); edit `internal/store/store.go` and `internal/store/mariadb.go` (existing) with a dialect `Search` method; reuse blob rules from `internal/files/path.go` (existing); add an abandoned-upload job in `internal/jobs` (existing).
    - _Requirements: 15.4, 15.5, 15.6, 15.7, 15.8_
  - [ ] 15.5 Add procedures, CLI and Desk support
    - Create `proto/athena/v1/integrations.proto` (new); `make generate`; create `internal/server/integrations.go` (new); edit `gate.go`. Create `cmd/athena/{inbox,attachments}.go` (new). Edit `web/src/pages/RecordsPage.vue` (existing) for attachments and comments.
    - _Requirements: 15.10_
  - [ ] 15.6 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 15.9_
  - [ ] 15.7 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 16. G16 PWA and offline stock count
  - [ ] 16.1 Migration 24 and ADR-0033
    - Add migration 24 `offline_sync` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0033-offline-drafts-and-pwa.md` (new). OPEN: `vite-plugin-pwa` or a hand-written worker.
    - _Requirements: 16.1, 16.4_
  - [ ] 16.2 Implement SyncService
    - Create `proto/athena/v1/sync.proto` and `internal/server/sync.go` (new); `make generate`; edit `gate.go`. Reuse the Operation_ID contract from `internal/posting/operations.go` (existing). Check session, Grants, revision and count baseline; return the original draft on replay; never post before review.
    - _Requirements: 16.4, 16.5, 16.6, 16.7, 16.8_
  - [ ]* 16.3 Write property test for offline draft replay
    - **Property 27: Offline draft replay**
    - Create `internal/server/sync_test.go` (new).
    - **Validates: Requirements 16.4, 16.5, 16.7**
  - [ ] 16.4 Build the offline shell and queue
    - Create `web/src/offline/{queue,store,status}.ts` and tests (new directory; no existing code covers offline). Create the manifest, service worker and edit `web/index.html`, `web/src/main.ts`, `web/vite.config.ts` (existing). Edit `internal/webui` (existing) to serve the worker with correct headers and no private caching.
    - _Requirements: 16.1, 16.2, 16.3, 16.9_
  - [ ] 16.5 Build the count and review screens
    - Create `web/src/pages/CountPage.vue` (new); edit `web/src/App.vue`, `web/src/api.ts`. Show the four states, variance diff and supervisor approval.
    - _Requirements: 16.3, 16.6_
  - [ ] 16.6 Add CLI
    - Create `cmd/athena/sync.go` (new); edit `cmd/athena/main.go` (existing).
    - _Requirements: 16.11_
  - [ ] 16.7 Run the acceptance script
    - Add an acceptance test and a written device run sheet in `internal/server/sync_test.go` and `README.md` (existing): 12 lines, three serials, restart, edit, interrupted acknowledgement, foreground retry; real-device run remains manual.
    - _Requirements: 16.10_
  - [ ] 16.8 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `docs/decisions/decision-log.md` (existing).
    - _Requirements: 16.1_
  - [ ] 16.9 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass offline and online in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 17. G17 Business starter Packs
  - [ ] 17.1 Migration 25 and ADR-0034
    - Add migration 25 `setup_readiness` to `internal/server/sysmigrations.go` (existing). Create `docs/decisions/adr-0034-business-starter-and-setup.md` (new). OPEN: pilot company, jurisdiction, chart, valuation method.
    - _Requirements: 17.1, 17.11_
  - [ ] 17.2 Author the starter Packs
    - Create `packs/{party,items,buying,selling,stock,expenses,finance,reports}/` (new; Pack sources have no existing home) each with `pack.yaml`, Record Types, Rules, Queries, Reports, Roles, seeds and fixtures. Nothing is added to `internal/` for domain Record Types.
    - _Requirements: 17.2, 17.9_
  - [ ] 17.3 Implement the Setup Wizard and readiness
    - Edit `internal/workspace/{create,templates}.go` and `cmd/athena/main.go` (existing) so `athena new` offers Business starter or Empty Workspace. Create `cmd/athena/setup.go` (new). Create `web/src/pages/SetupPage.vue` (new); edit `web/src/App.vue`, `web/src/pages/OverviewPage.vue` (existing). Extend `PackService` readiness in `internal/packs` and `internal/server/packs.go` (existing).
    - _Requirements: 17.1, 17.3, 17.4, 17.12_
  - [ ] 17.4 Write the numerical fixture tests
    - Create `internal/packs/fixtures_test.go` (new) running through the real services: purchase, sell, return and close; late freight; employee advance; asset; FX; production split; 10.00 three-way split (3.34, 3.33, 3.33).
    - _Requirements: 17.5, 17.6, 17.7_
  - [ ] 17.5 Write the Research audit cases
    - Add A1 (line match with equal headers), A3 (concurrent invoices in one series with replay) and A4 (scoped totals, list, export and drill agree) to `internal/packs/fixtures_test.go` (new in 17.4).
    - _Requirements: 17.8_
  - [ ]* 17.6 Write property test for split allocation
    - **Property 11: Allocation sums**
    - Add to `internal/packs/fixtures_test.go`.
    - **Validates: Requirements 17.7, 17.10**
  - [ ] 17.7 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `AGENTS.md`, `docs/decisions/decision-log.md` (existing); keep D-007.
    - _Requirements: 17.2_
  - [ ] 17.8 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, first-day trial in the browser in both themes. Ensure all tests pass, ask the user if questions arise.

- [ ] 18. G18 Industry and channel Packs
  - [ ] 18.1 ADR-0035 and shared contracts
    - Create `docs/decisions/adr-0035-industry-and-channel-packs.md` (new). Define shared allocation, transformation and observation structures in `internal/packs` (existing) only where two Packs need them.
    - _Requirements: 18.1, 18.16_
  - [ ] 18.2 POS, supermarket and payments Packs
    - Create `packs/{pos,supermarket,payments}/` (new) with tender attempts, Check payment, Park sale, drawer close and the adapter contract over `internal/integrations` (G15). Fixtures: KES 1,350 incident, drawer 148.
    - _Requirements: 18.2, 18.3, 18.4_
  - [ ] 18.3 Catalogue, channels and WhatsApp Packs
    - Create `packs/{catalogue,channels,whatsapp}/` (new) with listings, external order dedupe and Flow bindings that change one Record.
    - _Requirements: 18.5, 18.6_
  - [ ] 18.4 Learning Pack
    - Create `packs/learning/` (new) with the 74 to 77 grade correction and transcript supersession fixture.
    - _Requirements: 18.7_
  - [ ] 18.5 Construction and Restaurant Packs
    - Create `packs/{construction,restaurant}/` (new) with the Friday, capital and recipe fixtures.
    - _Requirements: 18.8, 18.9_
  - [ ] 18.6 Dairy, Poultry and Hatchery Packs
    - Create `packs/{dairy,poultry,hatchery}/` (new) with collection, flock-day and hatch fixtures. Hatchery is optional.
    - _Requirements: 18.10, 18.11_
  - [ ] 18.7 Butchery and Manufacturing Packs
    - Create `packs/{butchery,manufacturing}/` (new) with cutting, mixed-batch recall and partial-completion fixtures.
    - _Requirements: 18.12, 18.13_
  - [ ] 18.8 Combined and upgrade fixtures
    - Create `internal/packs/combined_test.go` (new): butchery plus restaurant (stock 65, COGS 35, revenue 112, gross profit 77) and a two-Pack upgrade replay before activation.
    - _Requirements: 18.14, 18.15_
  - [ ] 18.9 Readiness states and integration page
    - Edit `internal/packs/` and `web/src/pages/SettingsPage.vue` (existing from G8) for readiness states and secret-reference bindings.
    - _Requirements: 18.16, 18.17_
  - [ ] 18.10 Update documentation
    - Edit `docs/architecture/overview.md`, `README.md`, `AGENTS.md`, `docs/decisions/decision-log.md` (existing). Mark each Pack designed, implemented, field-tested or qualified; mark build order, country and provider qualification OPEN.
    - _Requirements: 18.1, 18.18_
  - [ ] 18.11 Checkpoint - Ensure all tests pass
    - `make generate` no diff, `make test`, web typecheck and build, browser pass of two contrasting installations in both themes. Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP.
- Each task references specific requirements for traceability.
- Property tests validate the 27 correctness properties; the seven the user named are Properties 8 (balanced postings), 10 (reversal mirrors), 9 (idempotent replay), 11 (allocation sums), 3 (scope union), 2 (last owner) and 22 (stock value equals GL).
- Open sponsor questions are listed in the design and stay OPEN until answered.
- The Starlark Rule host stays "not production-ready" in docs until ADR-0016 and ADR-0019 are accepted.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["1.3"] },
    { "id": 3, "tasks": ["1.4"] },
    { "id": 4, "tasks": ["1.5"] },
    { "id": 5, "tasks": ["1.6"] },
    { "id": 6, "tasks": ["1.7"] },
    { "id": 7, "tasks": ["1.8"] },
    { "id": 8, "tasks": ["1.9"] },
    { "id": 9, "tasks": ["1.10"] },
    { "id": 10, "tasks": ["1.11"] },
    { "id": 11, "tasks": ["1.12"] },
    { "id": 12, "tasks": ["1.13"] },
    { "id": 13, "tasks": ["1.14"] },
    { "id": 14, "tasks": ["1.15"] },
    { "id": 15, "tasks": ["2.1"] },
    { "id": 16, "tasks": ["2.2"] },
    { "id": 17, "tasks": ["2.3"] },
    { "id": 18, "tasks": ["2.4"] },
    { "id": 19, "tasks": ["2.5"] },
    { "id": 20, "tasks": ["2.6"] },
    { "id": 21, "tasks": ["3.1"] },
    { "id": 22, "tasks": ["3.2"] },
    { "id": 23, "tasks": ["3.3"] },
    { "id": 24, "tasks": ["3.4"] },
    { "id": 25, "tasks": ["3.5"] },
    { "id": 26, "tasks": ["3.6"] },
    { "id": 27, "tasks": ["4.1"] },
    { "id": 28, "tasks": ["4.2"] },
    { "id": 29, "tasks": ["4.3"] },
    { "id": 30, "tasks": ["4.4"] },
    { "id": 31, "tasks": ["4.5"] },
    { "id": 32, "tasks": ["5.1"] },
    { "id": 33, "tasks": ["5.2"] },
    { "id": 34, "tasks": ["5.3"] },
    { "id": 35, "tasks": ["5.4"] },
    { "id": 36, "tasks": ["5.5"] },
    { "id": 37, "tasks": ["5.6"] },
    { "id": 38, "tasks": ["5.7"] },
    { "id": 39, "tasks": ["5.8"] },
    { "id": 40, "tasks": ["5.9"] },
    { "id": 41, "tasks": ["5.10"] },
    { "id": 42, "tasks": ["6.1"] },
    { "id": 43, "tasks": ["6.2"] },
    { "id": 44, "tasks": ["6.3"] },
    { "id": 45, "tasks": ["6.4"] },
    { "id": 46, "tasks": ["6.5"] },
    { "id": 47, "tasks": ["6.6"] },
    { "id": 48, "tasks": ["6.7"] },
    { "id": 49, "tasks": ["6.8"] },
    { "id": 50, "tasks": ["6.9"] },
    { "id": 51, "tasks": ["6.10"] },
    { "id": 52, "tasks": ["6.11"] },
    { "id": 53, "tasks": ["6.12"] },
    { "id": 54, "tasks": ["7.1"] },
    { "id": 55, "tasks": ["7.2"] },
    { "id": 56, "tasks": ["7.3"] },
    { "id": 57, "tasks": ["7.4"] },
    { "id": 58, "tasks": ["7.5"] },
    { "id": 59, "tasks": ["7.6"] },
    { "id": 60, "tasks": ["7.7"] },
    { "id": 61, "tasks": ["7.8"] },
    { "id": 62, "tasks": ["7.9"] },
    { "id": 63, "tasks": ["8.1"] },
    { "id": 64, "tasks": ["8.2"] },
    { "id": 65, "tasks": ["8.3"] },
    { "id": 66, "tasks": ["8.4"] },
    { "id": 67, "tasks": ["8.5"] },
    { "id": 68, "tasks": ["8.6"] },
    { "id": 69, "tasks": ["8.7"] },
    { "id": 70, "tasks": ["8.8"] },
    { "id": 71, "tasks": ["8.9"] },
    { "id": 72, "tasks": ["8.10"] },
    { "id": 73, "tasks": ["8.11"] },
    { "id": 74, "tasks": ["8.12"] },
    { "id": 75, "tasks": ["8.13"] },
    { "id": 76, "tasks": ["8.14"] },
    { "id": 77, "tasks": ["9.1"] },
    { "id": 78, "tasks": ["9.2"] },
    { "id": 79, "tasks": ["9.3"] },
    { "id": 80, "tasks": ["9.4"] },
    { "id": 81, "tasks": ["9.5"] },
    { "id": 82, "tasks": ["9.6"] },
    { "id": 83, "tasks": ["9.7"] },
    { "id": 84, "tasks": ["9.8"] },
    { "id": 85, "tasks": ["9.9"] },
    { "id": 86, "tasks": ["9.10"] },
    { "id": 87, "tasks": ["10.1"] },
    { "id": 88, "tasks": ["10.2"] },
    { "id": 89, "tasks": ["10.3"] },
    { "id": 90, "tasks": ["10.4"] },
    { "id": 91, "tasks": ["10.5"] },
    { "id": 92, "tasks": ["10.6"] },
    { "id": 93, "tasks": ["10.7"] },
    { "id": 94, "tasks": ["11.1"] },
    { "id": 95, "tasks": ["11.2"] },
    { "id": 96, "tasks": ["11.3"] },
    { "id": 97, "tasks": ["11.4"] },
    { "id": 98, "tasks": ["11.5"] },
    { "id": 99, "tasks": ["11.6"] },
    { "id": 100, "tasks": ["11.7"] },
    { "id": 101, "tasks": ["11.8"] },
    { "id": 102, "tasks": ["11.9"] },
    { "id": 103, "tasks": ["11.10"] },
    { "id": 104, "tasks": ["11.11"] },
    { "id": 105, "tasks": ["12.1"] },
    { "id": 106, "tasks": ["12.2"] },
    { "id": 107, "tasks": ["12.3"] },
    { "id": 108, "tasks": ["12.4"] },
    { "id": 109, "tasks": ["12.5"] },
    { "id": 110, "tasks": ["12.6"] },
    { "id": 111, "tasks": ["12.7"] },
    { "id": 112, "tasks": ["13.1"] },
    { "id": 113, "tasks": ["13.2"] },
    { "id": 114, "tasks": ["13.3"] },
    { "id": 115, "tasks": ["13.4"] },
    { "id": 116, "tasks": ["13.5"] },
    { "id": 117, "tasks": ["13.6"] },
    { "id": 118, "tasks": ["13.7"] },
    { "id": 119, "tasks": ["14.1"] },
    { "id": 120, "tasks": ["14.2"] },
    { "id": 121, "tasks": ["14.3"] },
    { "id": 122, "tasks": ["14.4"] },
    { "id": 123, "tasks": ["14.5"] },
    { "id": 124, "tasks": ["14.6"] },
    { "id": 125, "tasks": ["14.7"] },
    { "id": 126, "tasks": ["14.8"] },
    { "id": 127, "tasks": ["14.9"] },
    { "id": 128, "tasks": ["15.1"] },
    { "id": 129, "tasks": ["15.2"] },
    { "id": 130, "tasks": ["15.3"] },
    { "id": 131, "tasks": ["15.4"] },
    { "id": 132, "tasks": ["15.5"] },
    { "id": 133, "tasks": ["15.6"] },
    { "id": 134, "tasks": ["15.7"] },
    { "id": 135, "tasks": ["16.1"] },
    { "id": 136, "tasks": ["16.2"] },
    { "id": 137, "tasks": ["16.3"] },
    { "id": 138, "tasks": ["16.4"] },
    { "id": 139, "tasks": ["16.5"] },
    { "id": 140, "tasks": ["16.6"] },
    { "id": 141, "tasks": ["16.7"] },
    { "id": 142, "tasks": ["16.8"] },
    { "id": 143, "tasks": ["16.9"] },
    { "id": 144, "tasks": ["17.1"] },
    { "id": 145, "tasks": ["17.2"] },
    { "id": 146, "tasks": ["17.3"] },
    { "id": 147, "tasks": ["17.4"] },
    { "id": 148, "tasks": ["17.5"] },
    { "id": 149, "tasks": ["17.6"] },
    { "id": 150, "tasks": ["17.7"] },
    { "id": 151, "tasks": ["17.8"] },
    { "id": 152, "tasks": ["18.1"] },
    { "id": 153, "tasks": ["18.2"] },
    { "id": 154, "tasks": ["18.3"] },
    { "id": 155, "tasks": ["18.4"] },
    { "id": 156, "tasks": ["18.5"] },
    { "id": 157, "tasks": ["18.6"] },
    { "id": 158, "tasks": ["18.7"] },
    { "id": 159, "tasks": ["18.8"] },
    { "id": 160, "tasks": ["18.9"] },
    { "id": 161, "tasks": ["18.10"] },
    { "id": 162, "tasks": ["18.11"] }
  ]
}
```
