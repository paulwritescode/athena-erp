# ADR-0026 Session lifetimes, step-up and rate limiting

**Proposed · 2026-11-21.** Amends ADR-0014 and ADR-0017. The sponsor approved the B7 lifetime, step-up and sign-in/write rate defaults on 2026-11-21. The implementation choices below are proposed for review.

## Context

Workspace operators need to bound the time a lost browser session can access ERP data, require recent password proof for destructive changes and bound repeated sign-in and write requests. Athena runs one Go process per Workspace. Server-enforced idle and absolute deadlines and renewal after privilege changes follow the [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).

## Decision

Migration 16 stores operator overrides in `athena_security_settings` and each session's idle duration, absolute deadline and last password proof in `athena_sessions`. Dedicated settings keep authentication policy available before ledger setup. The alternative was the G5 Settings chain. Existing sessions require fresh sign-in on upgrade.

| Setting | Default | Operator range |
| --- | ---: | ---: |
| `security.session.idle_minutes` | 30 | 5–480 |
| `security.session.build_idle_minutes` | 15 | 5–480 |
| `security.session.absolute_hours` | 12 | 1–168 |
| `security.rate.ip_per_minute` | 10 | 1–10,000 |
| `security.rate.username_per_15_minutes` | 5 | 1–10,000 |
| `security.rate.writes_per_minute` | 60 | 1–10,000 |
| `security.rate.reports_per_minute` | 10 | 1–10,000 |
| `security.rate.exports_per_minute` | 10 | 1–10,000 |

The final two defaults and rate bounds are implementation choices. A remembered everyday session gets a persistent cookie and a seven-day absolute deadline. Root and users with Build or administrative permissions get the shorter idle policy and a browser-session cookie. The server returns both deadlines to the Desk. Login, completed Challenges and step-up rotate the presented credential. Step-up retains the original absolute deadline and rate budget. Explicit permission changes end affected sessions transactionally. Dated assignments end the session at the next effective boundary.

The gate requires password proof less than 15 minutes old for destructive changes by root or a builder. Sign-in counts as proof. The procedure list lives beside the permission policies in `internal/server/gate.go`. Record Type saves require fresh proof when confirming data loss, and invites when they grant Roles. A wrong password at step-up, ownership transfer or reset start counts toward the same failed-attempt counter and lockout as sign-in and is logged as `step_up_failed` without the password; a successful step-up is logged as `step_up`. The failure names only the password, never the username. Logout and password change clear the cookie, cache and browser storage. Password change ends all browser sessions.

Login, CompleteChallenge and StartReset share buckets for the socket peer IP and normalized username, including root. Writes use a per-session bucket. Audit verification uses the expensive-report bucket. Report and export procedures have independent budgets when introduced. Limits refill continuously and apply across all tabs. The process bounds its bucket map to 8,192 hashed keys with 15-minute idle expiry. At capacity it delays new keys until space becomes available. Denials return a retry interval in the Connect detail and HTTP header. The Access Log records the fixed procedure and category.

## Consequences and revisit conditions

A policy edit affects newly created sessions and the next rate-limited call. Restarting the single server resets in-memory buckets. Review shared limiter state and trusted peer addresses before introducing multiple replicas or a reverse proxy. Desk tab coordination, warning and password prompts are a separate B7 deliverable.

## Addendum: security settings RPC (2026-11-21)

`AuthService.GetSecuritySettings` returns every setting with its value, default, bounds, unit and a one-line explanation; it needs `access:sessions` or `settings:security`. `AuthService.SetSecuritySettings` applies a partial update through the same validated, atomic, audited function as `athena security set`; it needs `settings:security` (a new system permission, counted as build access) and fresh step-up proof, and writes a `security_settings_changed` Access Log row naming each key with its old and new value. No new service: the settings belong to AuthService, which already owns them.
