# ADR-0017: Access model v2 (invites, scoped assignments, no owner shortcut)

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0014 (amended), ADR-0008 (vocabulary), ADR-0010 (Record kinds), ADR-0013 (row conditions), O-008, `.kiro/specs/athena-batch-2` G1

## Context

ADR-0014 ships local users with administrator-set permanent passwords, a
single `access` permission, Role links with no Scope or dates, and an instance
owner flag that bypasses every check. That leaves four gaps the batch-2 revamp
closes: there is no invite or first-login flow, no way to say "this Role
applies to company X", no guard against removing the last owner through Role
changes, and no explanation of why an action was allowed or denied. Sign-in
stays local only: no OIDC, no multi-factor sign-in, no TLS (O-008).

## Options considered

1. **Keep administrator-set passwords and add scopes only.** Smaller change,
   but the password still travels out of band with no expiry, and every new
   administrator password is permanent until changed. Rejected: the revamp
   requires single-use credentials.
2. **Invite links with a random token in the URL (chosen for delivery).**
   The administrator creates an invited user with no password and reads a
   single-use Challenge once, then passes it to the user however the site
   works today (no mail relay in the core; the outbox delivers it only where
   G8 finds a configured destination). The user sets their own password at
   first login. Resets work the same way.
3. **Union of all actions with union of all Scopes.** Simple to compute, but
   it lets two narrow assignments combine into a broad one (the PB §3
   counterexample: read on company A plus write on company B must not give
   write on company A). Rejected: evaluation tests each whole assignment.
4. **Keep the owner bypass and add granular permissions beside it.**
   Rejected: a bypass flag cannot be scoped, dated, delegated, or explained,
   and the Permission Explorer could not show why an owner action passed.

## Decision (proposed)

- **Challenges** (`athena_challenges`, system migration 12). A 128-bit
  random token per invite, reset, or activation, stored only as a SHA-256
  digest with purpose, user, generation, expiry, consumed time, and attempt
  count. Consumption is one conditional update, so two concurrent
  completions accept exactly one. A Challenge is refused when expired,
  consumed, superseded by a newer user generation, or wrongly presented
  five times. Resending an invite or starting a reset raises the user's
  generation, invalidating earlier Challenges; a reset also revokes the
  target's sessions. The Access Log records Challenge events without the
  value, and the public error message is one phrase for unknown, expired,
  and consumed Challenges.
- **User state** (`athena_users.state`): `invited`, `active`,
  `reset_required`, `suspended`. Only `active` users sign in. The password
  hash is nullable: invited users have none. `generation` invalidates old
  Challenges; `invite_delivery` is `manual` until G8 delivers through the
  outbox. Migration 12 marks enabled users `active` and disabled users
  `suspended`, so nobody gains or loses sign-in.
- **Assignments** (`athena_role_assignments`) replace the
  `athena_user_roles` links. One assignment holds a user, a Role, a Scope,
  an active-from date, an expiry date, and a revision. Scope is a JSON
  object from dimension name to the list of allowed values
  (`{"company": ["acme"]}`); `{}` is empty. An assignment outside its dates
  is absent. A user may hold the same Role twice with different Scopes or
  dates. Migration 12 copies each link to one assignment with empty Scope
  and no dates and keeps the old links as the rollback source.
- **Scope dimensions.** A Record Type field may declare a scope dimension
  (for example `company`). A Role lists its `requires_scope` dimensions as
  a JSON array. An assignment whose Scope misses a required dimension
  allows nothing. `system` marks the Roles the core ships.
- **Whole-assignment evaluation.** The gate allows an action on a row only
  when one single active assignment covers it: a Grant with the action, a
  true row condition, a matching Scope, and a sufficient field level, all
  in that same assignment. Results of whole matching assignments union;
  record updates recheck both the old and the proposed row. Scope
  compilation returns one disjunct per matching assignment.
- **No owner bypass.** The `Owner` short-circuits leave
  `internal/auth/principal.go`. The `is_owner` flag remains only as the
  marker for holding an ownership assignment: every assignment write
  re-marks its user in the same transaction, so revocation sticks and the
  seeding backfill only ever matches migration-era owners. The single
  `access` permission
  becomes `access:ownership`, `access:users`, `access:roles`,
  `access:assign`, `access:recover`, `access:sessions`, and
  `access:explore`. Migration 12 maps each existing `access` holder to all
  of them except `access:ownership`.
- **Last-owner guard.** Any request that would remove, suspend, or demote
  the last active verified Instance Owner is refused inside the same
  transaction as the change, including two concurrent removals of different
  owners. System Managers cannot assign ownership, platform status, or a
  Grant beyond their own delegation; a privileged recovery asks for the
  administrator's password again when it is older than ten minutes.
- **System Roles.** Platform Administrator (Build, Rules, Files, no
  business grants), Instance Owner (ownership, users, assign, recover, no
  business or Build), System Manager (users, roles, assign, sessions), and
  the business Role templates Receiving Clerk, Purchase User, Accounts
  Payable Clerk, Treasury Officer, Accountant, Auditor, and Report Viewer
  with no Grants on Record Types that do not yet exist.
- **Permission Explorer.** For a chosen user, action, and Record Type it
  shows which assignment matched or why each assignment failed (Grant,
  condition, Scope, dates, field level), including separation-of-duty
  conflicts. Editing a Role or assignment shows a preview of affected users
  and a diff, and applies with an expected-revision check.
- **Sessions.** Users list and revoke their own sessions; authorised
  administrators list and revoke another user's. Suspension and reset
  revoke sessions. Session rows gain nullable device and address-digest
  columns; old rows keep NULLs.
- **Proto.** `CreateUserRequest` marks field 3 and name `password`
  reserved; `ResetPasswordRequest` marks field 2 and name `new_password`
  reserved. New procedures cover invite, Challenge completion, resend,
  reset, assignments, access explanation, sessions, and ownership transfer.
  Every new procedure gets a `procedurePolicies` entry.

## Consequences

- New users cannot sign in until they complete an invite; administrators
  can no longer set a permanent password for someone else.
- Owners lose every permission that is not granted through a Role; the
  first owner keeps working because migration 12 preserves sign-in and G1
  seeds the Instance Owner Role.
- Each call still reloads the principal from the database; assignments add
  one Scope predicate per matching assignment to list queries.
- The desk gains a Users & Roles page in Build and a first-login
  completion screen; the CLI gains `user invite|resend|reset|suspend|
  reactivate|sessions` and `role assign|revoke|explain`.
- The `disabled` column stays as the stored flag behind `suspended` until
  a later pass unifies them; both refuse sign-in.

## Open points

- Multi-factor sign-in stays out until the sponsor decides it (O-008).
- Whether the local first owner also receives an explicit, removable
  Platform Administrator assignment so Build stays reachable on a
  single-user Workspace.

## Revisit when

The sponsor accepts or rejects the revamp access model, multi-factor
sign-in is chosen, or hosted use (TLS, deployment, OIDC) is planned.
