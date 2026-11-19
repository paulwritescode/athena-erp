package workspace

// agentsTemplate is workspace_repository.agents_template from
// docs/architecture/implementation.yaml.
const agentsTemplate = `# Working in this Athena workspace

Read athena.yaml for the workspace identity and supported Bare Metal version. Check the
workspace README for setup, environment variables and recovery commands. Keep those
instructions current when workspace behavior changes.

Author Record Types under record_types/. Keep stable IDs across label changes.
Put trusted Go changes in hooks.go. The publisher owns *_gen.go. Reference shared
Record Types from Pack manifests. Bind Starlark rules to declared events and field keys.

Use the pinned pnpm version from web/package.json with the committed lockfile.
Use shared theme and type tokens for Vue components. Build and View must apply
the same field definitions and permissions.

Use the workspace services for data access and the publisher for schema changes.
Keep credentials, Record data and uploads out of source and logs. Document changes
to schema, dependencies, permissions and migration behavior alongside the change.

Before handoff, identify the affected workflow and report the checks actually run.
`

// gitignoreTemplate is workspace_repository.gitignore_template from
// docs/architecture/implementation.yaml, written verbatim.
const gitignoreTemplate = `# Private environment files. Commit placeholder values in .env.example.
.env
.env.*
!.env.example

# Credentials supplied as files. Store them in this private workspace directory.
/secrets/

# Business data and recovery copies. Back these up separately from Git.
/storage/uploads/
/backups/

# Athena runtime state, publish staging, logs and compiled workspace binaries.
/.athena/
/logs/
/bin/

# Installed JS dependencies, local pnpm store and generated View output.
node_modules/
.pnpm-store/
/web/dist/
/web/.vite/
*.tsbuildinfo

# Local OS metadata.
.DS_Store
Thumbs.db

# Commit web/pnpm-lock.yaml, athena.lock, go.mod, go.sum, Record definitions,
# generated Record Type Go source, handwritten hooks, Packs, Rules and View source.
`

const envExampleTemplate = `# Optional workspace-local settings. Copy to .env.local, which Git ignores.
# The database server login normally lives in the home .env.local.
# Set it here only to override the home login for this workspace.
# ATHENA_SERVER_URL='athena_user:change-me@tcp(127.0.0.1:3306)/'
`

const readmeTemplate = `# {{name}}

An Athena workspace. Its identity is in athena.yaml and its data lives in the
MariaDB database ` + "`{{slug}}`" + ` on the home database server.

See AGENTS.md before changing this workspace.

## Run

From this directory:

` + "```sh" + `
athena run
` + "```" + `

From the home directory:

` + "```sh" + `
athena run --workspace {{slug}}
` + "```" + `

The desk opens at http://127.0.0.1:3000. Only one Athena workspace runs at a
time per user; a second launch reports the one already running.

## Environment

The home .env.local holds ATHENA_SERVER_URL, the database server login
without a database name. A workspace .env.local can override it. Both files are
ignored by Git. .env.example lists the variables.

## Backup and restore

Back up three things together: this directory (Git), the ` + "`{{slug}}`" + ` database
(for example with mariadb-dump --single-transaction {{slug}}) and storage/uploads/.
Restore the database into an empty database named ` + "`{{slug}}`" + `; Athena checks
that the database's workspace ID matches athena.yaml before it starts.
`
