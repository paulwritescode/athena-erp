# Athena ERP

Athena is a reusable ERP platform core built with Go, ConnectRPC, MariaDB, and a Vue 3 + TypeScript + Vite control desk. It is not preconfigured for a particular industry. Records and Modules will be installed or authored on top of the core.

The admin desk has separate **Build** and **View** modes. Build contains Overview, Records, Modules, and Rules. A Record definition describes a database table; a Record entry is a row. Modules compose Records and their relationships into reusable capabilities.

## Documentation

- Start with [`docs/README.md`](docs/README.md).
- Read [`AGENTS.md`](AGENTS.md) for current decisions and contributor guidance.
- See the [Record and Module model](docs/architecture/record-and-module-model.md) for intended data mapping and code generation boundaries.

## Run

Requirements: Go 1.26+, Node.js 20.19+, and npm. MariaDB is optional while inspecting the UI and required to save Record definitions and provision their tables.

```sh
make run
```

The Vue app is built into embedded assets, then one Go process serves the UI and API at <http://localhost:3000>. The same-origin health endpoint is `GET /healthz`.

Set `ATHENA_DATABASE_URL` to a Go MySQL-driver DSN to enable MariaDB:

```sh
ATHENA_DATABASE_URL='athena:change-me@tcp(127.0.0.1:3306)/athena?parseTime=true' make run
```

Create the database and dedicated local user in MariaDB first. The password above is only an example; supply credentials through the environment, never a tracked file. On startup Athena creates metadata tables. Saving a Record definition creates its dedicated table and adds its field columns. A Link field requires a target Record table and creates a foreign key.

To use a different address:

```sh
make run ARGS='--addr :3001'
```

Build the single-server executable:

```sh
make build
./bin/athena serve --addr :3000
```

## Current limits

- Record-entry CRUD is not implemented: table provisioning does not yet save business rows.
- Module composition, relationship/join-table management, and package installation are not implemented.
- Go source generation for dashboard-created Records and Starlark rule execution are accepted product directions but are not implemented.
- Authentication and authorization are not implemented. Do not expose database-backed authoring endpoints to an untrusted network.
- PostgreSQL support is planned for later; MariaDB is the only current database target.

## Project layout

```text
cmd/athena/             CLI entry point (`athena serve`)
internal/server/         Go HTTP/ConnectRPC server and MariaDB-backed Record schema
internal/webui/          embedded frontend assets
proto/athena/v1/         Protobuf API contract
gen/                    generated Go API code
web/                    Vue 3 + TypeScript + Vite and shadcn-vue components
docs/                   architecture and decision records
```

Useful commands: `make web-build`, `make generate` (requires Buf), `make build`, and `make run`.
