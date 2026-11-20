# Athena ERP kernel roadmap

**Status:** APPROVED by the user on 2026-11-18 with the answers in section 8. Individual ADRs stay PROPOSED until their phase lands and the user accepts them; each phase still stops for approval before the next starts.
**Source spec:** `/Users/iampaulking/Downloads/ERP Kernel Specification Records, Ledgers, Operations, Reports.md` (sections 0 to 24, read in full).
**Written:** 2026-11-18. Work happens in place on `main`; nothing is committed unless the user asks.

## 1. Goal and how to read this

The goal is an Athena that works as the spec defines it: Go kernel (records, lifecycle, posting engine, ledgers, reports, logs, permissions) plus configurable logic on top. The spec's build order (section 16.1, milestones M0 to M10) drives the phase order. Phases are numbered P0, P1, ... and each is sized for one review.

The spec marks work **Kernel** (Go) or **Module/Pack** (definitions and Starlark on top). This roadmap plans all Kernel work as phases. Catalogue work (M5 order to cash, M6 procure to pay, and so on) is Module work. It is planned only as gated, conditional phases at the end, because it conflicts with the product scope (section 4).

### Constraints that apply to every phase

- Go backend; Vue 3 + TypeScript + Vite; pnpm only (pinned 11.22.0, committed `web/pnpm-lock.yaml`).
- UI: shadcn-vue components (Reka Luma, Zinc tokens, `web/src/theme.css`, `web/src/typography.css`, pointer cursors, light/dark toggle, no hard-coded colors). Action results are Sonner toasts at the bottom right, not inline banners. Build (Overview, Records, Modules, Rules) and View (saved Records only) stay. Entry forms validate with `web/src/fieldRules.ts`, which must mirror the server rules.
- API is ConnectRPC + Protobuf only (`RecordService`, `EntryService`, `SystemService`, plus new services where listed). No parallel JSON endpoints. The user said gRPC; ConnectRPC speaks the gRPC protocol and is the accepted choice (D-003), so it stays.
- One Go process serves the embedded frontend and API on `127.0.0.1:3000`. There is no authentication or authorization today. Authoring and entry endpoints stay loopback-only until P8 ships and is verified.
- OpenTelemetry stays optional and off unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. No entry values, credentials or request bodies in spans or span attributes. Every new kernel operation (posting, lifecycle transition, repost, report run, job) gets a span carrying only ids, counts, durations and outcome codes.
- Starlark never bypasses Go permission checks, invariants or transaction boundaries. It is not implemented and is not described as production-ready until it is.
- Reuse before creating; no backup or copy files; delete replaced code in the same change; use libraries already in `go.mod` / `web/package.json` first (new ones are named per phase with a reason).
- Never commit credentials, `.cache`, `node_modules` or build output.

### Common verification (every phase)

1. `go build ./...`, `go vet ./...`, `go test ./...` pass.
2. After any proto change: `make generate` (needs Buf), and a second `make generate` leaves no further diff.
3. In `web/`: `CI=true pnpm --dir web run typecheck` and `CI=true pnpm --dir web run build` pass, and the frontend tests added in P0 pass.
4. UI phases: `make restart` against local MariaDB (`.env.local`, MariaDB 11.8 from Homebrew), confirm the desk shows the database connected, exercise the flow in a browser in both themes, and confirm the embedded build in `internal/webui/dist` was rebuilt (so the served build is not stale).
5. From P1 on, database-touching tests run against MariaDB through `ATHENA_TEST_DATABASE_URL`; from the PostgreSQL phase (P10) on, against both engines.

### Common documentation updates (every phase)

Update `docs/architecture/overview.md` (status table and the affected sections, implemented versus intended kept separate), `README.md` commands if any changed, `docs/README.md` for each new doc, `docs/decisions/decision-log.md`, and an ADR from `docs/decisions/adr-template.md` when the phase changes system boundaries, durable data, extension contracts, security or deployment. Keep `AGENTS.md` aligned when an accepted decision changes. Never write an open option as settled.

## 2. What exists today (surveyed 2026-11-18)

Working tree has about 57 uncommitted changes (in progress). Phases must not revert them.

| Area | Current state |
| --- | --- |
| Records | `RecordService` (List, Plan, Save, Delete). A definition maps to table `athena_record_<key>`. Field types: Text, Long text, Email, Phone, Number, Currency, Date, Select, Link, Checkbox. Every table has `id` (UUIDv7 CHAR(36)), `revision`, `created_at`, `updated_at`. Saves go through the journaled publisher in `internal/server/records.go` (`athena_record_operations`, resumable at startup). |
| Migrations | `internal/server/migrations.go` plans field adds, removals, type/link changes and required-field backfills with confirmation. This is a per-save planner, not versioned migrations. System tables are created by an ad hoc `migrate()` in `internal/server/server.go` using MariaDB-only syntax (`ADD COLUMN IF NOT EXISTS`, `JSON`, `ON UPDATE CURRENT_TIMESTAMP`). |
| Entries | `EntryService` CRUD with optimistic `revision`, keyset paging, `athena_entry_history` (changed field keys, actor "Local administrator"). Values are strings/bool/null; numbers are exact decimal strings in `DECIMAL(18,6)`. |
| Missing from the spec | Record kinds, system fields (`docstate`, `version`, `owner`, `company`, `def_version`, `request_id`), exact money/quantity types, lifecycle (submit/cancel/amend), numbering, ledgers, posting engine, balances, reports, report runs, rules host, expression language, auth, permissions, jobs, logs beyond entry history, module/pack packaging, dimensions, periods, currencies. |
| Telemetry | `internal/telemetry`: otelconnect + otelsql, optional, browser spans forwarded through `/otlp/v1/traces`. Matches the constraint. |
| Frontend | Build/View modes, `RecordsPage`, `EntriesPage`, `EntriesTable` (TanStack), `MigrationDialog`, shadcn-vue components: alert-dialog, badge, button, card, input, label, select, separator, sonner, switch, table, tabs, textarea, marker. `ModulesPage` and `RulesPage` are placeholders. |
| Tests | **None.** There are no `*_test.go` files in the repository and no frontend test runner. |
| Projects | `athena init / new / run / serve`; one database per project (ADR-0004); one running project per user. |
| Tooling | Go 1.26, Buf, pnpm available. Local MariaDB 11.8 installed. **Docker is installed but its daemon was not running; `psql` is not installed.** |

### Inconsistencies found in the current docs (fixed in P0)

- `AGENTS.md` and ADR-0003 say only additive schema changes are allowed. `docs/architecture/overview.md` and `internal/server/migrations.go` implement destructive changes with confirmation and backfills. P0 reconciles them.
- `decision-log.md` has two rows with ID D-011.
- `docs/README.md` points at `record-and-module-model.md`, which exists; ADR-0002 still says the metadata endpoints use JSON/HTTP, which is stale.

## 3. Spec versus repository: conflicts to resolve (not decided here)

| # | Spec says | Repository says | Handling |
| --- | --- | --- | --- |
| C1 | Kernel knows no business, yet the movement catalogue (section 7) and "an ERP that functions as defined" are expected | D-007 / AGENTS.md: no domain Records or Modules seeded into the platform shell | Decision 1. Kernel phases are unaffected. Catalogue phases are gated. |
| C2 | Section 22 renames: Project becomes Pack, Project/instance becomes Workspace, Record becomes Record Type (definition) and Record (row), Rule means code only, permission rule becomes Grant | D-005: Record = definition, Record entry = row, Module = installable bundle. ADR-0004 uses "workspace" for the parent folder and "project" for the database. | Additional point A. Until decided, phases use the repo's current words. |
| C3 | Section 15.1 suggests SQLite first | ADR-0002 MariaDB first; user wants MariaDB and PostgreSQL interchangeable | Section 20 of the spec (dialect layer) is followed. SQLite is not planned. |
| C4 | Section 19.5: one process serves many instances | ADR-0004: one running project per user | Out of scope until tenancy (O-006) is decided. Not planned. |
| C5 | Ids are ULID or bigint; money is integer minor units; table names `p_<pack>__<record_type>` | UUIDv7 CHAR(36); `DECIMAL(18,6)`; `athena_record_<key>` | Additional point B. P3 and P5 need an answer before they change storage shape. |
| C6 | Conditions use a restricted CEL subset; logic uses Starlark | Only Starlark is in the accepted direction | Decision 4. |
| C7 | Reports are a first-class area | Build nav is fixed to Overview, Records, Modules, Rules | Additional point D. |

## 4. Phase list

Order follows spec section 16.1. M-numbers show the spec milestone each phase serves. Phases P0 to P2 are enabling work the spec assumes but the repo lacks (tests, dialect layer, versioned migrations).

| Phase | Name | Spec milestone |
| --- | --- | --- |
| P0 | Baseline: test harness and doc reconciliation | prerequisite |
| P1 | Storage dialect layer (MariaDB only) | M0, section 20 |
| P2 | Versioned migrations (OPEN design) | M0, sections 15.1, 20.3 |
| P2a | Vocabulary rename (spec section 22) and storage naming ADR | section 22, 20.3, 23.2 |
| P3 | Exact types: decimal, money, quantity, dates, allocation | M0, section 3 |
| P4 | Record kinds, system fields and extended field attributes | M0, sections 2, 3.2, 24 |
| P5 | Document lifecycle: submit, cancel, amend, events | M1, section 5 |
| P6 | Numbering, audit log with hash chain, request correlation | M1, sections 10.4, 12 |
| P7 | Expression language and computed fields | sections 3.3, 23.6 (OPEN) |
| P8 | Authentication and permissions | M1, section 21 (OPEN timing) |
| P8a | Workspace files: FileService, Files page, `athena files`, generated-source destination (REQUIRED) | sections 23.1, 23.2, 24, 13, 12.1, 21 |
| P9 | Starlark rules host | sections 13, 5.2 (OPEN) |
| P10 | PostgreSQL adapter and engine parity | section 20 (OPEN test setup) |
| P11 | Ledger foundation: dimensions, calendars, periods, currencies | M2, sections 6.1, 10 |
| P12 | Posting engine, general ledger, reversal, idempotency | M2, sections 6.4, 5.3 |
| P13 | Party ledger, settlements, realised FX | M2, section 6.2 |
| P14 | Balances, query compiler, reports engine v1, Report Run snapshots | M3, section 11 |
| P15 | Module packaging (OPEN design) | sections 14, 19, 23 |
| P16 | Jobs, outbox, reposting framework | sections 15.2, 6.4, 12.1 |
| P17 | Stock ledger: valuation layers, bins | M4, section 6.2 |
| P18 | Reservations, serial/batch, backdated repost, stock to GL check | M4 |
| P19 | Remaining ledgers: tax, asset, budget, project, leave, meter, intercompany | sections 6.2, 6.3 |
| P20 | Trees, effective dating, account determination, UoM | sections 8.1, 10.5 to 10.7 |
| P21 | Report engine complete: schedules, delivery, controls, Explain trail | M9, sections 11.5, 12.2 |
| P22 | Integration surface, attachments, search, collaboration objects | sections 10.10, 10.12, 15.1 |
| P23 | Catalogue modules M3 to M10 (GATED by decision 1) | M3 to M10, section 7 |

P0 to P14 plus P8a are the first batch to author as workflow steps (about 16 phases). P15 onward is authored as the next batch.

## 5. Phases in detail

### P0. Baseline commit, test harness and doc reconciliation

- **Baseline commit (user decision C):** first step of P0. Before committing, scan the working tree for credentials, DSNs and secrets (`.env.local`, DSN strings in docs, `.pnpm-store`, `.cache`, `node_modules`, `bin`, `internal/webui/dist` build output) and follow `.gitignore`; add ignore rules only if something that must be excluded is not covered; stage specific paths, never `git add -A`. One Conventional Commit, for example `feat: add record entries, migrations, telemetry and project workspace`. No push, no PR, no deploy. Record the commit hash and the exclusion scan in `.agents/tasks/phase-0-baseline/notes.md`. From then on phases still do not commit.

- **Spec:** none directly; section 16.2 needs a test suite to exist.
- **Exists:** no Go tests, no frontend tests. Docs inconsistent (section 2).
- **Depends on:** nothing.
- **Touches:** `internal/server/*_test.go` (new), `internal/testdb` helper (new, creates a throwaway database per test from `ATHENA_TEST_DATABASE_URL`, skips when unset), `web/package.json` plus a test runner and `web/src/fieldRules.test.ts`, `Makefile` (`make test`), `AGENTS.md`, `docs/decisions/decision-log.md`, `docs/decisions/adr-0003-record-table-model.md`, `docs/decisions/adr-0002-mariadb-first.md`, `README.md`.
- **Work:** characterization tests for `SaveRecord`/`PlanRecordSave`/`DeleteRecord` and `EntryService` (create, update with stale revision, delete, paging, history). Frontend: add Vitest (the Vite-native runner; a new dev dependency, justified because `fieldRules.ts` must mirror server rules and has no test). A shared table of valid and invalid values checked by both Go and TypeScript tests keeps the two in step. Fix the doc inconsistencies. No behavior change.
- **Verification:** common verification; `make test` runs Go and web tests; new Go tests pass against local MariaDB and skip cleanly without it.
- **Kernel invariant tests added:** optimistic concurrency (stale revision rejected), history written in the same transaction as the change, server/client field rule parity.

### P1. Storage dialect layer (MariaDB only)

- **Spec:** section 20 (portability rules, `Dialect` interface), 15.1 ("SQL generation behind an interface"), 20.5 (`RecordStore` seam).
- **Exists:** MariaDB SQL is inlined in `records.go`, `entries.go`, `migrations.go`, `server.go` (`GET_LOCK`-style schema lock, `ADD COLUMN IF NOT EXISTS`, `information_schema` lookups, MySQL error numbers, backtick quoting, 64-char identifier assumptions).
- **Depends on:** P0 (tests protect the refactor).
- **Touches:** new `internal/store` package (`Dialect` interface plus `mariadb.go`), then `internal/server/{records,entries,migrations,server}.go` call it. `docs/decisions/adr-0005-database-dialect-layer.md` (**PROPOSED**), decision-log row, `adr-0002` gets a "superseded in part, if accepted" note only after acceptance.
- **Work:** extract only what two engines will differ on: identifier quoting and length limit with deterministic hash shortening, type mapping, DDL for create/add/alter/drop, upsert, pagination, row lock clause, advisory/schema lock, JSON column type, error classification (duplicate key, deadlock, serialization failure), and `now()` handling. One implementation now; the interface exists because PostgreSQL is planned (P10). No behavior change. Do not add PostgreSQL code or claim support.
- **Verification:** common verification; all P0 characterization tests unchanged and green.
- **Invariant tests:** identifier shortening is deterministic and within 63 characters (the stricter PostgreSQL limit); error classifier maps known MariaDB errors.

### P2. Versioned migrations (OPEN design)

- **Spec:** 15.1 (migration planner, idempotent and resumable, state table), 20.3 (MariaDB commits DDL implicitly), 24.4 (`renamed_from`), 14 migration section of definitions.
- **Exists:** per-save planner and journal; ad hoc system-table creation. Field and Record keys cannot be renamed.
- **Depends on:** P1 (dialect handles DDL syntax).
- **Touches:** `internal/store/migrate.go` (new: migration state table `athena_migrations`, ordered steps, each idempotent), `internal/server/server.go` (replace `migrate()` and `addEntryRevisions`, delete the old code in the same change), `internal/server/migrations.go` (reuse planner for definition-driven steps), ADR-0006 **OPEN** until the user decides (decision 2), `docs/decisions/decision-log.md` (resolve O-003 only if accepted).
- **Work:** a numbered, recorded sequence of system migrations replaces the ad hoc statements. Definition migrations (add `kind`, system columns, renames via `renamed_from`) are expressed as plans stored with `def_version`. This phase is the gate for P3/P4/P5: spec features that need new columns on populated tables (system fields, `docstate`, `version`) cannot ship before it.
- **Verification:** common verification; test that a fresh database and a database created by the pre-P2 build both reach the same schema; interrupted migration resumes; running twice changes nothing.
- **Invariant tests:** migrations are idempotent; resume after simulated crash between steps.

### P2a. Vocabulary rename and storage naming (user decision A and B)

- **Spec:** 22 (all of it), 23.2 (naming), 20.3 and 20.4 (identifier limits, table names), 17.2.
- **Exists:** D-005 vocabulary (Record = definition, Record entry = row, Module = installable bundle, project = database, workspace = parent folder per ADR-0004) is used in proto (`RecordService`, `Record`, `EntryService`), Go, web labels, CLI help and docs. Persisted names: `athena_records`, `athena_record_<key>`, `athena_record_operations`, `athena_entry_history`, `athena_modules`, `athena_rules`, FK names `fk_record_*`.
- **Depends on:** P2 (persisted renames run as recorded, idempotent migrations; existing stored metadata and tables must keep working).
- **Mapping (old -> new), to be finalised in the ADR that supersedes D-005:** Record (definition) -> Record Type; Record entry -> Record; Module (installable bundle) -> Pack; Module as functional area stays "Module" in its narrowed meaning; project (database) -> Workspace; permission rule -> Grant; Rule stays and means Starlark code only. **Conflict to resolve in the ADR:** ADR-0004 already uses "workspace" for the parent folder holding projects. The phase must rename that folder concept to a distinct word (proposal: "home") so no word means two things, and state the choice. No mixed vocabulary may remain.
- **Touches:** proto (`RecordService` -> `RecordTypeService`, `EntryService` -> `RecordService`, messages and fields; breaking change accepted because nothing is released; `make generate`), Go packages and handlers, web labels, routes, `fieldRules.ts`, `AGENTS.md`, README, CLI help and command names (`athena new` creates a Workspace), all docs, ADR superseding D-005 and amending ADR-0003/0004, decision log. Storage ADR (PROPOSED then accepted by user): money integer minor units plus currency; UUIDv7 ids (time-ordered, portable, no auto-increment gaps; kept over ULID/bigint); table naming `p_<pack>__<record_type>`, children `__<child_field>`, ledgers `l_<ledger>`, system tables `athena_<name>`, snake_case, 63-character limit with deterministic hash-suffix shortening, ASCII only, lower-case so MariaDB and PostgreSQL agree (PostgreSQL unquoted-identifier behaviour). Core Record Types built by the user before packs exist live under a reserved pack name `workspace`.
- **Detailed plan:** `.agents/tasks/phase-2a-vocabulary/plan.md` (mapping, name inventory, migration 5 `vocabulary_rename`, ADR-0008 and ADR-0009 drafts, ordered steps). The user answered YES: the ADR-0004 parent folder is renamed "workspace" to "home" (marker `athena-home.yaml`, `athena init` creates a home).
- **Migration/compat:** a P2 migration renames persisted tables and metadata columns (or adds a name map consulted by the dialect), idempotent and resumable; old databases open and keep entries.
- **Verification:** common verification; a database created by the pre-rename build opens and keeps its Records after the migration; browser pass of Build and View with the new labels in both themes; no old term remains (checked by reading the diff and a documented term search, with the allowed historical mentions listed in the ADR).
- **Invariant tests:** migration is idempotent and resumable; entry data survives the rename; generated identifiers are deterministic, lower-case and at most 63 characters.

### P3. Exact types: decimal, money, quantity, dates, allocation

- **Spec:** 3.1 (exact numerics, Money, Quantity, Time), 3.4 (money rules), decision 17.5.
- **Exists:** `Number`/`Currency` as `DECIMAL(18,6)` decimal strings; no currency reference, no quantity, no rounding.
- **Depends on:** nothing structural; run after P2 so any column change is a recorded migration.
- **Touches:** new `internal/types` (pure Go: decimal, money in minor units plus currency, quantity with UoM, rounding modes half_up and bankers, deterministic `Allocate`), later wired into `entries.go` by P4. No UI. ADR-0007 (**PROPOSED**): scaled integers versus decimal strings (spec decision 17.5, additional point B). Decimal library: evaluate `github.com/shopspring/decimal` against `math/big`; pick one and say why (new dependency otherwise unneeded).
- **Verification:** common verification; property tests (`testing/quick` or `pgregory.net/rapid`, named in the ADR).
- **Invariant tests:** allocation always sums back exactly; rounding never drifts across split and re-add; no float conversion anywhere (a test greps the package for `float`).

### P4. Record kinds, system fields and extended field attributes

- **Spec:** 2 (kinds), 2.2 (flags), 3.2 and 24.1 to 24.5 (system fields, attributes, extra types), 24.3 (naming forms, handled in P6).
- **Exists:** one implicit kind; fields have key, label, type, required, link target.
- **Depends on:** P2, P3. Needs the OPEN migration design accepted, because existing tables gain columns.
- **Touches:** `proto/athena/v1/records.proto` (kind, class, attributes: unique, default, min, max, pattern, max_length, read_only, set_once, no_copy, index; new types money, quantity, enum with values), `internal/server/{records,entries,migrations}.go`, `web/src/{types.ts,fieldRules.ts,pages/RecordsPage.vue,components/EntriesTable.vue}`, `web/src/gen/*` (generated). shadcn-vue components needed: `dialog`/`sheet` for field settings, `popover`, `checkbox` if absent (added with `pnpm dlx shadcn-vue@latest add` through the pinned pnpm).
- **Work:** kind selector in the Record builder (Master, Document, Child, Setting first; Ledger, Balance, Log, Snapshot, Meta are system-assigned and not user-creatable); system fields added by the kernel (`version`, `owner`, `created_by`, `modified_by`, `deleted_at` for Master, `def_version`, `request_id`); field attributes enforced server-side and mirrored in `fieldRules.ts`. Kept domain-neutral: no seeded Records.
- **Verification:** common verification plus a browser pass creating a Master and a Document-kind Record, entering entries, hitting each validation, light and dark.
- **Invariant tests:** Master soft delete never hard-deletes; `set_once` rejects change after first save; unique and required enforced on the server; `fieldRules.ts` and Go agree on the shared table from P0.

### P5. Document lifecycle: submit, cancel, amend, events

- **Spec:** 5.1 to 5.3, 2.2 (submittable/amendable flags), 12.1 (Document Version, Activity Timeline), invariants 16.2 (2 in part).
- **Exists:** only draft-style CRUD.
- **Depends on:** P4 (kinds and system fields), P2.
- **Touches:** `proto/athena/v1/entries.proto` (new RPCs `SubmitEntry`, `CancelEntry`, `AmendEntry` on `EntryService`; no new JSON), new `internal/lifecycle` (state machine, event dispatcher with the fixed order of 5.2, Go hook registry; Starlark attaches in P9), `internal/server/entries.go`, new tables `athena_document_versions`, `athena_activity`, `web/src/pages/EntriesPage.vue` plus `components/` for status badge and action buttons (shadcn `badge`, `alert-dialog`, Sonner toasts). OpenTelemetry: span per transition with record key, state from/to, outcome only.
- **Work:** `docstate` 0/1/2 owned by the kernel; draft editable, submitted immutable except `allow_on_submit` fields (audited); cancel blocked while a linked downstream document is submitted; amend creates a draft with `amended_from` and a suffixed name; `on_trash` only for drafts; Document Version snapshot at submit, amend and cancel. Business status stays module-owned and is not built here.
- **Verification:** common verification plus browser: create draft, submit, try to edit (rejected toast), cancel, amend.
- **Invariant tests:** immutability after submit (every non-`allow_on_submit` field); cancel blocked by live downstream link; amend preserves original as cancelled; event order matches 5.2; failure in `on_submit` rolls back state and versions together; stale `version` rejected.

### P6. Numbering, audit log with hash chain, request correlation

- **Spec:** 10.4, 24.3 (naming forms), 12.1 to 12.2 (Audit Log, tamper-evident, correlation), invariant 10, 6.1 (`request_id`, `created_by`).
- **Exists:** `athena_entry_history` (changed field keys, actor string). Telemetry trace ids exist but are not stored.
- **Depends on:** P5.
- **Touches:** new `internal/audit` (append-only Log writer with `hash`/`prev_hash` per log, verifier), new `internal/numbering` (series, gap-free counter row with `SELECT ... FOR UPDATE` through the dialect, manual override with uniqueness check, no renumbering after submit), migrate `athena_entry_history` into the Audit Log (delete the old table and writer in the same change, with a recorded migration that preserves rows), `request_id` set from a ConnectRPC interceptor and stored on rows and audit entries (the OTel trace id is not stored as an attribute of values; only the id). UI: history tab in `EntriesPage.vue` shows the field-level audit.
- **Verification:** common verification; concurrency test with parallel submits producing gap-free series.
- **Invariant tests:** hash chain verifies first to last (10), and detects an edited or deleted row; gap-free series has no gaps under concurrent submit and rolled-back submit; name permanent after submit; one `request_id` links the request, audit rows and documents.

### P7. Expression language and computed fields (OPEN)

- **Spec:** 23.6 (restricted CEL, `decimal` and `money` value types, SQL translation), 3.3 (formula, rollup, lookup), 24.4 (`required_if`, `read_only_if`, `depends_on`, `validate`).
- **Exists:** nothing.
- **Depends on:** P3, P4; decision 4 (engine choice).
- **Touches:** new `internal/expr` (parse, type-check at save time, evaluate, translate to SQL predicate through the dialect), computed field handling in `internal/server/entries.go` (dependency-ordered evaluation, rollups maintained by the kernel), frontend evaluation only for display hints; the server remains the authority.
- **Work:** `github.com/google/cel-go` (spec recommendation) versus a hand-written mini-language. Decision 4 settles it; the choice is recorded in an ADR. Expressions are side-effect free, terminating, no clock beyond injected `today()`.
- **Verification:** common verification; table-driven expression tests, including null semantics and `money` arithmetic.
- **Invariant tests:** same input gives same output (determinism); formulas evaluate in dependency order and a cycle is rejected at save; rollup equals recomputation from child rows; SQL translation returns the same rows as in-memory evaluation.

### P8. Authentication and permissions (OPEN timing)

- **Spec:** 21 (users, Argon2id, sessions, Roles, Grants, field levels, row conditions, data scopes, segregation of duties, Access Log), 10.9, M1 done-when "with audit and role checks".
- **Exists:** none. Actor is a hard-coded "Local administrator".
- **Depends on:** P6 (Access Log and `request_id`), P7 (conditions compile to SQL predicates). If decision 5 moves it earlier, P7's SQL translation is replaced by simple role checks first.
- **Touches:** new `proto/athena/v1/auth.proto` (`AuthService`), `internal/auth` (users, `credential`, `session`, Argon2id via `golang.org/x/crypto/argon2`, lockout), a single permission gate as a ConnectRPC interceptor so every service path, report and export uses it, `internal/server/*` call sites pass the acting user, login and user pages in `web/src` (shadcn `form` inputs, `dialog`, `dropdown-menu` for the user menu). Secure cookies, CSRF protection, content security headers. Breaking change: first-run needs an instance owner created through `athena` CLI. ADR-0010 (**PROPOSED**). Do not expose beyond loopback until this phase is verified; then update `AGENTS.md` guardrails.
- **Work:** default deny; additive grants per role; row conditions and user-permission scopes compiled into queries; field-level permission levels; segregation of duties (creator cannot approve); Access Log for logins, failures and denials. Credentials never in spans, logs or audit values.
- **Carry-overs from P6 and P7 (approved 2026-11-19):**
  - Audit masking: the Audit Log and Document Version snapshots hold field values. Fields above the user's permission level are masked when read through `ListRecordHistory`, `ListRecordVersions` and the CLI; the stored hash input stays unmasked so the chain still verifies. Reading the log needs its own permission.
  - Parent rollups update in the same save: a parent's rollup formula over a child Record Type can be saved together with the child (no second save), with a test. Today a child needs its parent saved first, then the parent is saved again.
  - The expression cache (compiled programs reused across requests) can wait; it is not part of P8.
  - PostgreSQL stays untested by design until its phase.
- **Verification:** common verification; browser: sign in, sign out, denied action shows a toast, role change takes effect.
- **Invariant tests:** no grant means deny; list queries never return rows the user cannot open; field above the user's level is hidden in responses and masked in logs; denial is written to the Access Log; failed logins throttle; passwords and tokens never appear in any log or span (tested with a capturing exporter).

### P8a. Workspace files (REQUIRED kernel feature)

Project file management is required (user instruction 2026-11-19). In the new vocabulary it manages the files of a Workspace: its directory on disk (Record Type definition files, Rule `.star` sources, generated Go source, Pack layout files) through the desk, the API and the CLI.

- **Spec:** 23.1 (folder layout), 23.2 (naming, `renamed_from`), 24 (Record Type files), 13 and 23.1 (`.star` sources), 12.1 (audit entries for file changes), 21 (Grants on files).
- **Placement and why:** immediately after P8 and before P9. It needs P8 because a write endpoint for host files must be deny-by-default behind the permission interceptor and must not exist on an unauthenticated server (the loopback-only rule). It needs P6 for the audit log and `request_id`, and P2a for the vocabulary, the `workspace` layout and the table and file names. It comes before P9 because the Rules editor stores `.star` sources as files, and before P15 because Pack build, install and overlays read and write the same layout. It also resolves the open question O-002 and D-009 (where generated source goes), through an ADR.
- **Depends on:** P2a, P6, P8. Uses `internal/workspace` (P2a) for the Workspace root.
- **Touches:**
  - `proto/athena/v1/files.proto` (new `FileService`: `ListFiles`, `GetFile`, `PutFile` with an expected content hash for optimistic concurrency, `DeleteFile`, `MoveFile`); no JSON endpoints.
  - `internal/files` (new): root-confined path resolver (reject absolute paths, `..`, and symlinks that leave the Workspace root; fixed allow-list of top-level folders such as `record_types/`, `rules/`, `packs/`, `generated/`; deny `.git`, `.env*`, the lock and runtime state; size and count limits), atomic write (temp file plus rename), SHA-256 content hash, writes recorded in the audit log with path, hash and actor only (never file content).
  - `internal/server/files.go` (handler, Grant checks for `files:read` and `files:write`), `cmd/athena/files.go` (`athena files list|get|put|rm|mv [--workspace slug] [--json]`, calls `internal/files` directly like the other CLI commands).
  - `web/src/pages/FilesPage.vue` (Build nav gains Files; shadcn `table`, `dialog`, `textarea`, `tabs`, `alert-dialog`; Sonner toasts for saved, conflict and denied; token colours only), `web/src/api.ts` (`fileClient`), `AGENTS.md` (Build list), README, CLI help.
  - ADR for the generated-source destination (next free ADR number, **PROPOSED**): where generated Go source for Record Types lands, who owns `*_gen.go` (the publisher; header marks it generated), how dashboard and codebase edits are reconciled (content hash plus conflict detection, never silent overwrite), and the rebuild boundary (generated Go takes effect only after a new build). Resolves O-002 and D-009 only after the user accepts it. The generator itself stays out of this phase unless the ADR says otherwise.
  - OpenTelemetry: a span per file operation with the operation name, byte count, outcome code; never the path contents beyond a hashed or relative path id, never file content.
- **Work:** list a tree of the Workspace files, read, edit and save with a hash check, create, move and delete; Record Type definition files under `record_types/` validate against the Record Type file format of spec 24 before saving and report errors with line numbers; `.star` files are stored and edited but not executed here (P9 runs them). Binary and oversized files are refused.
- **Verification:** common verification; `athena files list|get|put|rm` against a throwaway Workspace; browser: open the Files page, edit and save a Record Type file, trigger a stale-hash conflict toast, try a path outside the root and see it denied, both themes.
- **Invariant tests:** a path that escapes the root (absolute, `..`, symlink, encoded separators, case tricks) is rejected in every operation and nothing is written; a write with a stale hash fails and leaves the file unchanged; writes are atomic (a failure between temp file and rename leaves the old content); a user without the `files:write` Grant is denied and the denial is logged; `.env*`, `.git` and runtime state are never listed or readable; the audit entry for a write holds path, hash and actor and no content; the CLI and the API apply the same checks.

### P9. Starlark rules host (OPEN)

- **Spec:** 13.1 to 13.4, 5.2 (events), 12.1 (Rule Run), 23.1 (`.star` source).
- **Exists:** nothing; Rules page is a placeholder.
- **Depends on:** P5 (events), P6 (Rule Run log), P8 (rules act as the user), P7 (conditions).
- **Touches:** new `internal/rules` using `go.starlark.net` (standard Go implementation; confirm in decision 4), step and memory limits, no float, injected clock, API per rule type (read, set, `fail`, `warn`, `require`, money/qty helpers), Rule Run log with input hash and outcome, `web/src/pages/RulesPage.vue` (editor with shadcn `textarea`/`tabs`, run history, simulate/dry run with a diff), `proto/athena/v1/rules.proto`.
- **Work:** Validation, Derivation, Guard and Transform types first; Posting type arrives in P12; Reaction and Scheduled in P16. Cascade depth limit with cycle detection. Rules run inside the transaction through the lifecycle dispatcher and cannot reach SQL, files, network or randomness. Documented as not production-ready until P9 is accepted.
- **Verification:** common verification; browser: create a validation rule, see it reject a save with a toast, see the Rule Run.
- **Invariant tests:** deterministic replay of a stored input; step limit stops a runaway rule and logs the outcome; a rule cannot write a ledger or change a field outside its type; a rule that errors rolls back the document and its writes; every read is permission-checked as the acting user; floats rejected.

### P10. PostgreSQL adapter and engine parity (OPEN test setup)

- **Spec:** 20.1 (invariant suite runs on every engine), 20.2 type table, 20.3 pitfalls (DDL transactions, isolation, identifier length, case and collation, time in UTC).
- **Exists:** nothing; `psql` not installed; Docker daemon not running.
- **Depends on:** P1 (interface), P2 (migrations), P5 to P6 invariants exist so parity is meaningful. Placed here so P11 onward runs on both engines. Decision 6 may move it.
- **Touches:** `internal/store/postgres.go`, driver `github.com/jackc/pgx/v5/stdlib` (maintained, works with `otelsql`, so tracing carries over; confirm in the ADR), `athena init` and project templates accept `postgres` as `driver`, `internal/testdb` supports both engines, `docs/decisions/adr-0005` accepted or amended, ADR-0002 marked superseded in part (only after the user accepts), O-001 resolved.
- **Work:** implement every dialect method; use explicit row locks (`FOR UPDATE`) rather than isolation defaults; a `normalised` column for case-insensitive search; both engines at READ COMMITTED or a stated isolation level. Do not state PostgreSQL is supported until the whole suite passes on it.
- **Verification:** common verification plus the full suite against a real PostgreSQL instance (source set by decision 3).
- **Invariant tests:** the entire suite from P0 to P9 passes on both engines; migrations resume after interruption on PostgreSQL (DDL is transactional there, resume must still be safe); identifier shortening collision test.

### P11. Ledger foundation: dimensions, calendars, periods, currencies

- **Spec:** 6.1 (ledger columns), 10.1 to 10.3, 5.3 (period locks), 8 (Organisation, Accounting masters), 8.1.
- **Exists:** nothing.
- **Depends on:** P5 to P9 (permissions for override, rule logs), P3 (money).
- **Touches:** new `internal/ledger` (Ledger record kind, common columns, deterministic total order with `posted_at` and a tie-breaker sequence), tables for Dimension Definition, Fiscal Year and Period, Period Lock with override log, Currency and Exchange Rate (as kernel system definitions of kind Meta/Setting, not business Records seeded in the UI), `l_<ledger>` table naming through the dialect. Open sub-question inside decision 1: which masters the kernel owns (Company, Currency, Fiscal Year, Dimension) versus which a pack provides (Account, Party, Item). Default proposal: kernel owns only what ledger rows must reference to be valid (company, currency, period, dimension); Account and Party are declared capabilities that a pack satisfies, and tests use fixtures.
- **Verification:** common verification; browser: periods and period lock managed from Build, shown as shadcn tables and dialogs.
- **Invariant tests:** ledger tables reject any write not made by the posting engine (direct insert through `EntryService` fails); every ledger row has company, dimensions, `posted_at`; total order is stable.

### P12. Posting engine, general ledger, reversal, idempotency

- **Spec:** 6.2 (GL), 6.4 (posting plan), 5.3 (reversal generated, period locks, idempotency), 8.1 (account determination, in part), invariants 1, 2, 7, 8.
- **Exists:** nothing.
- **Depends on:** P11, P5 (on_submit/on_cancel), P9 (posting rule type). Until P9 is accepted, plans come from Go-registered builders used by tests only.
- **Touches:** `internal/posting` (plan type, validation, writer, Posting Log, mirror derivation), GL ledger definition, `athena_posting_log`, hooks `on_submit` and `on_cancel`, `proto/athena/v1/ledgers.proto` (read APIs only; no write RPC for ledgers), View mode ledger drill-down page for a voucher. OpenTelemetry: one span per posting with ledger names, row counts, totals as counts only (no amounts or party ids in attributes).
- **Work:** engine validates balanced per voucher and per currency view, leaf and open accounts, open period, mandatory dimensions; resolves rounding with an explicit rounding line; writes rows and Posting Log in one transaction with an idempotency key from voucher and version; cancel derives exact mirror rows flagged `is_reversal` with `reverses_id`; period lock with a logged override permission.
- **Verification:** common verification on both engines.
- **Invariant tests:** every voucher's GL balances in base and transaction currency (1, property-based over random plans); submit then cancel leaves every ledger exactly as before apart from the flagged mirror rows and nets to zero (2); retrying with the same key changes nothing, including concurrent retries (7); posting into a locked period fails without the override and succeeds with it only when logged (8); unbalanced plan is refused and nothing is written.

### P13. Party ledger, settlements, realised FX

- **Spec:** 6.2 (party ledger and Settlement), 3.4 (allocation), 10.2, invariant 5.
- **Depends on:** P12.
- **Touches:** party ledger definition, Settlement record kind, allocation using P3 `Allocate`, realised gain or loss at settlement through the posting engine, `internal/posting`, ledger read page in View.
- **Verification:** common verification on both engines.
- **Invariant tests:** per-invoice outstanding equals open-item sum; settlement allocations sum exactly to the payment; party ledger equals receivable/payable control totals in a fixture (5); foreign-currency settlement at a different rate posts exactly the expected gain or loss and still balances; cancel of a settlement restores open items.

### P14. Balances, query compiler, reports engine v1, Report Run snapshots

- **Spec:** 6.3 (balances, rebuild), 11.1 to 11.4, 11.6, 17.8 (declarative query), invariants 3 and 9.
- **Depends on:** P12, P13, P8 (permission-compiled queries), P7 (expressions), P2.
- **Touches:** `internal/query` (declarative query in Protobuf messages, compiled to SQL through the dialect; Starlark may post-process but never issues SQL), `internal/report`, Balance record kind for account balance and party balance (updated in the posting transaction, rebuildable), Report Run snapshot kind with `as_of`, `ledger_high_water`, `content_hash`, `proto/athena/v1/reports.proto` (`ReportService`), Reports surface in View (saved runs, shadcn table, parameters form, export CSV/JSON) and an authoring entry in Build per additional point D. Report definitions and the trial balance are shipped as test fixtures only in this phase; the product catalogue belongs to P23. OpenTelemetry: span per report run with report key, row count and duration only.
- **Verification:** common verification on both engines; browser: run a report from a test project, view the snapshot, reproduce it.
- **Invariant tests:** balances rebuilt from the ledger equal stored balances (3); a report reproduced with a stored high-water mark returns identical results after later backdated entries (9); drill-down from every figure reaches ledger rows and vouchers; report results are filtered by the same permission predicates as record reads.

### P15. Module packaging (OPEN design)

- **Spec:** 14, 19.1 to 19.4, 22, 23 (manifest, install, upgrade, uninstall, extension, overlay).
- **Exists:** `ModulesPage` placeholder; O-005 open; D-009 and O-002 (dashboard/codebase sync) open.
- **Depends on:** P2 (migrations), P4 to P9, P14 (what a pack can ship); decisions 1, 2, additional point A (vocabulary).
- **Touches:** new `internal/modules` (manifest, dependency resolution, install plan shown before running, hash verification, install state recorded and resumable, uninstall blocked while data exists), definition file format (YAML or JSON validated by Go; spec recommends data files, `gopkg.in/yaml.v3` is already a dependency), overlays, `proto/athena/v1/modules.proto`, `web/src/pages/ModulesPage.vue` (install plan preview, installed list), `athena pack build` style CLI commands in `cmd/athena`. Resolves D-009 and O-002 only with the user's answers.
- **Verification:** common verification on both engines; install, upgrade and uninstall a throwaway test pack.
- **Invariant tests:** install is atomic or resumable; uninstall refused while records hold data; upgrade re-applies overlays and reports conflicts instead of overwriting; extension fields are namespaced and removable; table names stay within limits on both engines.

### P16. Jobs, outbox, reposting framework

- **Spec:** 15.2 (durable job table with leasing, retries, backoff), 11.5, 12.1 (Job Run), 13.1 (Reaction, Scheduled rules), 10.12 (outbox).
- **Depends on:** P9, P12.
- **Touches:** `internal/jobs`, `athena_jobs`, `athena_outbox`, Job Run log, workers started by the single Go process, Reaction and Scheduled rule types in `internal/rules`, a Jobs view in Build or View (shadcn table).
- **Verification:** common verification on both engines.
- **Invariant tests:** a job is leased by one worker at a time; retry never duplicates an output (idempotency keys); resumable batch after crash; outbox delivery is at least once with dedupe key; every job writes a Job Run.

### P17. Stock ledger: valuation layers, bins

- **Spec:** 6.2 (stock ledger, valuation, FIFO and moving average from day one per decision 17.3), 6.3 (Stock Bin), 15.2 (serialise per item and warehouse), invariant 4 (setup).
- **Depends on:** P12, P16, P20 (UoM) for conversions or a minimal stock-UoM-only version first.
- **Touches:** stock ledger and valuation layer tables, bin balance, per `(item, warehouse)` row lock through the dialect, COGS and stock account posting via the posting engine, stock view in View.
- **Verification:** common verification on both engines.
- **Invariant tests:** FIFO and average cost match a hand-computed golden table; returns cost at original layer; negative stock policy blocks or allows as configured; bins rebuilt from the ledger equal stored bins; concurrent writes to one item-warehouse serialise.

### P18. Reservations, serial/batch, backdated repost, stock to GL check

- **Spec:** 6.2 (reservation, serial and batch ledgers), 6.4 step 5 (repost), 5.3 (backdated), 12.1 (Integrity and Incident Log), invariants 4 and 6.
- **Depends on:** P17, P16.
- **Touches:** reservation ledger, serial/batch ledgers, repost job (replays forward and logs every changed row), continuous stock to GL reconciliation check and Incident Log.
- **Invariant tests:** backdated entry followed by repost equals replaying the full history from scratch (6); stock ledger value equals GL stock accounts (4); available equals actual minus reserved; each repost row change is logged.

### P19. Remaining ledgers: tax, asset, budget, project, leave, meter, intercompany

- **Spec:** 6.2 (all remaining ledgers), 6.3 (balances), 10.8 (tax template mechanism only, country behaviour stays in packs).
- **Depends on:** P12 to P16.
- **Work:** one ledger definition each on the shared ledger foundation. This phase may be split into two reviews: (a) tax, budget, project; (b) asset, leave, meter, intercompany. Kernel only: no depreciation methods or payroll rules (Module work).
- **Invariant tests:** each ledger rejects direct writes; reversal mirrors exactly; leave balance is always a sum; intercompany pairs share `pair_id` and net to zero across companies; budget `available = budgeted - committed - actual`.

### P20. Trees, effective dating, account determination, UoM

- **Spec:** 10.5 (tree path and depth), 10.6 (effective dating), 8.1 (Determination master), 10.7 (UoM and conversions), 2.2 flags.
- **Depends on:** P4, P7, P12.
- **Touches:** record-type flags `tree` and `effective_dated` in definitions, `internal/store` path helpers (VARCHAR path and depth, usable on both engines), non-overlap constraint enforced in Go within the transaction, Determination resolution (most specific match) used by posting.
- **Invariant tests:** subtree queries correct after move and merge; postings only to leaves; overlapping effective ranges rejected; "as of" lookup returns the right version; determination picks the most specific match and falls back to company defaults.

### P21. Report engine complete: schedules, delivery, controls, Explain trail

- **Spec:** 11.5, 11.3 (controls), 12.2 (Explain trail), M9, invariants 9 and 10 at scale.
- **Depends on:** P14, P16, P18, P19.
- **Touches:** schedule engine aware of the company calendar, delivery channels (in-app first; email and webhook are optional integrations), conditional delivery, closing pack Snapshot with lock, control reports as definitions (unbalanced vouchers, stock versus GL), an Explain action on every figure in View.
- **Invariant tests:** a closing pack is locked and never silently regenerated; control reports return empty on a healthy ledger and non-empty on injected faults; Explain walks ledger row to voucher to version to rule run to request to user.

### P22. Integration surface, attachments, search, collaboration objects

- **Spec:** 10.10, 10.12, 15.1 (blob store, search), 18 checklist items. Cloud storage stays optional (D-006).
- **Depends on:** P8, P16.
- **Work:** CSV/JSON import and export with dry run through the same permission gate, webhooks via the outbox, content-addressed attachment store on local disk first, search through a dialect method (MariaDB FULLTEXT, PostgreSQL tsvector), comments, assignments, tags, shares as dynamic-link records. Print/PDF engine, i18n and mobile are listed in spec section 18 and are **not** scheduled; they need a decision before planning.
- **Invariant tests:** import dry run writes nothing; import and API enforce the same grants; attachment content hash verifies; search results respect row permissions.

### P23. Catalogue modules M3 to M10 (GATED by decision 1)

- **Spec:** sections 7, 8, 9 and milestones M3 (accounts pack and first reports) to M10 (people, manufacturing, meters), plus golden scenarios 16.3.
- **Status:** not planned in detail. Runs only if decision 1 is "also ship catalogue modules", and only after P15 (packaging) is accepted. Each milestone would be its own sub-phase: definitions and Starlark only. Spec rule: if a milestone after M4 needs a Go change, record the missing primitive, fix the kernel in a new phase, and redo the module test.
- **Invariant tests:** golden scenarios from spec 16.3; M5 passes with zero Go changes.

## 6. Cross-phase notes

- **Kernel invariant suite:** from P5 on, tests live in `internal/<pkg>/invariants_test.go` and share a fixture builder in `internal/testdb`. Fixtures define test-only Records inside the test database; no domain Records appear in the product binary.
- **Telemetry rule:** each phase that adds an operation adds a test using an in-memory span exporter that fails if any span attribute contains an entry value, amount or credential.
- **Reviewability:** if a phase's diff exceeds roughly 1,500 changed lines excluding generated code, split it before coding. P12, P14 and P15 are the likeliest to split.
- **Baseline problem:** about 57 uncommitted in-progress changes exist. Per-phase diffs will be mixed with them unless the user commits a baseline first (additional point C). Without it, each phase records the exact files it changed in its notes file, and the reviewer uses that list.
- **Vocabulary after P2a:** phase text written before P2a keeps the old words (Record definition, Entry, `EntryService`, project). Read it through the P2a mapping: Record definition is Record Type, Entry is Record, `EntryService` is `RecordService`, `RecordService` is `RecordTypeService`, project is Workspace, installable Module is Pack. Phase authors use the new words in step prompts. ADR numbers named in phases (for example the auth ADR in P8, now numbered ADR-0010) are placeholders; P2a takes ADR-0008 and ADR-0009, so later phases use the next free number.
- **Batching:** at the 50-step workflow limit, the first batch is P0 to P14 (about 15 phases of two steps each plus approval steps). P15 to P23 are authored after the first batch is approved.

- **Carried into P9 (before Starlark work):** the Users & Roles Build page (backend `ListUsers`, `CreateUser`, `ListRoles` exist in `auth.proto`; no web page) was not built in P8a.
## 7. Decisions needed from you

These are open. The roadmap does not answer them.

1. **Kernel only, or also catalogue modules?** The spec's movement catalogue (sales, procurement, inventory, manufacturing, assets, payroll) is Module work and your accepted scope (D-007) says the core ships no domain Records. "An ERP that functions as defined there" needs those modules to exist. Options: (a) kernel only, catalogue left to later installable packs; (b) kernel plus catalogue packs as separate, clearly installable packages kept outside the platform shell; (c) kernel plus a thin first pack (accounts, M3 only) to prove the kernel. This also decides whether P11 may ship Company, Currency and Fiscal Year as kernel-owned masters.
2. **Migration design and its position.** The roadmap puts versioned migrations at P2 because P3 to P5 add columns to populated tables. Do you accept a recorded, idempotent, resumable migration sequence with a state table (spec 20.3), and that position, or do you want it designed later (which blocks P4 and P5)?
3. **How is PostgreSQL parity tested?** Docker is installed but its daemon is not running, and `psql` is not installed. Options: start Docker and run a Postgres container, install Postgres with Homebrew, or point at an existing instance you have. Which one, and what connection do I use (never in the repo)?
4. **Starlark and expression engine, and timing.** Starlark library: `go.starlark.net` (the reference Go implementation) is my proposal. Expression language: restricted CEL via `cel-go` (spec recommendation) or a hand-written mini-language. Rules are placed at P9 and expressions at P7. Confirm the choices and whether to move either earlier or later.
5. **When do auth and permissions land?** The spec puts permissions in M1 and requires audit, role checks and period-lock overrides. The roadmap places auth at P8, after lifecycle, numbering and audit, and keeps everything loopback-only until then. Do you want it earlier (before P5), where it adds work to every later phase but never leaves the system unprotected, or as planned?
6. **Should any phase be skipped, merged or reordered?** For example: PostgreSQL parity earlier or later than P10, skipping P22, merging P3 into P4, or starting the catalogue before P19.

### Additional points raised by the survey (need an answer or an explicit "proceed with the default")

- **A. Vocabulary.** The spec renames (section 22) conflict with accepted D-005 and ADR-0004. Keep the current words (Record, Record entry, Module, project) as the default, or adopt Pack, Workspace, Record Type, Grant? Adopting them touches protobuf names, docs, UI copy and tables, so it should be done once, early, as its own phase.
- **B. Storage shapes.** Spec: integer minor units for money, ULID or bigint ids, table names `p_<pack>__<record_type>`. Repo: `DECIMAL(18,6)`, UUIDv7, `athena_record_<key>`. Default proposal: keep UUIDv7 and `athena_record_<key>`, use scaled integers for money inside new ledger tables, and keep `DECIMAL` decimal strings for user Number fields. Confirm or change.
- **C. Commit a baseline?** Committing the current in-progress work before P0 would give clean per-phase diffs. I will not commit unless you ask.
- **D. Where do Reports live in the desk?** Build today is Overview, Records, Modules, Rules. Proposal: add a Reports page to Build for authoring and show runs in View. This changes the AGENTS.md rule on Build pages, so it needs your approval.
- **E. Frontend tests.** Adding Vitest (P0) is a new dev dependency. Confirm.

## 8. User answers (approved 2026-11-18)

| Point | Answer | Effect on the plan |
| --- | --- | --- |
| 1 | Kernel first. P0 to P22 in scope. P23 stays gated and is not started without a new go-ahead. D-007 stands. | P11 keeps kernel-owned masters to company, currency, fiscal year and dimension only. |
| 2 | Versioned migrations at P2 approved. | Decision log marks the migration design **accepted** once P2 lands and is reviewed. |
| 3 | Do not test PostgreSQL now. Keep P1. P10 is limited to adapter code with status "implemented but untested". No parity claims, no Docker or psql setup. | P10 verification is build, vet and unit tests of the dialect SQL generation only. Every later phase runs invariants on MariaDB only. The "both engines" lines in P11 to P22 read "MariaDB; PostgreSQL untested". ADR-0005 and docs say "untested" explicitly. |
| 4 | `go.starlark.net` (P9) and `cel-go` (P7) with custom decimal, money and quantity types so no floats enter. Record both as ADRs. | ADR for each, status PROPOSED until reviewed. |
| 5 | P8: local users, argon2id, server-side DB sessions in HttpOnly SameSite cookies, CSRF protection, role permissions with permission levels and row conditions, enforced in Go on every ConnectRPC method. OIDC documented as a future option. Loopback-only until P8 lands. | P8 adds an interceptor that denies by default for all services, including those added earlier. |
| 6 | No phase skipped or merged. Order as drafted. | No dependency forces a change. P7 (CEL) already precedes P8 and P9. |
| A (revised) | YES rename to spec vocabulary (Pack, Workspace, Record Type, Record). Supersedes D-005. | New phase P2a after P2; mapping table and ADR in section 5; no mixed vocabulary; compat through P2 migrations. Phases after P2a use the new words. |
| B (revised) | I decide and record as an ADR (proposed, accepted on user approval): minor units, UUIDv7, naming convention. | P2a ADR. |
| C (revised) | YES, local baseline commit first, after a secret and cache scan. No push. | First step of P0. Later phases do not commit; they note changed files. |
| D | Add a Reports page to Build. | `AGENTS.md` Build list updated in P14. |
| E | Add Vitest. | P0. |

### Additional requirements (apply to every phase)

- **CLI parity (mandatory).** Every kernel capability that lands is also reachable as `athena <command>` (for example `athena migrate`, `athena user`, `athena post`, `athena report`, `athena record-types`, `athena records`, `athena files`), with help text, consistent flags (`--workspace`, `--addr`, `--json`; renamed from `--project` by P2a), and README and CLI help updated in the same phase. Existing `init`, `new`, `run`, `serve` behavior is unchanged. CLI commands call the same Go kernel packages as the ConnectRPC handlers; they do not add HTTP endpoints. A phase is incomplete without its CLI commands and a test for them.
- **Execution.** Per phase: an implementing agent, then a separate review agent that does not re-run suites, then a pause for user approval. Independent subtasks (proto, Go, web, docs) may run in parallel inside a phase only when they touch disjoint files; `make generate` always runs after proto edits and before Go or web code that uses them.
- OTel, ConnectRPC, shadcn-vue and minimal-style constraints from section 1 still apply.
