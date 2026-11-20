# ADR-0018: Workspace root Administrator

- **Status:** Accepted 2026-11-20
- **Date:** 2026-11-20
- **Decision owner:** not assigned
- **Related decisions:** ADR-0014, ADR-0017, D-019

## Context

G1 (ADR-0017) removed every bypass: owners hold permissions only through
Roles, and the first owner arrives through a guarded command-line bootstrap.
Operating experience with that model raised two needs. First, every
Workspace needs an account that can always sign in and see everything, so
access can be recovered and audited no matter what the Roles say; a
demonstration showed a Platform Administrator seeing record forms with all
values masked, which is correct under the model but leaves nobody able to
inspect the data until someone grants access. Second, the product owner
wants that account created by Athena setup itself, not by an operator
command, with the database password chosen at setup as its initial
credential. Facts: users live per workspace database; the server connects
with the home's server login, so the configured database password is
available at startup; usernames are immutable once created (no rename or
delete operation). Assumption: one human operator performs setup and keeps
the database password.

## Options considered

1. Full-bypass root per workspace, as specified — always signs in with the
   right password, sees all rows and fields, cannot be locked out,
   suspended, or deleted. Advantages: recovery is always possible from the
   desk; the rule is simple to reason about and to test. Costs and risks: a
   compromise of this account is total; it reintroduces a bypass stronger
   than the pre-G1 owner shortcut, so every use must be audited and the
   account must be hard to lose (see the credential rule below).
2. All-powerful roles without a bypass — root holds every permission
   through Roles instead. Advantages: disable-able, auditable through the
   same paths, no special cases in the gate. Costs and risks: "always" is
   not absolute; a misconfigured Role or a lockout can still shut root
   out, which defeats the stated purpose.
3. Single installation-wide root across all workspaces. Advantages: one
   credential to guard. Costs and risks: users live per database, so this
   needs cross-database authentication, a major architecture change for no
   product gain; workspaces stop being independent.

## Decision

Accepted: option 1, one root per workspace, with first boot after migrations as the setup seam.

- Each workspace database gets one `administrator` user (display name
  `Administrator`) at setup. The username is reserved: creating or
  inviting it fails like any other validation error.
- The initial credential is the database server password configured at
  setup. When that password is empty or fails the password policy, setup
  generates a random credential and prints it once, prominently, instead
  of storing a weak root password. Afterwards the credential is ordinary:
  the root rotates it with change password, and database password rotation
  does not touch it.
- "Always" means authentication still applies but nothing else blocks:
  the right password always signs in (no lockout accrual or check, no
  disabled, suspended, or state checks); a wrong password still fails with
  the generic error. Suspension of the account is refused. The permission
  checks treat the account as holding everything: all system permissions,
  all actions, unrestricted scope, maximum field level.
- The account carries no assignments and takes no part in the last-owner
  guard; there is nothing to demote. The command-line bootstrap keeps
  working and ignores the account when counting owners, so it stays
  available as emergency recovery. Its actions are logged like any user's.
- Seeding is idempotent and also backfills upgraded workspaces on next
  boot. If the name is already taken by a non-root user, seeding logs an
  error and skips rather than claiming the account.

Accepted by the sponsor with the bypass as specified, including the
generate-and-print fallback for an unusable database password.

## Consequences

- Product behavior: every Workspace has a recoverable, inspectable
  superuser from setup. The Users & Roles desk needs no changes: the
  permission-driven UI opens fully for the account.
- Implementation: a reserved-username check, root branches in sign-in and
  principal evaluation, a suspension refusal, bootstrap exclusion, and a
  startup seeding step with the credential rule. No migration is needed
  (usernames are immutable, so the reserved name cannot drift).
- Operations: guard the database password at setup; after first sign-in,
  rotate the root credential with change password. Losing it means
  database surgery or the CLI bootstrap path.
- Security: the account is the highest-value credential in the system.
  Compromise is total by design; audit reliance increases accordingly.
  The bypass is confined to one reserved username per database.
- Future choices: multi-factor sign-in (open) should land on this account
  first. A Frappe-style Role permission matrix stays a separate,
  read-first deliverable on top of the Permission Explorer verdicts.

## Revisit when

Acceptance review of the access model, any hosted-use decision, or evidence
that per-workspace recovery is insufficient (which would reopen option 3).
