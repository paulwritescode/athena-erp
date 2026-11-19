# ADR-0004: One database per project on a workspace database server

- **Status:** Accepted
- **Date:** 2026-11-17
- **Decision owner:** Project owner (requested in the 2026-11-17 working session)
- **Related decisions:** ADR-0002, ADR-0003, O-006

## Context

Athena Bare Metal is installed once per user. The project owner asked that the database server is chosen when Athena is set up, and that each new project gets its own database, named from the project name, holding all of that project's tables. The implementation plan (`docs/architecture/implementation.yaml`) already called for a database per project, a stable slug, a project UUID checked at startup, and one active local project per user.

## Options considered

1. **One database per project (chosen).** Clear isolation and simple backup and restore per project. Needs a server login that can create databases.
2. **One shared database with a table prefix per project.** Needs no extra privileges, but mixes projects' data and makes restore and access control per project harder.
3. **A separate server per project.** Strongest isolation; too heavy for local use.

## Decision

- `athena init` creates a workspace directory with `athena-workspace.yaml` (driver `mariadb`) and saves the database server login, without a database name, as `ATHENA_SERVER_URL` in the workspace `.env.local` (mode 0600).
- `athena new "Name"` derives a lowercase snake_case slug, creates the project directory `<workspace>/<slug>` and the database `<slug>`, and records the project UUIDv7 in `<slug>.athena_project`. The slug, directory and database name are fixed at creation; renaming the project changes only `name` in `athena.yaml`.
- Creation stages files in `<workspace>/.<slug>.creating`, then creates the database, then renames the staging directory and runs `git init` on `main`. A retry reuses the staged ID and files. An existing project directory, or a database with tables but no matching project ID, is refused.
- `athena run` resolves one project, refuses to serve a database whose `athena_project` ID differs from `athena.yaml`, and holds a per-user OS lock (`flock` on `<user config dir>/athena/active.lock`) for its lifetime. `athena serve` takes the same lock.

## Consequences

- The workspace login needs `CREATE` on the databases it will create. Per-project database users are not created yet; every project in a workspace uses the workspace login.
- Slugs avoid MariaDB system schema names and `test`/`athena`, and are at most 48 characters.
- PostgreSQL is not supported by `athena init`; the driver field leaves room for it.
- `athena serve` with `ATHENA_DATABASE_URL` remains as the development path for this repository and has no project identity.

## Verification

On 2026-11-18, against local MariaDB 11.8 with a login limited to `zz_%` databases: created a workspace and two projects with distinct databases and IDs; ignore rules kept secrets, uploads, backups, runtime and build output out of Git while tracking `.env.example`, `athena.yaml`, Record definitions and lockfiles; a second launch reported the running project; a display-name change kept entries; a mismatched project ID and a populated database were refused; an interrupted creation resumed with the same ID and kept an edited `AGENTS.md`.

## Revisit when

- Live deployments need their own credentials (the plan's separate live database), or a hosted environment cannot grant `CREATE`.
- The PostgreSQL adapter is implemented.
