# ADR-0024 Notifications

- **Status** Proposed
- **Date** 2026-11-21
- **Decision owner** Awaiting sponsor review
- **Related decisions** ADR-0006, ADR-0014, ADR-0016, ADR-0017, ADR-0022

## Context

The Desk needs an inbox for each account. A user must be able to find an event after reloading, including a save that a Rule stopped.

## Proposed decision

Migration 15 adds `athena_notifications`. The owning service inserts a UUIDv7 row in the transaction that records the event. A stopped validation or guard writes its notification with the durable Rule Run log. The document transaction can then roll back. Simulations produce no notifications.

| Event | Recipient |
| --- | --- |
| First-login invitation accepted | Account that issued the accepted invitation |
| Assignment granted, replaced or revoked, or assigned Role edited or deleted | Affected account |
| Third and subsequent consecutive wrong passwords, including lockout | Account whose sign-in failed. Root receives alerts and retains its lockout exemption. |
| Another person ends a session, including administrative suspension or reset | Session owner |
| Validation or guard stops a document operation | Signed-in person performing the operation |
| Period becomes soft-closed or hard-closed | Active accounts with a currently effective `ledger:write` permission and the Workspace root |

The Challenge stores its issuer's user ID from migration 15 onward. An operator-issued invitation or an invitation issued before migration 15 can have no attributable recipient. Operator actions have a null actor ID. The lifecycle has no assignment of a submission to an approver, so approval notifications await that workflow.

Go builds display text from the kind, subject label and typed parameters. Subject references identify the relevant account, session, document or period. The only payload parameter currently used is the number of unsuccessful sign-ins. Record field values, Rule messages and credentials stay outside inbox rows. Responses include the actor's ID and current display name when available, and UTC timestamps.

Each RPC takes the inbox owner from the session. A composite creation-time and ID cursor resolves timestamp ties, and a second index supports unread queries. Marking an item read is idempotent. The operator CLI can explicitly select an account. Using the existing post-commit outbox would introduce a delivery worker and a window in which the event exists without its inbox item.

## Sibling note on Workspace appearance

`WorkspaceService` publishes the kernel-owned scalar `athena/brand.accent` at the empty Workspace scope in the G5 Settings chain. Each successful save appends one published generation. A fresh Workspace uses ink. Generic ledger setting writers cannot publish custom appearance values. The public sign-in call returns the preset, optional custom colors and resolved accent tokens. Only `settings:appearance` authorizes the RPC write. Existing storage covers the feature, so it needs no separate ADR-0025 or appearance table.

For a custom accent the supplied light-theme color must pass 4.5:1 against the light theme's primary text. The server derives the closest passing dark-theme lightness against the dark theme's primary text. It fits both resolved tokens into sRGB by reducing chroma only when needed. The Desk should apply `light_accent` and `dark_accent` for Custom. A contrast failure returns the nearest passing light-theme input and its dark counterpart. Up to five chart slots can override their defaults. [WCAG contrast ratio](https://www.w3.org/TR/WCAG22/#dfn-contrast-ratio)

## Revisit when

Review the decision when the sponsor evaluates the beta, when approval recipients exist, or when notification delivery or retention needs a background worker.
