# Athena ERP

Athena is a reusable ERP platform core built with Go, ConnectRPC, MariaDB, and a Vue 3 + TypeScript + Vite control desk. It is not preconfigured for a particular industry. Record Types and Packs will be installed or authored on top of the core.

The admin desk has separate **Build** and **View** modes. Build contains Overview, Record Types, Packs, Rules, Files, and Access (shown with `access:users` or `access:roles`). A Record Type describes a database table; a Record is a row. A Pack bundles Record Types and their relationships into an installable capability. A Workspace is a database plus its installed Packs and data. Vocabulary follows [ADR-0008](docs/decisions/adr-0008-vocabulary.md) (Proposed).

## Documentation

- Start with [`docs/README.md`](docs/README.md).
- Read [`AGENTS.md`](AGENTS.md) for current decisions and contributor guidance.
- See the [Record Type and Pack model](docs/architecture/record-type-and-pack-model.md) for intended data mapping and code generation boundaries.

## Run

Requirements: Go 1.26+, Node.js 20.19+, pnpm 11.22.0, and MariaDB. `make run` requires a configured database.

```sh
make run
```

The Vue app is built into embedded assets, then one Go process serves the UI and API at <http://localhost:3000>. The server binds to loopback by default: sign-in with invites and scoped Grants exist (ADR-0014 amended by ADR-0017, both Proposed) but are not yet reviewed for use beyond the local machine. The same-origin health endpoint is `GET /healthz`.

The desk opens on a sign-in page. The first boot creates the workspace root `administrator`, whose password is the database password from setup (a random one, printed once, when that password is unusable); sign in as `administrator`, rotate the credential with change password, then invite owners and users from the Users & Roles page. The command line keeps a guarded owner bootstrap as emergency recovery:

```sh
set -a; . ./.env.local; set +a
go run ./cmd/athena user invite admin --owner     # or: bin/athena user invite admin --owner --workspace <slug>
```

Copy `.env.example` to `.env.local` and set `ATHENA_DATABASE_URL` to a Go MySQL-driver DSN. `make run` loads this ignored, local-only file automatically:

```sh
ATHENA_DATABASE_URL='athena_app:change-me@tcp(127.0.0.1:3306)/athena_erp?parseTime=true' make run
```

Use a dedicated database and user for Athena. The password above is only an example; keep credentials in the ignored `.env.local`, never in a tracked file. On startup Athena applies pending system migrations, which create the metadata tables (`athena migrate status` lists them, `athena migrate up [--to N] [--dry-run]` applies them without starting the server; both take `--workspace slug` or fall back to `ATHENA_DATABASE_URL`, and `--json`; `--addr` does not apply because no server starts). Saving a Record Type creates its dedicated table (`p_workspace__<key>`) and adds its field columns. A Link field requires a saved target Record Type and creates a foreign key. Each save is recorded in `athena_record_type_operations` and checked against the Record Type's revision, so a save from a stale tab is refused. A save interrupted by a crash is finished at the next startup. Adding optional fields and renaming labels save directly. Removing a field, changing its type or link target, or adding a required field to a table with Records is planned first (`RecordTypeService.PlanRecordTypeSave`) and needs `confirm_data_loss` and fill-in values where the plan asks for them. Record Type migrations and rollback are not implemented. A saved Record Type can be deleted with `RecordTypeService.DeleteRecordType`, which drops its table and Records through the same journal and revision check. It is refused while other Record Types link to it, and needs `delete_records` when Records exist. The desk asks for confirmation first; an unsaved Record Type is just discarded. A database created before the vocabulary rename is upgraded by system migration 5 on the next start; back it up first with `mariadb-dump` because the rename cannot be rolled back. `make run` stops with an error if `ATHENA_DATABASE_URL` is unset or MariaDB is unreachable, so the served desk always has a database. `make run` also gives the Files page a directory: `ATHENA_FILES_DIR`, default `.athena/files` (ignored by git). `athena serve --files-dir dir` does the same; without it the Files page reports that no directory is configured. When telemetry is on but the collector is down, browser spans posted to `/otlp/v1/traces` are dropped with 204 and the server logs the failure once a minute.

To stop the running server, or stop it and start again after a change:

```sh
make stop
make restart
```

`make stop` stops only an Athena process listening on the address. If another program holds the port, it reports that program and exits.

To use a different address:

```sh
make run ADDR=127.0.0.1:3001
```

Build the single-server executable:

```sh
make build
./bin/athena serve --addr 127.0.0.1:3000
```

## Traces and metrics (optional)

Athena exports OpenTelemetry traces and metrics when `OTEL_EXPORTER_OTLP_ENDPOINT` is set, and does nothing extra when it is not.

```sh
make telemetry        # starts grafana/otel-lgtm in Docker; Grafana at http://127.0.0.1:3001
# add OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 to .env.local
make restart
```

- The server traces every ConnectRPC call (`otelconnect`) and every MariaDB statement (`otelsql`), and exports request and connection-pool metrics. SQL text is recorded; parameter values (Record data) are not.
- The desk traces page loads and each API call, and sends its spans to `/otlp/v1/traces` on the Athena server, which forwards them to the collector. Browser and server spans for one action share a trace.
- In Grafana, open Explore and choose Tempo to search traces (service `athena` or `athena-desk`), or Prometheus for metrics. `make telemetry-stop` stops the container.

## Homes and Workspaces

`make run` above is the development path for this repository. To use Athena as an installed tool, build `bin/athena` and create a home. The home keeps the database server login; each Workspace gets its own folder and its own database.

```sh
make build
./bin/athena init ~/athena --user <mariadb-user>   # prompts for the password, or set ATHENA_SERVER_PASSWORD
cd ~/athena
../path/to/bin/athena new "Acme Trading"            # creates ./acme_trading and the database acme_trading
cd acme_trading && athena run                       # or from the home: athena run --workspace acme_trading
```

- The MariaDB user given to `athena init` must be able to create databases. Its login is saved as `ATHENA_SERVER_URL` in the home `.env.local` (owner-only). A home that still has `athena-workspace.yaml` from an earlier build has the file renamed to `athena-home.yaml` the first time a command finds it. `athena new --driver` must match the home driver; `postgres` is implemented but untested ([ADR-0019](docs/decisions/adr-0019-postgresql-adapter.md), Proposed), claims no parity, and a postgres workspace does not boot yet.
- The Workspace slug, folder and database name come from the name at creation and never change. Renaming the Workspace changes only `name` in `athena.yaml`.
- A new Workspace contains `athena.yaml`, `AGENTS.md`, `README.md`, `.gitignore`, `.env.example`, `record_types/` and `storage/uploads/`, and is a Git repository on `main` with no commits.
- `athena record-types list|show|save|delete` and `athena records list|lookup|create|delete|submit|cancel|amend|versions|activity|history`, `athena expressions check|eval|preview` and `athena audit verify|anchor` use the same kernel code as the desk (`--workspace slug` or `ATHENA_DATABASE_URL`, `--json`; `--addr` does not apply because no server starts). `record-types save --file def.json` takes a JSON RecordType (kind, class, parent, fields, attributes and form layout), `--dry-run` prints the plan and `--confirm-data-loss` allows steps that delete values. `records create --values` takes image values as `{"path":...,"hash":...,"alt_text":...}` objects. `records submit|cancel|amend <record-type> <id> --revision N` run the document lifecycle of a submittable document; `versions` and `activity` print its snapshots and timeline. `records lookup <record-type> [text]` finds Records by display label as the desk pickers do, `records create --name` sets a manual document name where the Record Type allows it, `records history` prints a Record's Audit Log rows with old and new values, `records list --filter 'doc.total > dec("100")'` keeps the Records for which an expression is true, `expressions check <record-type> <expression>` type-checks one (and says whether a filter can run in the database), `expressions eval ... --record id` evaluates it for a Record, `expressions preview <record-type> --values '{...}'` calculates formulas and field states for unsaved values, and `audit verify [--anchor seq:hash]` checks the Audit Log hash chain and prints its head (exit status is non-zero when it is broken). Run `athena help` for every flag.
- `athena notifications list [--unread] [--limit 30] [--page token]`, `athena notifications read <id>...` and `athena notifications read-all` operate on the administrator inbox by default. Pass `--user name` to select another account. The signed-in RPCs always act on the caller's own inbox.
- `athena workspace appearance show` reads the Workspace colors. Use `athena workspace appearance set --preset ink` (the default), `--preset red`, or `athena workspace appearance set --accent 'oklch(0.42 0.13 22)' --chart1 'oklch(0.62 0.09 55)'`. Custom charts use flags `--chart1` through `--chart5`. Both commands support `--workspace slug`. Appearance reads are public over RPC. Changes require `settings:appearance`. [ADR-0024](docs/decisions/adr-0024-notifications.md)
- `athena ledger company|period|currency|account|determination|setting` manages the kernel's ledger masters (`--workspace slug` or `ATHENA_DATABASE_URL`, `--json`): companies, periods with soft/hard close and reopen, currencies with immutable exchange rates, account types and tree accounts, determination mappings with specificity resolution, and Settings with draft generations, prospective publish and diff preview. The desk has the same surface on the Ledger setup page with a readiness view.
- `athena user invite|list|set|resend|reset|suspend|reactivate|sessions`, `athena role list|show|save|delete|assign|revoke|explain` and `athena access-log` manage sign-in (`--workspace slug` or `ATHENA_DATABASE_URL`, `--json`). Users are invite-only: `user invite` prints a one-time challenge, shown once, and the user chooses their own password with it; administrators never set passwords for others. `user invite <name> --owner` creates an instance owner, which only the command line can do; owners hold permissions through the Instance Owner Role, like everyone else. `user set <name> --roles a,b` replaces a user's Roles, `--disable` ends their sessions, `--unlock` clears a lockout after five failed sign-ins. `role assign <user> <role> [--scope company=acme] [--from date] [--to date]` gives one Role with its own Scope and dates; `role explain` shows which Assignment allows an action or why each one fails. `role save --file role.json` takes a JSON Role such as `{"name":"Clerk","systemPermissions":["build"],"grants":[{"recordTypeKey":"invoice","actions":["read","create","write"],"permissionLevel":1,"condition":"doc.owner == user.name"}]}`. Whole matching Assignments union and nothing is allowed without one. `access-log [--event denied]` prints sign-ins, failures, lockouts, denials and access changes. The command line acts as the operator: it applies no Grants and records changes as "Local administrator".
- `athena security show` lists Workspace session and rate settings. For example, `athena security set security.session.idle_minutes 45 --workspace demo` changes the idle timeout for new everyday sessions. `athena user sessions alice --workspace demo` shows device, last seen and absolute expiry. The Desk reads them through `AuthService.GetSecuritySettings` (`access:sessions` or `settings:security`) and changes them through `AuthService.SetSecuritySettings` (`settings:security` plus fresh step-up proof); every change is audited. See the [policy settings and bounds](docs/decisions/adr-0026-session-policy.md).
- `athena migrate status` and `athena migrate up [--to N] [--dry-run]` show and apply system migrations for a Workspace (or for `ATHENA_DATABASE_URL`) without starting the server.
- `athena run` refuses a database whose recorded Workspace ID differs from `athena.yaml`.
- Only one Athena process runs at a time per user (`athena run`, `athena serve` and `make run`). A second launch reports the running Workspace and exits.

- `athena files list|get|put|put-image|mv|rm|history` manages the Workspace files (`record_types/`, `rules/`, `packs/`, `generated/`, `images/`) with the same checks as the Files page (`--workspace slug`, `--json`; no `--addr`, because no server starts). Paths are relative and lowercase, files are UTF-8 text up to 1 MiB, and Record Type files are validated with line numbers. `put <path> --file local.yaml` replaces a file (`--new` refuses an existing one, `--expected-hash h` refuses a changed one), `get <path> [--revision N] [--out file]` reads it, `mv <from> <to>` renames, `rm <path> --yes` deletes it, and `history <path>` lists its versions. `put-image images/logo.png --file local.png` uploads a PNG, JPEG or WebP image (at most 5 MiB, metadata stripped) with the same guards as `put`. `list` shows which file backs which Record Type, Rule or Report. Every change keeps a version and an Audit Log row with path, hash and actor, never content. Over the API the `files:read` and `files:write` system permissions are required; give them to a Role with `"systemPermissions":["files:read","files:write"]`.
- Sign-in errors: the desk tells apart no users yet, wrong credentials, a locked or disabled account (only after the right password, so names cannot be probed; `athena user set <name> --unlock` clears a lock), an ended session, an unreachable server and a database that is not connected. `AuthService.Login` answers Unauthenticated for wrong credentials, FailedPrecondition with an `AuthFailure` detail for locked or disabled, PermissionDenied with reason `csrf` for a bad CSRF token and Unavailable when the database cannot be reached.
- `athena rules list|show|save|rm|versions|simulate|runs` manages Starlark Rules with the same code as the Rules page (`--workspace slug`, `--json`; no `--addr`, because no server starts). A Rule is a Starlark program of type validation (events `validate`, `before_submit`), derivation (`validate`), guard (`before_submit`, `before_cancel`, `on_trash`) or transform (called, no event) on a document Record Type. `save <key> --file rule.star --type validation --record-type invoice --event validate` creates a version (flags left out keep their value on an existing Rule; `--expected-version n` guards against a changed Rule), `simulate <key> [--file draft.star] [--record-id id] [--value field=text]...` is a rolled-back dry run that prints the outcome and a diff, `runs [--rule key] [--outcome rejected]` prints the Rule Run log, and `rm <key> --yes` deletes a Rule (versions and runs stay). Rules run inside the save as the acting user, through Go host functions only, with no files, network, clock or floats; they stop at 200,000 steps or 500 ms. Over the API the `rules:read` and `rules:write` system permissions are required. Starlark Rules are not production-ready (ADR-0016, Proposed).

## Tests

```sh
make test                                   # Go tests, then Vitest
go test ./...                               # Go only; MariaDB tests skip when no test database is set
CI=true pnpm --dir web run test             # Vitest only
ATHENA_BRUNO_PASSWORD='...' make bruno-test # Bruno black-box API checks; Athena must be running
```

The Bruno collection lives in [`bruno/athena-api`](bruno/athena-api), with
instructions in [`docs/testing/bruno.md`](docs/testing/bruno.md). It tests the
running ConnectRPC boundary, cookie/CSRF session flow, permission gate and
audit verification without changing business data. Bruno's desktop app and
CLI are separate; install the CLI once with `pnpm add --global @usebruno/cli`.

MariaDB tests read `ATHENA_TEST_DATABASE_URL`, a DSN for a user who can create and drop databases named `test_athena_*` (MariaDB grants this to every user by default). `make test` falls back to `ATHENA_DATABASE_URL` from `.env.local`. Each test creates its own empty database and drops it afterwards, so your Workspace data is never touched. MariaDB is the only tested database; PostgreSQL is implemented but untested ([ADR-0019](docs/decisions/adr-0019-postgresql-adapter.md), Proposed). The field-rule cases in `internal/server/testdata/field_rules.json` run in both Go and Vitest.

## Current limits

- Records can be listed, created, edited and deleted through `athena.v1.RecordService` and the View page. Each Record has a UUIDv7 `id` and a `revision`; a stale `expected_revision` returns `aborted`. Each change is appended to the hash-chained Audit Log (`athena_audit_log`) and shown on the Record page with its old and new values; every call carries one `X-Request-Id` that is stored on the Record and its audit rows. Changes made in the desk are attributed to the signed-in user; changes made with the command line to "Local administrator".
- Pack composition, relationship/join-table management, and Pack installation are not implemented.
- Go source generation for dashboard-created Record Types is an accepted product direction but is not implemented. Where generated source goes is proposed in ADR-0015. Starlark Rules run (ADR-0016, Proposed) but are not production-ready: they have step and time limits and no memory limit. The Files page stores `rules/*.star` text; the Rule that runs is the one saved through the Rules page or `athena rules save`.
- Local sign-in with invite-only users, single-use challenges, scoped dated Role assignments, granular access permissions and a transactional last-owner guard are implemented (ADR-0014 amended by ADR-0017, both Proposed), with a Users & Roles desk page and first-login completion. Owners hold no permission outside their Roles; each workspace has one root `administrator` from setup that bypasses all gates (ADR-0018, Accepted). OIDC, multi-factor sign-in and TLS are not implemented. Keep the server on loopback; do not expose it to an untrusted network.
- PostgreSQL support is planned for later; MariaDB is the only current database target.

## Repository layout

```text
cmd/athena/             CLI entry point (`athena init`, `new`, `run`, `serve`, `version`)
internal/workspace/      homes, Workspace creation, Workspace resolution and the run lock
internal/server/         Go HTTP/ConnectRPC server, the permission gate and MariaDB-backed Record Type schema
internal/auth/           users, Argon2id passwords, sessions, Roles, Grants and the Access Log
internal/files/          Workspace file paths, validation, atomic writes, versions and audit entries
internal/store/          Dialect interface, the MariaDB implementation and the system migration runner; engine-specific SQL lives here
internal/webui/          embedded frontend assets
proto/athena/v1/         Protobuf API contract
gen/                    generated Go API code
web/                    Vue 3 + TypeScript + Vite and shadcn-vue components
docs/                   architecture and decision records
```

Useful commands: `make test`, `make web-build`, `make generate` (requires Buf), `make build`, and `make run`. Frontend dependencies are installed with the committed pnpm lockfile; npm and Yarn are not supported.
