# Athena ERP

Athena is a reusable ERP platform core built with Go, ConnectRPC, MariaDB, and a Vue 3 + TypeScript + Vite control desk. It is not preconfigured for a particular industry. Record Types and Packs will be installed or authored on top of the core.

The admin desk has separate **Build** and **View** modes. Build contains Overview, Record Types, Packs, and Rules. A Record Type describes a database table; a Record is a row. A Pack bundles Record Types and their relationships into an installable capability. A Workspace is a database plus its installed Packs and data. Vocabulary follows [ADR-0008](docs/decisions/adr-0008-vocabulary.md) (Proposed).

## Documentation

- Start with [`docs/README.md`](docs/README.md).
- Read [`AGENTS.md`](AGENTS.md) for current decisions and contributor guidance.
- See the [Record Type and Pack model](docs/architecture/record-type-and-pack-model.md) for intended data mapping and code generation boundaries.

## Run

Requirements: Go 1.26+, Node.js 20.19+, pnpm 11.22.0, and MariaDB. `make run` requires a configured database.

```sh
make run
```

The Vue app is built into embedded assets, then one Go process serves the UI and API at <http://localhost:3000>. The server binds to loopback by default because database-backed authoring endpoints do not yet have authentication. The same-origin health endpoint is `GET /healthz`.

Copy `.env.example` to `.env.local` and set `ATHENA_DATABASE_URL` to a Go MySQL-driver DSN. `make run` loads this ignored, local-only file automatically:

```sh
ATHENA_DATABASE_URL='athena_app:change-me@tcp(127.0.0.1:3306)/athena_erp?parseTime=true' make run
```

Use a dedicated database and user for Athena. The password above is only an example; keep credentials in the ignored `.env.local`, never in a tracked file. On startup Athena applies pending system migrations, which create the metadata tables (`athena migrate status` lists them, `athena migrate up [--to N] [--dry-run]` applies them without starting the server; both take `--workspace slug` or fall back to `ATHENA_DATABASE_URL`, and `--json`; `--addr` does not apply because no server starts). Saving a Record Type creates its dedicated table (`p_workspace__<key>`) and adds its field columns. A Link field requires a saved target Record Type and creates a foreign key. Each save is recorded in `athena_record_type_operations` and checked against the Record Type's revision, so a save from a stale tab is refused. A save interrupted by a crash is finished at the next startup. Adding optional fields and renaming labels save directly. Removing a field, changing its type or link target, or adding a required field to a table with Records is planned first (`RecordTypeService.PlanRecordTypeSave`) and needs `confirm_data_loss` and fill-in values where the plan asks for them. Record Type migrations and rollback are not implemented. A saved Record Type can be deleted with `RecordTypeService.DeleteRecordType`, which drops its table and Records through the same journal and revision check. It is refused while other Record Types link to it, and needs `delete_records` when Records exist. The desk asks for confirmation first; an unsaved Record Type is just discarded. A database created before the vocabulary rename is upgraded by system migration 5 on the next start; back it up first with `mariadb-dump` because the rename cannot be rolled back. `make run` stops with an error if `ATHENA_DATABASE_URL` is unset or MariaDB is unreachable, so the served desk always has a database.

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

- The MariaDB user given to `athena init` must be able to create databases. Its login is saved as `ATHENA_SERVER_URL` in the home `.env.local` (owner-only). A home that still has `athena-workspace.yaml` from an earlier build has the file renamed to `athena-home.yaml` the first time a command finds it.
- The Workspace slug, folder and database name come from the name at creation and never change. Renaming the Workspace changes only `name` in `athena.yaml`.
- A new Workspace contains `athena.yaml`, `AGENTS.md`, `README.md`, `.gitignore`, `.env.example`, `record_types/` and `storage/uploads/`, and is a Git repository on `main` with no commits.
- `athena record-types list|show|save|delete` and `athena records list|lookup|create|delete|submit|cancel|amend|versions|activity|history` and `athena audit verify` use the same kernel code as the desk (`--workspace slug` or `ATHENA_DATABASE_URL`, `--json`; `--addr` does not apply because no server starts). `record-types save --file def.json` takes a JSON RecordType (kind, class, parent, fields and attributes), `--dry-run` prints the plan and `--confirm-data-loss` allows steps that delete values. `records submit|cancel|amend <record-type> <id> --revision N` run the document lifecycle of a submittable document; `versions` and `activity` print its snapshots and timeline. `records lookup <record-type> [text]` finds Records by display label as the desk pickers do, `records create --name` sets a manual document name where the Record Type allows it, `records history` prints a Record's Audit Log rows with old and new values, and `audit verify [--anchor seq:hash]` checks the Audit Log hash chain and prints its head (exit status is non-zero when it is broken). Run `athena help` for every flag.
- `athena migrate status` and `athena migrate up [--to N] [--dry-run]` show and apply system migrations for a Workspace (or for `ATHENA_DATABASE_URL`) without starting the server.
- `athena run` refuses a database whose recorded Workspace ID differs from `athena.yaml`.
- Only one Athena process runs at a time per user (`athena run`, `athena serve` and `make run`). A second launch reports the running Workspace and exits.

## Tests

```sh
make test                                   # Go tests, then Vitest
go test ./...                               # Go only; MariaDB tests skip when no test database is set
CI=true pnpm --dir web run test             # Vitest only
```

MariaDB tests read `ATHENA_TEST_DATABASE_URL`, a DSN for a user who can create and drop databases named `test_athena_*` (MariaDB grants this to every user by default). `make test` falls back to `ATHENA_DATABASE_URL` from `.env.local`. Each test creates its own empty database and drops it afterwards, so your Workspace data is never touched. The field-rule cases in `internal/server/testdata/field_rules.json` run in both Go and Vitest.

## Current limits

- Records can be listed, created, edited and deleted through `athena.v1.RecordService` and the View page. Each Record has a UUIDv7 `id` and a `revision`; a stale `expected_revision` returns `aborted`. Each change is appended to the hash-chained Audit Log (`athena_audit_log`) and shown on the Record page with its old and new values; every call carries one `X-Request-Id` that is stored on the Record and its audit rows. There is no authentication or filtering yet, so every change is attributed to "Local administrator".
- Pack composition, relationship/join-table management, and Pack installation are not implemented.
- Go source generation for dashboard-created Record Types and Starlark rule execution are accepted product directions but are not implemented.
- Authentication and authorization are not implemented. Do not expose database-backed authoring endpoints to an untrusted network.
- PostgreSQL support is planned for later; MariaDB is the only current database target.

## Repository layout

```text
cmd/athena/             CLI entry point (`athena init`, `new`, `run`, `serve`, `version`)
internal/workspace/      homes, Workspace creation, Workspace resolution and the run lock
internal/server/         Go HTTP/ConnectRPC server and MariaDB-backed Record Type schema
internal/store/          Dialect interface, the MariaDB implementation and the system migration runner; engine-specific SQL lives here
internal/webui/          embedded frontend assets
proto/athena/v1/         Protobuf API contract
gen/                    generated Go API code
web/                    Vue 3 + TypeScript + Vite and shadcn-vue components
docs/                   architecture and decision records
```

Useful commands: `make test`, `make web-build`, `make generate` (requires Buf), `make build`, and `make run`. Frontend dependencies are installed with the committed pnpm lockfile; npm and Yarn are not supported.
