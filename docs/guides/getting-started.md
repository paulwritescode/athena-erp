# Getting started with Athena

This guide takes a new workspace from nothing to signed in: build the
program, create a home, create a workspace, run it, sign in as the
workspace root, and invite the first owner. Every command below ran
verbatim against the repository during testing (B11 slice 1); the same
path runs unattended as `scripts/e2e-install.sh`.

Athena keeps one home per user. The home holds the database server login;
each workspace is a folder in the home with its own database. Only one
Athena server runs at a time per user.

## What you need

- Go 1.26 or newer, Node.js 20.19 or newer, and pnpm 11.22.0.
- MariaDB with a user that can create databases. PostgreSQL is
  implemented but untested and claims no parity; use MariaDB.

## 1. Build the program

From the repository root:

```sh
make build
```

This installs the frontend dependencies from the committed lockfile,
builds the desk, and writes the single executable to `bin/athena`.
Check it answers:

```sh
./bin/athena version
```

Put `bin/athena` on your `PATH`, or keep using the `./bin/athena` path
in the commands below.

## 2. Create a home

A home is a directory that keeps the database server login. Create it
once:

```sh
athena init ~/athena --user <mariadb-user>
```

The password is read from `ATHENA_SERVER_PASSWORD` or asked for
interactively:

```sh
ATHENA_SERVER_PASSWORD='...' athena init ~/athena --host 127.0.0.1 --port 3306 --user <mariadb-user>
```

`init` checks the login against the server before writing anything. On
success it prints `Home ready at ...` and writes `athena-home.yaml`,
an owner-only `.env.local` holding `ATHENA_SERVER_URL`, and a
`.gitignore` for it. Keep `.env.local` private; it is the keys to
every workspace database in the home.

If the server cannot be reached, `init` says so and writes nothing:

```text
athena: cannot connect to MariaDB at 127.0.0.1:3306 as <user>: dial tcp ...: connect: connection refused
```

Running `init` twice in the same directory is refused with
`<dir> already contains a home`.

## 3. Create a workspace

Move into the home and create a workspace. The name becomes the folder,
the database, and the login slug, in lowercase with underscores; it
never changes afterwards:

```sh
cd ~/athena
athena new "Acme Trading"
```

On success `new` prints where the workspace lives and its database
name:

```text
Created Acme Trading in /Users/you/athena/acme_trading
Database: acme_trading
Git repository initialized on main. Commit and add a remote when ready.
Run it with:
  cd /Users/you/athena/acme_trading && athena run
```

A new workspace contains `athena.yaml`, `AGENTS.md`, `README.md`,
`.gitignore`, `.env.example`, `record_types/` and `storage/uploads/`,
and is a Git repository on `main` with no commits.

Useful variants, all equivalent:

```sh
athena new "Acme Trading" --logo ./logo.png   # set the light-theme logo at creation
athena new --logo ./logo.png "Acme Trading"   # flags work before the name too
athena new                                    # asks for the name interactively
```

The logo is a PNG or SVG file of at most 512 KB. A missing file fails
fast, before any prompt: `read ./logo.png: no such file or directory`.

Plain-language errors and their fixes:

| What you see | What it means |
|---|---|
| `no Athena home found here or in any parent directory; run athena init first` | Run `new` from inside a home. |
| `<dir> already exists; choose another name` | A workspace with that slug exists; pick another name. |
| `workspace slug "test" is reserved (from name "Test")` | That slug is a system name; pick another name. |
| `workspace slug "9_lives" must start with a letter and use up to 48 lowercase letters, digits or underscores (from name "9 lives")` | Start the name with a letter. |
| `athena new: --driver "postgres" does not match the home driver "mariadb"` | The flag must match the home; homes are MariaDB unless converted. |

Renaming a workspace later changes only `name` in `athena.yaml`; the
slug, folder and database stay as created.

## 4. Run the workspace

From the workspace directory:

```sh
cd ~/athena/acme_trading
athena run
```

Or from the home with the slug:

```sh
cd ~/athena
athena run --workspace acme_trading
```

The server listens on `http://127.0.0.1:3000` by default
(`--addr 127.0.0.1:3001` moves it) and stays on loopback: sign-in is
local only, with no TLS, and the server must not face an untrusted
network. The first start applies the system migrations and creates the
workspace root account (step 5), then prints
`Athena ERP listening at http://127.0.0.1:3000`.

`run` takes no positional arguments: `athena run acme_trading` is
refused with `unexpected argument "acme_trading"` — pass
`--workspace acme_trading` instead. Running from the home without the
flag is refused with `this is the home, not a workspace; run from a
workspace directory or pass --workspace <slug>`.

Only one Athena server runs per user. A second `run` (or `serve`)
reports the running workspace and exits:

```text
athena: Athena is already running Acme Trading (acme_trading) at http://127.0.0.1:3000 (pid 1234); stop it first
```

`run` also refuses a database whose recorded workspace id differs from
`athena.yaml`, so a workspace never starts against the wrong database.

## 5. Sign in as the workspace root

Open <http://localhost:3000>. The desk opens on a sign-in page. Sign in
as `administrator`:

- When the database password is usable (12 to 256 characters and not
  the username), it is the root password. Nothing is printed about it.
- When it is not usable, the first start prints a generated password
  once in the server log and never again:
  `Administrator account created with a generated password, shown once: ...`

The root account bypasses all permission checks, always signs in, and
cannot be suspended; every other account goes through invites and
Roles. Change the root password right after signing in: open the user
menu in the top bar and choose **Change password**. Changing the
database password later does not change the root password.

## 6. Invite the first owner

The command line is the only place that can create an instance owner.
Owners hold permissions through the Instance Owner Role, like everyone
else; the root account from step 5 stays the emergency account.

```sh
athena user invite alice --owner
```

This prints a one-time invite challenge, shown once. Alice chooses her
own password with it at her first sign-in; administrators never set
passwords for others. Further users and Roles are managed from the
desk's Access page or with `athena user` and `athena role`
(`athena help` lists every flag).

## CLI and desk equivalents

Steps 2 to 4 happen before any desk can run, so they are command-line
only. From step 5 on, each side has its match:

| Step | Command line | Desk |
|---|---|---|
| Create the home | `athena init` | — (the desk needs a running workspace) |
| Create a workspace | `athena new "Name" [--logo]` | — (workspaces are created from the command line) |
| Run the workspace | `athena run` | — (this starts the desk) |
| Workspace name and logos | `athena workspace identity show` / `set --name ... --logo-light ... --logo-dark ...` | Build mode → Settings → Workspace identity |
| Sign in as root | `athena user list` only inspects; sign-in is interactive | The sign-in page |
| Change the root password | — (administrators never set passwords, not even from the command line) | The user menu → Change password |
| Invite the first owner | `athena user invite <name> --owner` | Build mode → Access (after the first owner exists) |

A guided Setup wizard (company, logo, currency, period, first owner)
and a readiness view are planned next; the desk's Setup page currently
holds their place with an empty state.

## Checking your own setup

`scripts/e2e-install.sh` replays this guide against throwaway homes,
workspaces, databases and loopback ports, then cleans up:

```sh
make build && ./scripts/e2e-install.sh
```

It reads the database server login from `ATHENA_TEST_DATABASE_URL`,
falling back to `ATHENA_DATABASE_URL` in `.env.local`, exactly like
`make test`, and never touches real homes or databases.
