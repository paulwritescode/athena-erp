# ADR-0014: Local users, Argon2id, database sessions and Grants

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0001, ADR-0008 (Grant vocabulary), ADR-0010, ADR-0012 (Audit Log, request IDs), ADR-0013 (row conditions use the expression language), O-008, spec sections 10.9 and 21, roadmap phase P8 and user answer 5

## Context

Until P8 every ConnectRPC method was open and every change was attributed to "Local administrator". The server stayed on loopback for that reason. The user's answer 5 asks for local users, Argon2id password hashing, server-side database sessions in HttpOnly SameSite cookies, CSRF protection and Role permissions with permission levels and row conditions, enforced in Go on every ConnectRPC method, denying by default, including for services added before P8. OIDC is a later option.

## Options considered

1. **Signed stateless tokens (JWT in a cookie).** No session table, but no way to end a session at once, and a Role change waits for the token to expire. Rejected: the spec needs revocation and immediate Role changes.
2. **Database sessions with an opaque token (chosen).** One row per session holds the SHA-256 of a 256-bit random token; the browser holds the token in a cookie. Revocation, idle and absolute expiry, and "sign out other sessions" are single-row or single-statement updates.
3. **bcrypt or scrypt for passwords.** Both are acceptable, but the spec and the user answer name Argon2id, the winner of the Password Hashing Competition and the first choice of RFC 9106 and OWASP.
4. **An external identity provider (OIDC) first.** Recorded as a future option; it adds a network dependency the core does not need for local use (D-006).

## Why golang.org/x/crypto/argon2

- The Go standard library has no Argon2. `golang.org/x/crypto` is maintained by the Go team, pure Go, no cgo, and was already in `go.sum` as an indirect dependency; P8 makes it direct (v0.59.0). It keeps the single static binary of ADR-0001.
- Alternatives (`github.com/alexedwards/argon2id`, `github.com/matthewhartstonge/argon2`) are thin wrappers over the same package; the PHC encoding they add is about 40 lines here.

## Decision (proposed)

- **Package.** `internal/auth` owns users, password hashing, sessions, Roles, Grants and the Access Log. It never logs or returns a password, a token or a hash. `internal/server/gate.go` is the single permission gate; `internal/server/access.go` applies row conditions, field levels and separate duties inside the handlers; `internal/server/auth_rpc.go` implements `athena.v1.AuthService`.
- **Tables** (system migration 9): `athena_users`, `athena_roles`, `athena_grants`, `athena_user_roles`, `athena_sessions`, `athena_access_log`. UUIDv7 ids, except the Access Log, which uses an auto-increment id for ordering and paging.
- **Passwords.** Argon2id with 64 MiB, 3 passes and 4 lanes (RFC 9106, second recommended option), a 16-byte random salt and a 32-byte key, stored as a PHC string with its parameters. Verification reads the parameters from the hash (bounded so a corrupt row cannot allocate gigabytes), compares in constant time, and rehashes after a successful sign-in when the defaults have changed. At most four hashes run at once. A sign-in for an unknown, disabled or locked user verifies a dummy hash, so its timing and its message match a wrong password. Policy: 12 to 256 characters, not the username.
- **Lockout.** Five wrong passwords in a row lock the user for 15 minutes. `athena user set --unlock` and `UpdateUser.unlock` clear it.
- **Sessions.** A 256-bit random token, sent only in the `athena_session` cookie (`HttpOnly`, `SameSite=Strict`, `Path=/`, `Secure` when the page was served over HTTPS). The table stores its SHA-256. Sessions end after 8 hours idle or 7 days, on sign-out, on a password change (other sessions) or reset (all sessions), and when the user is disabled. The principal (user, Roles, Grants) is reloaded from the database on every call, so a Role change takes effect on the next call.
- **CSRF.** Every method except `Login`, `GetSession` and `GetSystemInfo` needs `X-CSRF-Token`, an HMAC-SHA-256 of a fixed label keyed by the session token. The desk learns it from `Login` or `GetSession`; another site cannot read it. As a second guard, the HTTP handler refuses a state-changing request whose `Origin` is not the server's own host, and sets `Content-Security-Policy` (scripts and connections from `'self'` only, no framing), `X-Content-Type-Options`, `X-Frame-Options` and `Referrer-Policy`.
- **Gate.** A ConnectRPC interceptor, after the request-ID and tracing interceptors, looks up each procedure in `procedurePolicies`. A procedure missing from the table is denied, and a test fails when a generated procedure has no entry. Policies: public (system info, sign-in, session check), signed in (sign-out, own password, the Record Type list), a system permission (`build` for saving, planning and deleting Record Types; `access` for users and Roles; `audit` for verifying the Audit Log and reading the Access Log), or a Grant action on the Record Type the request names (read, create, write, delete, submit, cancel, amend, audit).
- **Roles and Grants.** A Role has system permissions and Grants. A Grant names a Record Type (or `*`), its actions, a permission level 0 to 9, an optional row condition and an optional separate-duties flag. Grants are additive; with none, everything is denied. An **instance owner** holds every permission without Grants. Owners are created with `athena user create --owner` only; the API cannot create one, and the last enabled owner cannot be disabled.
- **Row conditions** use the expression language (ADR-0013) and are translated to SQL with `Translate`, so lists, lookups and single-Record reads only touch rows the user may open. `user.name` stands for the signed-in username, substituted as a string literal (usernames contain only `[a-z0-9._-]`). A Record outside the read scope reads as not found; one that is readable but outside the scope of the action is denied. A create is refused when the new Record would fall outside the creator's scope. A condition that no longer compiles allows nothing. A Grant's Record Type and condition are checked when the Role is saved.
- **Field permission levels.** `RecordTypeField.permission_level` (0 to 9, stored in the definition, no table change). A field above the user's highest level on the Record Type is left out of Records, of the Record Type definition sent to non-builders, of filters and expressions (it is not in scope), and of preview results; writing it is denied. Audit Log rows read through `ListRecordHistory` show the field name with `masked: true` and no values; Document Version snapshots and diffs drop it. The stored rows and their hashes are unchanged, so `athena audit verify` still verifies. Reading the history, versions and timeline needs the `audit` action.
- **Separate duties.** When every Grant that allows `submit` on a Record Type has the flag, the user who created a Record cannot submit it.
- **Access Log.** Sign-ins, failed sign-ins, lockouts, sign-outs, denials, password changes and resets, and user and Role changes, with the username, procedure, Record Type, action, request ID and a fixed detail phrase. No password, token, Record value or attempted unknown username.
- **Actor.** Records, Audit Log rows, versions and the timeline carry the signed-in username. The CLI opens the database directly as the operator (the person with the database login), has no session, applies no Grants and is recorded as "Local administrator". A call that came through the API without a principal is refused, never treated as the operator.
- **Telemetry.** New spans (`athena.login`, `athena.logout`, `athena.change_password`, user and Role operations) carry the operation, request ID and outcome only. A test with an in-memory exporter, the process log and the auth tables checks that no password, session token or CSRF token appears.
- **Parent rollups in one save** (carried over from P7). `SaveRecordTypeRequest.parent` saves a child Record Type and its parent's new definition in one request. The parent's expressions are checked against the new child before anything is saved; the parent save runs after the child save on the same schema lock. If the parent save then fails, the child stays saved and the error names the parent.

## Consequences

- **Breaking for existing installs:** the desk shows a sign-in page and, before the first user exists, tells the person to run `athena user create <name> --owner`. API clients need a session cookie and the CSRF header.
- The server still binds to loopback by default. Authentication exists, but exposing the server beyond the local machine also needs TLS termination, a reviewed deployment and this ADR accepted; that stays a separate decision.
- Each call reads the session, the user, the Roles and the Grants (three small queries). There is no cache.
- Users and Roles are managed with the `athena user` and `athena role` commands and the `AuthService` API; the desk has sign-in, sign-out and change password only.
- PostgreSQL is untested (ADR-0005).

## Not built

OIDC and SAML, multi-factor sign-in, invitations and first-login password changes, password history and breach checks, user-permission data scopes (spec 21.5 "applies to company X"), time-limited support access, period-lock overrides, report and export permissions (no reports yet), attachment permissions, an Access Log retention policy, a desk page for users and Roles, rate limiting by IP address, session listing per user.

## Revisit when

A hosted deployment is planned (TLS, `Secure` cookies always, a reverse proxy and the `Origin` check), OIDC is needed, P11 adds company scopes, P14 adds reports and exports, or sign-in throughput needs a cheaper hash or a principal cache.
