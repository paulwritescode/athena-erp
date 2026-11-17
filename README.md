# Athena ERP

Athena is an early ERP platform foundation built around Go, Starlark, Vue 3, TypeScript, Vite, and ConnectRPC. **Athena is a working name; the public product name is not finalized.**

One Go server serves the embedded Vue application and a typed ConnectRPC API from the same address. The admin UI has distinct **Build** and **View** modes: Build contains Overview, Forms, Modules, and Rules; View renders a selected Form as an end-user record screen. MariaDB metadata persistence is optional until configured and currently saves Form definitions; record data, module installation, and executable Starlark rules are not implemented yet.

## Documentation

- Start at [`docs/README.md`](docs/README.md) for the documentation map.
- Read [`AGENTS.md`](AGENTS.md) before making project changes. It summarizes the current accepted decisions and the documentation workflow for keeping them current.
- Architecture and decision records live under `docs/architecture/` and `docs/decisions/`.

## Requirements

- Go 1.26 or newer
- Node.js 20.19 or newer and npm

## Run the app

From the repository root:

```sh
make run
```

This builds the Vue app into embedded assets, then runs the Go server at <http://localhost:3000>. The UI and ConnectRPC API share the same Go process and origin. Set `ATHENA_DATABASE_URL` to a MariaDB DSN to enable persistent Form definitions and metadata tables.

For a local Homebrew MariaDB install, start the database service and create a database and user with access to it. Then run Athena with the DSN for that account:

```sh
brew services start mariadb@11.8
mariadb -u root -e "CREATE DATABASE athena CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE USER 'athena'@'127.0.0.1' IDENTIFIED BY 'change-me'; GRANT ALL PRIVILEGES ON athena.* TO 'athena'@'127.0.0.1';"
ATHENA_DATABASE_URL='athena:change-me@tcp(127.0.0.1:3306)/athena?parseTime=true' make run
```

This is a development-only account example; use a strong secret and a restricted account outside local development. The server creates the initial Forms, Modules, and Rules metadata tables when a database is configured. Do not put credentials in tracked files.

To choose another address:

```sh
make run ARGS='--addr :3001'
```

To build a standalone executable:

```sh
make build
./bin/athena serve --addr :3000
```

The generated web assets are embedded in the executable. Node and npm are only needed to build the app.

## Project layout

```text
cmd/athena/             CLI entry point (`athena serve`)
internal/server/         HTTP server and ConnectRPC service implementation
internal/webui/          embedded frontend assets
proto/athena/v1/         protobuf API contract
gen/                    generated Go API code
web/                    Vue 3 + TypeScript + Vite app and generated TS clients
docs/                   architecture notes, decision records, and open questions
AGENTS.md               contributor guidance and current decision summary
```

## Useful commands

```sh
make generate            # regenerate Go and TypeScript RPC code (requires Buf on PATH)
make web-build           # compile Vue into embedded static assets
make build               # build ./bin/athena
make run                 # generate, build the UI, and run one Go server
```

The initial RPC is `athena.v1.SystemService/GetSystemInfo`, which the UI calls to confirm that it is talking to the Go server. MariaDB integration uses `ATHENA_DATABASE_URL`; without it, the UI remains available with sample definitions held only in browser memory. Authentication and record persistence are not implemented.
