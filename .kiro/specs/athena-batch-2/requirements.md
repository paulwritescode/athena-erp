# Requirements Document

## Introduction

This spec covers Athena batch 2: everything after the committed baseline P0 to P9 and P8a (`.agents/tasks/erp-kernel-roadmap.md`). It follows the revamp docs in `docs/revamp/` and the original kernel specification. It is organised in eighteen groups, G1 to G18, in build order. Requirement N belongs to group GN. Each group builds on the one before.

Already finished and not re-specified here: Record Types and kinds (`internal/server/kinds.go`, `records.go`, `record_types.go`), document lifecycle (`internal/lifecycle`), numbering (`internal/numbering`), the hash-chained Audit Log (`internal/audit`), expressions (`internal/expr`), authentication with Argon2id, sessions, Grants and CSRF (`internal/auth`, `internal/server/gate.go`, `access.go`), Workspace files (`internal/files`), the Starlark Rules host (`internal/rules`), versioned system migrations (`internal/store/migrate.go`, `server.SystemMigrations`, last applied number 11), the dialect layer (`internal/store`), exact types (`internal/types`), the RecordPicker and the sign-in error states. Requirements below extend those parts and name the files they extend.

Source labels: PB = Playbook, RM = Records-and-movements, PE = Process-and-Event-Catalogue, RA = Research-and-audit, IO = Industry-and-Operations-Design, CC = Checkout-Commerce-and-Learning, KS = kernel specification, RD = `.agents/tasks/erp-kernel-roadmap.md`.

Ordering note: G8 (jobs, outbox, events, workflows, schedules; roadmap P16) moves before G9 (balances and reports; roadmap P14) because Report schedules and delivery need durable jobs. The roadmap order P14 then P16 is superseded for this spec only.

Placement note: Record lifetimes, key custody and secret references (IO sections 2 and 3) are Core concerns and sit in G8, where jobs and the outbox they depend on exist. Safe Settings with scope chains (IO section 4) sit in G5, where Company and Branch exist.

Open sponsor questions are marked OPEN. They are not answered here: pilot company, jurisdiction and chart, stock valuation method, backdating window, multi-factor sign-in, licence, retention periods and deployment profile.

## Glossary

- **Core**: The Go runtime of Athena that owns transactions, authorization, lifecycle and posting invariants.
- **Workspace**: One database plus installed Packs, data, users and Settings.
- **Access_Service**: The `AuthService` ConnectRPC service and the `internal/auth` package.
- **Principal**: The signed-in user as the Core sees them for one request (`internal/auth/principal.go`).
- **Assignment**: One Role given to one user with its own Scope, active-from date and expiry date.
- **Scope**: A set of dimension values (Company, Branch, Warehouse) that limits what one Assignment reaches.
- **Challenge**: A single-use, expiring credential for first login, password reset or activation, stored only as a digest.
- **Last_Owner_Guard**: The transactional check that leaves at least one active verified Instance Owner.
- **Permission_Explorer**: The Access page view that shows which Assignment matched each action and why a denied action failed.
- **Rule_Host**: The Starlark host in `internal/rules`.
- **Resource_Guard**: Host-side caps on Starlark string, list and dict sizes, values built, rows read and cascade depth.
- **Publisher**: The journaled Record Type save path in `internal/server/record_types.go`.
- **Ledger_Service**: The Core component that owns Company, Branch, dimensions, periods, currencies, rates, Accounts and Account Determination.
- **Posting_Engine**: The Core component in `internal/posting` that validates and writes a posting plan.
- **Posting_Plan**: The list of ledger lines a posting Rule proposes for one voucher revision.
- **Posting_Group**: The set of ledger rows written for one voucher revision, event and book.
- **Operation_ID**: A client-supplied durable key that identifies one command across retries.
- **Posting_Log**: The Log Record Type that records every posting attempt and its outcome.
- **Settlement_Service**: The Core component that applies receipts and payments to open items by line.
- **Event_Bus**: The durable business event, outbox, inbox and subscription layer in `internal/events`.
- **Job_Runner**: The database-backed worker in `internal/jobs`.
- **Workflow_Engine**: The component that stores workflow instances, waits and tasks.
- **Balance_Service**: The component that maintains and rebuilds Balance rows with watermarks.
- **Query_Compiler**: The component in `internal/query` that compiles typed Queries with Scope into SQL.
- **Report_Service**: The `ReportService` ConnectRPC service and `internal/report`.
- **Report_Run**: An immutable Snapshot of one Report execution with cutoff and provenance.
- **Pack_Manager**: The component in `internal/packs` that plans and installs Packs.
- **Seed_Identity**: The tuple (pack, seed key, scope) that identifies one seeded row.
- **Stock_Service**: The component in `internal/stock` that owns the stock ledger, layers, bins and reservations.
- **Closing_Pack**: A locked Snapshot of tie-outs, exceptions and sign-offs for a closed period.
- **Provider_Inbox**: The verified inbox in `internal/integrations` for provider webhooks.
- **Sync_Service**: The `SyncService` that accepts offline drafts.
- **Setup_Wizard**: The first-run flow that offers Business starter or Empty Workspace.
- **Desk**: The Vue 3 admin interface in `web/`.
- **CLI**: The `athena` command in `cmd/athena`.
- **Gate**: The permission gate in `internal/server/gate.go` with its `procedurePolicies` table.

## Requirements

### Requirement 1: Users and Roles page and the revamp access model (G1)

**User Story:** As an Instance Owner, I want to invite users, give them scoped Roles and see what they can do, so that access is safe without writing code.

#### Acceptance Criteria

1. WHEN an administrator creates a user, THE Access_Service SHALL create an invited user with no password and return a single-use Challenge shown once (PB §3 Create a user).
2. THE Access_Service SHALL store each Challenge only as a digest with purpose, user, generation, expiry and consumed time (PB §3 No mail relay).
3. WHEN a user presents a valid unconsumed Challenge and a chosen password, THE Access_Service SHALL consume the Challenge, store the Argon2id hash and start a session (PB §3 steps 4 and 5).
4. WHEN two completions of one Challenge arrive together, THE Access_Service SHALL accept exactly one (PB §3 table).
5. IF a Challenge is expired, consumed, superseded by a newer generation or presented five times wrongly within its life, THEN THE Access_Service SHALL refuse it and write an Access Log event without the Challenge value (PB §3 recovery).
6. WHEN an administrator resends an invite or starts a reset, THE Access_Service SHALL invalidate earlier Challenges and, for a reset, revoke the target's sessions (PB §3 step 5).
7. THE Access_Service SHALL remove the administrator-set permanent password by marking `CreateUserRequest.password` (field 3) and `ResetPasswordRequest.new_password` (field 2) reserved in `proto/athena/v1/auth.proto` (PB §3 RPC evolution, RA first login).
8. WHEN migration 12 runs on a Workspace with active users, THE Access_Service SHALL leave those users active without a forced first-use flow (PB §3 RPC evolution).
9. WHEN an owner is created through the CLI, THE Access_Service SHALL create the user through the same bootstrap path guarded against two competing initialisations (PB §3 No mail relay).
10. THE Principal SHALL gain no business, Build, Files or Rules permission from the owner flag, and THE Access_Service SHALL replace the `Owner` shortcut in `internal/auth/principal.go` with explicit protected capabilities `access:ownership`, `access:users`, `access:roles`, `access:assign`, `access:recover` and `access:sessions` (RA ownership enforcement, PB §3 Instance Owner).
11. WHEN a request would remove, suspend or demote the last active verified Instance Owner, THE Access_Service SHALL refuse it inside the same transaction as the change (PB §3 counterexample, RA P0).
12. WHEN two requests each remove a different owner at the same time, THE Access_Service SHALL leave at least one active owner (PB §3 counterexample).
13. THE Access_Service SHALL store each Role Assignment with a Scope, an active-from date and an expiry date, and SHALL treat an Assignment outside its dates as absent (PB §3 Create a user).
14. IF a Role requires a Scope dimension and the Assignment gives none, THEN THE Gate SHALL deny the Assignment's actions (PB §3 Evaluation).
15. WHEN the Gate evaluates an action, THE Principal SHALL test each whole Assignment for action, Record Type, row condition, Scope and field level together, and SHALL union only the results of whole matching Assignments (PB §3 Evaluation and counterexample).
16. FOR ALL users with two Assignments, THE Principal SHALL allow an action on a row only when one single Assignment allows it (scope-union property, PB §3 counterexample).
17. WHEN a Record is updated, THE Gate SHALL recheck the Assignments against both the old and the proposed row (PB §3 Evaluation).
18. THE Permission_Explorer SHALL show, for a chosen user, action and Record Type, which Assignment matched or why each Assignment failed (PB §3 counterexample).
19. WHEN an administrator edits a Role or Assignment, THE Access_Service SHALL show a preview of affected users and a diff, and SHALL apply it with an expected revision check (PB §3 Select permissions, table).
20. THE Access_Service SHALL ship the system Roles Platform Administrator, Instance Owner and System Manager and the business Role templates Receiving Clerk, Purchase User, Accounts Payable Clerk, Treasury Officer, Accountant, Auditor and Report Viewer, with no Grants on Record Types that do not yet exist (PB §3 table).
21. IF a System Manager tries to assign ownership, platform status or a Grant beyond their own delegation, THEN THE Access_Service SHALL refuse the whole change (PB §3 table).
22. WHEN an administrator suspends a user, THE Access_Service SHALL revoke the user's sessions, and reactivation SHALL require review of privileged Assignments (PB §3 table).
23. THE Access_Service SHALL let a user list and revoke their own sessions, and let an authorised administrator list and revoke another user's sessions (PB §3 Local identity baseline).
24. THE Desk SHALL provide a Users & Roles page in Build, shown with `access:users` or `access:roles`, using shadcn-vue components and Sonner toasts (AGENTS.md, RD §6 carry-over).
25. WHEN a Role Assignment would grant the same user a creator and approver duty that a Grant marks as separate, THE Permission_Explorer SHALL show the conflict (PB §3 Role chips).
26. THE CLI SHALL provide `athena user invite|resend|reset|suspend|reactivate|sessions` and `athena role assign|revoke|explain` commands that call `internal/auth` directly (RD §8 CLI parity).
27. IF an administrator performs a privileged recovery, THEN THE Access_Service SHALL require the administrator's password again within the last ten minutes (PB §3 Reset, MFA OPEN).
28. OPEN: THE Access_Service SHALL leave multi-factor sign-in out of this spec until the sponsor decides it (PB §3, ADR-0014).

### Requirement 2: PostgreSQL adapter, implemented but untested (G2)

**User Story:** As a maintainer, I want a PostgreSQL dialect behind the existing seam, so that a later parity run has code to test.

#### Acceptance Criteria

1. THE Core SHALL provide a PostgreSQL implementation of `store.Dialect` in `internal/store/postgres.go` covering every method of the interface (RD P10).
2. WHEN the Workspace driver is `postgres`, THE Core SHALL open it through the pgx stdlib driver wrapped by `otelsql` (RD P10).
3. THE Core SHALL build, vet and unit-test the generated SQL strings without a running PostgreSQL server (RD §8 answer 3).
4. THE documentation SHALL label PostgreSQL "implemented but untested" in `docs/architecture/overview.md`, `README.md`, ADR-0005 and the decision log (RD §8 answer 3).
5. THE Core SHALL make no PostgreSQL parity claim and require no Docker (user constraint).
6. WHEN an identifier exceeds 63 bytes, THE Dialect SHALL shorten it deterministically and keep distinct inputs distinct (RD P1).
7. THE Core SHALL keep MariaDB as the only tested database and keep `make test` green against MariaDB (AGENTS.md).
8. WHERE the driver is `postgres`, THE Core SHALL use explicit row locks and UTC times (KS §20.3).
9. THE CLI SHALL accept `postgres` as a driver value in `athena new` and print an untested warning (RD P10).

### Requirement 3: Starlark memory guard (G3)

**User Story:** As an operator, I want a Rule that allocates too much to stop safely, so that Rules can be accepted for production use.

#### Acceptance Criteria

1. THE Rule_Host SHALL cap the length of any string, list, dict or tuple a Rule builds at limits held in `rules.Limits` (PB §2 Rule host, ADR-0016).
2. THE Rule_Host SHALL cap the total number of values a Rule builds in one run (PB §2 Rule host).
3. THE Rule_Host SHALL cap the rows a host function returns and the cascade depth of Rule-triggered writes (PB §2 Rule host, KS §13).
4. IF a Rule reaches a cap, THEN THE Rule_Host SHALL stop it with error code `memory_limit`, `row_limit` or `cascade_limit` and record the outcome in the Rule Run log (ADR-0016).
5. WHEN a Rule runs `[0] * 10**9`, THE Rule_Host SHALL stop it before allocation (ADR-0016 memory gap).
6. THE Rule_Host SHALL apply the same caps to Simulate (`RuleService.SimulateRule`) and to saved Rules (`internal/server/rules_engine.go`).
7. WHEN the caps close the gap, THE documentation SHALL amend ADR-0016 and AGENTS.md and keep the status Proposed until the sponsor accepts it (ADR-0016 trigger).

### Requirement 4: Atomic child and parent Record Type save (G4)

**User Story:** As a Record Type author, I want a child and its parent saved together or not at all, so that a failed parent never leaves a saved child.

#### Acceptance Criteria

1. WHEN a save names a child Record Type and its parent, THE Publisher SHALL apply both or neither (RD §6 tracked item).
2. IF the parent save fails, THEN THE Publisher SHALL roll back the child's metadata, journal entry and table change, or run a recorded compensating step that restores the prior state (RD §6).
3. WHEN the process stops between the two saves, THE Publisher SHALL resume or roll back the pair at startup through `resumeOperations` (`record_types.go`).
4. THE Desk SHALL send one request for the pair and show one Sonner toast (`web/src/App.vue`).
5. THE parent's rollup formula over the child SHALL save in the same request without a second save (RD §3 P8 carry-over).

### Requirement 5: Ledger foundation (G5)

**User Story:** As a controller, I want companies, periods, currencies and accounts set up under the kernel's Ledger and Balance kinds, so that postings have valid targets.

#### Acceptance Criteria

1. THE Ledger_Service SHALL provide Company, Branch, Department and Cost Centre as kernel-provisioned definitions, with hierarchy cycle checks and no posting to group nodes (RM dictionary).
2. THE Ledger_Service SHALL provide Dimension Definition with permitted members, mandatory accounts and validity (RM dictionary, KS §10.1).
3. THE Ledger_Service SHALL provide Fiscal Year and Accounting Period per company and book, with no overlapping periods in one calendar (RM dictionary).
4. WHEN an administrator soft-closes a period, THE Ledger_Service SHALL restrict normal posting, and WHEN a hard close is set, SHALL require a separately approved reopen or a correction in an open period (RM dictionary, PB §2 Corrections).
5. THE Ledger_Service SHALL provide Currency with minor-unit exponent and Exchange Rate keyed by pair, rate type, source and effective instant, and SHALL reject ambiguous matches (RM dictionary).
6. THE Ledger_Service SHALL provide Account, Account Type and Account Determination with tree parent, group or leaf, class and control-account role (RM dictionary).
7. IF two Account Determination mappings match with equal specificity, THEN THE Ledger_Service SHALL name both and block preview and submit (RM dictionary, RA).
8. THE Core SHALL allow the Ledger and Balance kinds in `internal/server/kinds.go` only for kernel-owned definitions, and SHALL reject direct writes to Ledger rows through `RecordService` (RD P11).
9. THE Ledger_Service SHALL resolve one Setting generation per posting, with Settings keyed by owner Pack, type, scope chain, effective time and revision (IO §4).
10. WHERE a Setting is a list, THE Ledger_Service SHALL apply the declared replace, append or intersect rule, and IF two equal-priority candidates conflict, THEN SHALL report an error (IO §4).
11. WHEN a Setting changes, THE Ledger_Service SHALL show the effective diff for future dates before publishing, and SHALL apply the change prospectively (IO §4).
12. THE Desk SHALL add Build pages for Company setup, Periods, Currencies and Accounts, and a readiness view that names each unresolved mapping by action (RM provisioning stage).
13. THE CLI SHALL provide `athena ledger company|period|currency|account|determination|setting` commands (RD §8).
14. THE documentation SHALL record in ADR-0021 which masters the kernel owns, and mark the relation to D-007 OPEN until the sponsor confirms it (RD P11).
15. OPEN: THE Ledger_Service SHALL not choose a jurisdiction, chart or tax rate; the sponsor supplies them (PB §7 decisions before the pilot).

### Requirement 6: Posting engine (G6)

**User Story:** As a controller, I want every voucher to post balanced, once, and to reverse exactly, so that books can be trusted.

#### Acceptance Criteria

1. WHEN a posting Rule returns a Posting_Plan, THE Posting_Engine SHALL validate it balanced per company and book in base currency and consistent per transaction currency (PB §2 step 5).
2. FOR ALL valid Posting_Plans, THE Posting_Engine SHALL write ledger rows whose debits equal credits (balanced-postings property, KS invariant 1).
3. THE Posting_Engine SHALL reject a plan with an inactive or group Account, a closed period, a missing mandatory dimension or a protected control account used without a source (PB §2 step 5, RM).
4. WHEN a voucher revision is submitted, THE Posting_Engine SHALL write one Posting_Group per declared event and book and SHALL enforce its uniqueness in the database (RM three relational contracts).
5. THE Posting_Engine SHALL write the voucher revision, ledger rows, balance deltas, audit event, operation result and outbox row in one transaction (PB §2 step 5).
6. IF any part of a posting fails, THEN THE Posting_Engine SHALL leave no ledger row, balance change or event, and SHALL record the attempt separately in the Posting_Log (PE §6, PB §4).
7. WHEN a command carries an Operation_ID, THE Posting_Engine SHALL return the prior result for the same ID and canonical payload hash (PB §2 Retry contract).
8. IF the same Operation_ID arrives with a different payload, THEN THE Posting_Engine SHALL return a conflict error (PB §2 Retry contract).
9. FOR ALL commands replayed any number of times with one Operation_ID, THE Posting_Engine SHALL leave the ledgers as after the first replay (idempotent-replay property, KS invariant 7).
10. WHEN two submits with different Operation_IDs target one voucher revision, event and book, THE Posting_Engine SHALL post once (PB §2 Retry contract).
11. WHEN a voucher is cancelled, THE Posting_Engine SHALL mirror the stored amounts, rates and quantities as rows flagged as reversals with the reversed row identity (PB §2 Corrections, RA P0).
12. FOR ALL submitted vouchers, submit followed by cancel SHALL net every ledger to zero apart from the flagged mirror rows (reversal-mirror property, KS invariant 2).
13. WHEN a voucher's period is hard-closed, THE Posting_Engine SHALL book a linked correction in an allowed period with the original date preserved (PB §2 Corrections, RM closed-period adjustments).
14. THE Posting_Engine SHALL provide a preview that returns the plan, resolved accounts, source lines, applied policy versions and rounding without writing (PB §2 step 4).
15. IF a changed plan at submit differs from the previewed plan where approval depended on it, THEN THE Posting_Engine SHALL require review (PB §2 step 4).
16. THE Posting_Engine SHALL require fixture replay, a plan diff and a named approval to activate a changed posting Rule (PB §2 Rule host).
17. THE Posting_Engine SHALL reject a manual journal to a control account that lacks the matching party item (RM contracts).
18. THE CLI SHALL provide `athena post preview|submit|cancel|status|log` (RD §8).
19. THE Gate SHALL carry a `procedurePolicies` entry for every new procedure (AGENTS.md).

### Requirement 7: Party ledger, settlements and realised FX (G7)

**User Story:** As an accounts clerk, I want receipts and payments applied to specific open items, so that receivables and payables reconcile.

#### Acceptance Criteria

1. THE Settlement_Service SHALL keep an append-only party ledger with open items per instalment in the instalment's currency (RM reconciliation equation 1).
2. WHEN a payment settles several instalments, THE Settlement_Service SHALL store line-level applications with both endpoints, amounts in each currency, base amount and reversal link (RM Settlement).
3. FOR ALL payments and instalment sets, THE Settlement_Service SHALL make allocations sum exactly to the payment amount using `types.Allocate` (allocation-sums property, KS §3.4).
4. THE Settlement_Service SHALL lock both residuals while allocating and SHALL enforce an aggregate ceiling under the lock (RM contracts).
5. WHEN a foreign-currency settlement uses a different rate than the invoice, THE Settlement_Service SHALL post the realised gain or loss through the Posting_Engine (RM fixture USD 100, KES 130 and 132).
6. WHEN a settlement is cancelled, THE Settlement_Service SHALL restore the open items by mirrored applications (KS §6.2).
7. THE Settlement_Service SHALL keep unapplied cash separate from open items (RM equation 1).
8. FOR ALL cutoffs, THE sum of signed party residuals SHALL equal the company's control account balance (KS invariant 5, RM contracts).
9. THE CLI SHALL provide `athena settle apply|reverse|open-items` (RD §8).

### Requirement 8: Jobs, outbox, events, workflows and schedules (G8)

**User Story:** As an operator, I want committed facts delivered once per consumer and long waits to survive restarts, so that follow-up work is reliable.

#### Acceptance Criteria

1. THE Event_Bus SHALL write a durable business event envelope in the same transaction as the business change (PE §5 Runtime contract).
2. THE Event_Bus SHALL carry event type, schema version, immutable event ID, Workspace, producer, subject type, ID and revision, actor, Operation_ID, correlation and causation IDs, recorded sequence and minimal payload (PE §5).
3. WHEN a consumer handles an event, THE Event_Bus SHALL store the consumer's inbox row and local effect in one transaction, and SHALL skip an event already recorded for that consumer and effect key (PE §5).
4. THE Job_Runner SHALL lease jobs with expiry and fencing, retry with backoff, and assign permanent failures to an owner (PB §2, PE §5).
5. WHEN the process stops after commit and before dispatch, THE Job_Runner SHALL recover the outbox row and deliver it (PE §6).
6. THE Event_Bus SHALL let a Pack declare events and subscriptions with mode, action, effect key and exhaustion behaviour (PE §5 example).
7. THE Workflow_Engine SHALL store instances with definition version, phase, expected facts, wait, due time, assigned Role and Scope, and completion evidence (PE §5).
8. WHEN two conditions gate one task, THE Workflow_Engine SHALL create exactly one task in either arrival order (PE §5 pick-task gate).
9. THE Job_Runner SHALL give each schedule occurrence a unique key over Workspace, schedule, scope and period, and SHALL track timezone, catch-up policy, lease, watermark and result (PE §5 Schedule semantics).
10. WHEN a scheduled expiry runs, THE Job_Runner SHALL recheck the subject under lock before changing it (PE §3, §5).
11. IF a reaction would raise a cycle past its declared budget, THEN THE Event_Bus SHALL stop it and show an operator-visible state (PE §5).
12. THE Event_Bus SHALL replay a projection without sending messages or running business commands (PE §5 Replay).
13. THE Core SHALL store secret references, not secret values, and SHALL keep a secret value out of Record CRUD, Pack source, Starlark, URLs, frontend responses, diffs, Reports, logs and spans (IO §3 Secret storage).
14. THE Core SHALL provide a key service interface for field encryption with algorithm, key reference, nonce and authenticated context binding Workspace, Record and field (IO §3).
15. IF a protected value is copied into another Record, THEN decryption SHALL fail (IO §3 test).
16. THE Core SHALL record each Record Type's lifetime profile, retention trigger, class, disposal action, archive readers and hold behaviour (IO §2).
17. WHEN a disposal job runs, THE Job_Runner SHALL preview candidates, skip held items and dependent evidence, and record outcome and exceptions (IO §2).
18. WHEN a backup is restored, THE Core SHALL reapply the disposal register before reopening (IO §2 acceptance case).
19. THE Job_Runner SHALL purge expired Challenges and sessions and keep a minimal Access Log event (IO §2 transient data).
20. THE Desk SHALL provide Build pages for Jobs, Events and Workflows and a Settings page for secret references with Connect, Test, Rotate and Revoke actions (IO §3, PE §5 operators).
21. THE CLI SHALL provide `athena jobs|events|workflows|secrets|retention` commands (RD §8).
22. THE Access_Service SHALL deliver invites and resets through the outbox WHERE a mail destination is configured (PB §3 step 3).
23. OPEN: THE sponsor SHALL choose retention periods and key custody for the pilot deployment (PB §7, IO §3).

### Requirement 9: Balances, Query compiler, Reports v1 and Report Runs (G9)

**User Story:** As a controller, I want reports I can reproduce and explain, so that a closing figure ties to ledger rows.

#### Acceptance Criteria

1. THE Balance_Service SHALL update Account Balance and Party Balance in the posting transaction and keep a watermark and generation per Balance (RM dictionary).
2. WHEN a rebuild runs, THE Balance_Service SHALL build a new generation, reconcile it and switch atomically (RM dictionary).
3. FOR ALL ledgers, a rebuilt Balance SHALL equal the stored Balance at the watermark (KS invariant 3).
4. THE Query_Compiler SHALL compile a typed Query to SQL with the acting user's Scope and row conditions applied at the source before joins, paging, totals and export (PB §3, RA A4).
5. THE Query_Compiler SHALL aggregate each side of a join to its grain before joining, and reject a join that multiplies rows (PB §4).
6. THE Report_Service SHALL ship Trial Balance, General Ledger, Profit and Loss and Balance Sheet definitions as Core test fixtures (PB §4, RD P14).
7. WHEN a Report runs, THE Report_Service SHALL create a Report_Run with definition and Rule hashes, parameters, date basis, timezone, cutoff, ledger high-water mark, requester, Scope, row count and content digest (PB §4 Report Run contract).
8. FOR ALL Report_Runs, rerunning with the stored high-water mark SHALL return identical results after later backdated entries (KS invariant 9).
9. THE Report_Service SHALL show generated time, effective date and freshness separately (PB §4).
10. WHEN a figure is clicked, THE Report_Service SHALL return drill-down to ledger rows and vouchers (KS invariant 3, PB §4).
11. THE Report_Service SHALL save the scoped query plan and timings for A4 evidence (RA A4).
12. THE Desk SHALL add a Reports page to Build and run views in View, and AGENTS.md SHALL list it (RD §8 answer D).
13. THE CLI SHALL provide `athena report list|run|show|rebuild-balances` (RD §8).
14. WHEN a Scope is removed while a Report is queued, THE Report_Service SHALL refuse delivery (RA P0).

### Requirement 10: Pack packaging (G10)

**User Story:** As a Pack author, I want an installable, verifiable Pack with a preview, so that installs are repeatable and safe.

#### Acceptance Criteria

1. THE Pack_Manager SHALL read `pack.yaml` and Module manifests with API version, dependencies, capabilities, Role and Grant templates, migrations, seed revisions, tests and licence (RM Pack author, PB §7 Pack upgrades).
2. THE Pack_Manager SHALL verify content hashes and resolve dependencies before planning (PB §7).
3. WHEN a plan is requested, THE Pack_Manager SHALL show tables, seeds, Grants, privilege changes and Overlay conflicts without writing (PB §3, PB §7).
4. IF a Pack update expands access, THEN THE Pack_Manager SHALL require explicit approval of the privilege diff before activation (PB §3 Select permissions).
5. THE Pack_Manager SHALL identify each seed by Seed_Identity and store the last installed hash and version apart from local edits (RM seed rerun).
6. WHEN a seed collides with a local edit, THE Pack_Manager SHALL present a three-way diff and SHALL not recreate a retired row as new (RM seed rerun).
7. THE Pack_Manager SHALL record checkpoints, run under the Workspace lock and resume after interruption (RM seed rerun, IO installation contract).
8. THE Pack_Manager SHALL store Overlays as declared Workspace differences against a pinned Pack version (PB §1).
9. IF a Pack is removed while data, holds or dependent Reports exist, THEN THE Pack_Manager SHALL refuse it (IO installation contract).
10. THE Pack_Manager SHALL install a verified local Pack archive without vendor credentials (PB §7 open-source deliverable).
11. THE CLI SHALL provide `athena pack plan|install|list|verify` (RD §8).
12. THE Desk SHALL extend `web/src/pages/PacksPage.vue` with the plan preview and installed list.

### Requirement 11: Stock ledger, reservations, serial and batch, valuation (G11)

**User Story:** As a storekeeper and controller, I want every unit and its cost traced from receipt to sale, so that stock value equals the books.

#### Acceptance Criteria

1. THE Stock_Service SHALL keep a stock ledger by owner, company, item, lot or serial, location and source line with signed base units and value (RM dictionary, contract 1).
2. THE Stock_Service SHALL keep valuation layers and Item application entries that link each issue to the receipt that supplied it and each return to the issue it reverses (RA Business Central row).
3. WHERE the sponsor chooses FIFO or moving average, THE Stock_Service SHALL apply that method per Item (OPEN valuation method, PB §7).
4. THE Stock_Service SHALL keep Stock Bin Balances rebuildable from the stock ledger (RM dictionary).
5. WHEN a late freight invoice allocates cost to received units, THE Stock_Service SHALL split it between units on hand and units sold by appending cost-delta rows (RM late freight fixture, RA P0).
6. THE Stock_Service SHALL never rewrite or update a historical ledger row to change valuation (RA P0).
7. WHEN a backdated movement changes cost, THE Stock_Service SHALL append deltas with the original economic date and an allowed posting date (RA P0).
8. FOR ALL ledgers, stock value in the stock ledger SHALL equal the stock control account in the GL at the same cutoff (stock-equals-GL property, KS invariant 4).
9. THE Stock_Service SHALL keep reservations separate from on-hand, so available equals on-hand minus active reservations (PE §3).
10. WHEN two carts compete for the last unit, THE Stock_Service SHALL give a hold to one under a lock (PE §3, §6).
11. THE Stock_Service SHALL require one identity per integral unit for serial-tracked Items and carry lot identity through receipt, issue, return and drill-down (RM UoM, RA Odoo row).
12. THE Stock_Service SHALL support quarantine and disposition by location and ownership, without changing company stock value on a move to quarantine (PE §2, RM lot cycle).
13. WHEN an interrupted rebuild restarts, THE Stock_Service SHALL show the watermark and block close on a pending valuation (RA A2).
14. WHEN a count is posted, THE Stock_Service SHALL post only the delta against movements since the cutoff (PB §5, RM stock counting).
15. THE CLI SHALL provide `athena stock balance|ledger|reserve|rebuild|check` and the Desk SHALL show a stock-to-GL check (RD P18, §8).
16. IF the stock-to-GL check differs, THEN THE Stock_Service SHALL write an Integrity and Incident Log entry with expected, observed and owner (RM equations, PB §4).

### Requirement 12: Remaining ledgers (G12)

**User Story:** As a finance lead, I want the tax, asset, budget, project, leave, meter and intercompany ledgers on the shared foundation, so that each domain posts through one engine.

#### Acceptance Criteria

1. THE Core SHALL provide a tax ledger and SHALL freeze applied tax components, base and filing period on each voucher line (RM Tax).
2. THE Core SHALL provide an asset ledger by Depreciation Book where only posting-enabled books create GL rows (RA P1, RM equation 4).
3. WHEN an asset's custodian changes with no monetary effect, THE Core SHALL write no GL row (RA P1).
4. THE Core SHALL provide a budget and commitment ledger where available equals budgeted minus committed minus actual (KS §6.2, RM Budget).
5. THE Core SHALL provide a project and cost ledger, a leave and entitlement ledger and a meter ledger (RM Additional Packs).
6. THE Core SHALL provide an intercompany ledger where paired rows share a pair ID and net to zero across companies (RM cycle table).
7. FOR ALL asset schedules, opening carrying amount plus additions minus depreciation, impairment and disposals SHALL equal closing carrying amount per book (RM equation 4).
8. THE Core SHALL reject direct writes to each of these ledgers outside the Posting_Engine (RD P11).
9. THE Core SHALL provide only the ledger mechanism and no country tax rate or depreciation method default (RM Business configuration).
10. THE CLI SHALL provide `athena ledger show <ledger>` for each ledger (RD §8).

### Requirement 13: Trees, effective dating, determination and UoM (G13)

**User Story:** As an administrator, I want hierarchies, dated references and unit families, so that history stays reproducible.

#### Acceptance Criteria

1. WHERE a Record Type is flagged as a tree, THE Core SHALL store path and depth and prevent cycles and cross-company links (RM dictionary, KS §10.5).
2. WHERE a Record Type is effective-dated, THE Core SHALL reject overlapping ranges and resolve "as of" lookups end-exclusive (RM date field family, KS §10.6).
3. WHEN Account Determination candidates exist, THE Core SHALL choose the most specific effective match within company and book and store the resolved account and mapping revision (RM contract 2).
4. THE Core SHALL provide UoM families with exact factors and item-specific overrides, and SHALL store entered unit, factor and base quantity on each line (RM UoM).
5. IF a conversion crosses families, THEN THE Core SHALL reject it (RM provisioning).
6. FOR ALL entered quantities, converting to base units and back SHALL return the entered quantity within the declared scale (round-trip property, KS §10.7).
7. THE CLI SHALL provide `athena tree|uom|determination` commands for these records (RD §8).

### Requirement 14: Complete report engine (G14)

**User Story:** As a controller, I want scheduled private reports, a Closing Pack and a way to explain any figure, so that I can sign off a close.

#### Acceptance Criteria

1. THE Report_Service SHALL run schedules through the Job_Runner with unique occurrence keys (PB §4 Automatic generation).
2. WHEN a scheduled Report runs and again at delivery, THE Report_Service SHALL recheck the owner's and each recipient's current Grants and Scope (PB §4).
3. THE Report_Service SHALL deliver an authenticated link and keep artifacts private (PB §4).
4. THE Report_Service SHALL create a Closing_Pack with tie-outs, exceptions and sign-offs, and lock it against regeneration (RM dictionary, RD P21).
5. IF a material discrepancy is unresolved, THEN THE Report_Service SHALL block the close (PB §7 gate 3).
6. THE Report_Service SHALL provide control Reports for unbalanced vouchers and stock versus GL, empty on a healthy ledger (RD P21).
7. WHEN a user selects "Explain this movement", THE Report_Service SHALL return ledger row, voucher and source line, transition, Rule and config revisions, originating actor or provider evidence, and any correction (PE §5 Explain).
8. THE Desk SHALL show work queues with owner, age, next action and source Record, and an Activity timeline on each Record in View (PE §5 operators).
9. THE Desk SHALL show only states backed by evidence in the Activity timeline (PE §5).
10. THE CLI SHALL provide `athena report schedule|deliver|close-pack|explain` (RD §8).

### Requirement 15: Integration surface (G15)

**User Story:** As an integrator, I want verified provider callbacks, attachments, search and comments through the same permission gate, so that outside input cannot bypass Core checks.

#### Acceptance Criteria

1. THE Provider_Inbox SHALL verify a webhook's signature, merchant, amount and identity before recording evidence (PE §2, CC §3).
2. WHEN a duplicate provider event arrives, THE Provider_Inbox SHALL record it once and map it to one Operation_ID (PB §6).
3. WHEN a signing key rotates, THE Provider_Inbox SHALL accept the previous key for a bounded period and deduplicate events independently (IO §3).
4. THE Core SHALL store attachments content-addressed with owner, digest, media type, size and immutable version, and SHALL authorise each download (PB §6, RM field families).
5. WHEN an upload is abandoned, THE Job_Runner SHALL remove it (PB §6 S3 row).
6. THE Core SHALL provide search through a dialect method and SHALL filter results by the same Scope as reads (RD P22).
7. THE Core SHALL provide comments and mentions that inherit Record visibility (PB §8).
8. THE Core SHALL provide CSV and JSON import and export with dry run through the same Gate (RD P22).
9. THE Core SHALL provide no unsupported "mark paid" path from a provider callback (CC §8 pilot 2).
10. THE CLI SHALL provide `athena inbox|attachments|search|import|export` (RD §8).

### Requirement 16: PWA and offline stock count (G16)

**User Story:** As a storekeeper, I want to count stock offline and submit when I return to coverage, so that a count survives patchy connectivity.

#### Acceptance Criteria

1. THE Desk SHALL ship a manifest, a versioned service worker and an offline shell that caches only public hashed assets (PB §5).
2. WHERE the Workspace opts in on a trusted device, THE Desk SHALL store count lines in IndexedDB with actor, Workspace, draft UUID, revision, schema version, cutoff and stable line IDs (PB §5).
3. THE Desk SHALL show Saved on this device, Awaiting sync, Needs review and Accepted by server as distinct states (PB §5).
4. WHEN a draft is replayed, THE Sync_Service SHALL check session, Grants, Record Type, revision and count baseline (PB §5).
5. WHEN the same draft is replayed with the same payload, THE Sync_Service SHALL return the original draft (PB §5).
6. WHEN stock moved since the count cutoff, THE Sync_Service SHALL show the movements and require supervisor approval of a variance (PB §5).
7. THE Sync_Service SHALL never add a resubmitted absolute count to the ledger (PB §5).
8. IF the user's Role was revoked while offline, THEN THE Sync_Service SHALL deny the replay (PB §5 acceptance run).
9. THE Desk SHALL warn before logout with unsynced drafts and purge private drafts on logout or account switch (PB §5).
10. THE acceptance run SHALL download a 12-line count, scan three serials, disconnect, restart, edit one line, reconnect and interrupt after server commit, and SHALL produce three identities once and one accepted draft (PB §5 acceptance run).
11. THE CLI SHALL provide `athena sync status|inspect` (RD §8).

### Requirement 17: Business starter Packs and setup (G17)

**User Story:** As an owner, I want a Business starter installed through the Setup_Wizard, so that I can trade and close without opening Build.

#### Acceptance Criteria

1. THE Setup_Wizard SHALL offer Business starter or Empty Workspace and, for Business starter, install the first-party Packs through the Pack_Manager (RM What appears).
2. THE Core SHALL ship Party with Customer and Supplier profiles, Item and Price, Buying, Selling, Stock, Expenses, Finance and Reports as Packs, and SHALL seed none into the core shell (RM, AGENTS.md D-007).
3. THE Setup_Wizard SHALL show Company setup readiness that names each unresolved mapping by action (RM provisioning).
4. WHEN a company uses services only, THE Setup_Wizard SHALL let setup skip warehouse configuration (RM provisioning).
5. THE Packs SHALL pass the purchase, sell, return and close fixture with closing Stock 300, Bank 150, COGS 300, Payable 300 and Revenue 450, trial balance 750 per side, net income 150 (RM worked records).
6. THE Packs SHALL pass the late freight fixture with Stock 330, gross profit 120 and the earlier Report_Run still at 300 (RM late freight).
7. THE Packs SHALL pass the employee advance, asset, FX, production split and 10.00 three-way split fixtures (RM fixtures: 3.34, 3.33, 3.33).
8. THE Packs SHALL pass cases A1, A3 and A4 (RA table).
9. THE Packs SHALL keep demo transactions in a separate demo Workspace (RM seed rerun).
10. FOR ALL totals split by `types.Allocate`, the parts SHALL sum to the total (allocation-sums property).
11. OPEN: THE sponsor SHALL name the pilot company, jurisdiction, chart and valuation method before a production-ready claim (RA release decision).
12. THE CLI SHALL provide `athena setup status|run` (RD §8).

### Requirement 18: Industry and channel Packs (G18)

**User Story:** As an adopting business, I want installable industry and channel Packs with declared dependencies and proofs, so that I add only what I need.

#### Acceptance Criteria

1. THE Pack_Manager SHALL install each Pack below with declared dependencies, events, posting ownership and tie-out Reports, as Packs outside the core shell (IO §1, RM Pack author).
2. THE POS and supermarket Pack SHALL provide the assisted lane, tender attempts and drawer close, and SHALL pass the KES 1,350 incident with one invoice, one stock issue and remaining due zero (CC §3, §2).
3. THE POS Pack SHALL pass the drawer fixture with expected drawer 148, net revenue 280, net tax 28 and net tenders 308 (IO §5).
4. THE payments connector SHALL create one Tender Attempt per amount and SHALL reconcile an unknown outcome before any second charge (CC §3, PE §3).
5. THE catalogue and channels Pack SHALL publish listings and import channel orders by external order ID (CC §4, §8).
6. THE WhatsApp binding SHALL change exactly one authorised Record per completed Flow (CC §5).
7. THE Learning Pack SHALL pass the 74 to 77 grade correction and transcript supersession (CC §6, §8).
8. THE Construction Pack SHALL pass the Friday fixture (gross 232) and the capital fixture (CIP 9,000, depreciation 120) (IO §6).
9. THE Restaurant Pack SHALL pass the recipe fixture (5 kg raw and 1 kg cooked closing) (IO §7).
10. THE Dairy Pack SHALL pass the collection fixture (A 57.20, B 40.00) (IO §8).
11. THE Poultry Pack SHALL pass the flock-day fixture (490 hens, 84 percent yield), and the optional hatchery SHALL pass the hatch fixture (81, 90 and 79 percent) (IO §9, §10).
12. THE Butchery Pack SHALL pass the cutting fixture (allocations 300, 160, 40) and the mixed-batch recall (IO §11).
13. THE Manufacturing Pack SHALL pass the partial-completion fixture (WIP 1,400 to zero) (IO §12).
14. WHEN Butchery and Restaurant are installed together, THE Packs SHALL pass the combined fixture: stock 65, COGS 35, revenue 112, gross profit 77 (IO §13).
15. WHEN a Pack is upgraded, THE Pack_Manager SHALL replay at least two sharing Packs' fixtures before activation (IO §1).
16. THE Core settings in G5 and key custody in G8 SHALL serve these Packs, and no Pack SHALL hold a secret value (IO §3, §4).
17. THE Pack readiness SHALL progress Installed, Configuration incomplete, Sandbox verified, Awaiting external approval, Live verified, Degraded or Suspended (CC §8).
18. OPEN: THE sponsor SHALL choose which Packs to build first, the pilot country and the provider qualification path (IO §13 research, RA release decision).
