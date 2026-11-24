# ADR-0033: Secrets and retention

- **Status:** Proposed
- **Date:** 2026-11-24
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0028, Z4 (retention periods and key custody, decided 2026-11-24)
- **Implementation notes (status only):** migration 22 (below) is reserved and lands once migration 21 (C7) merges; `internal/secrets/`, disposal and the `athena secrets` commands follow in C2 secrets slices 2–5. The decision stays Proposed until the sponsor accepts it.

## Context

Batch-2 items 8.8/8.9 (secret references and field encryption) and 8.10 (lifetimes, holds and disposal) were deferred on open Z4: ADR-0028 and migration 17 deliberately carry no secret or disposal tables. The sponsor has now ruled on Z4 (2026-11-24), unblocking this decision exactly as ruled — no redesign:

- Retention is 7 years ledger/invoices/stock/audit (NO beta auto-delete of this set), guest personal data anonymised 90 days after the event (consent model for dietary data), Access Log 1 year, outbox/events 30 days, job runs 90 days.
- The Workspace key comes from env or OS keychain, is owner-held, and is NEVER stored in the database or backups.
- `athena secrets rotate` rotates keys.

Requirements 8.13–8.19 demand references-not-values, a key service interface with bound authenticated context, lifetimes, previewed disposal with holds, disposal reapply on restore, and transient-data purge. The design puts references in `athena_secret_refs` with a provider interface (`file` for local, extension point for OpenBao or AWS), field encryption in `internal/secrets/keys.go` (AES-256-GCM, authenticated data of Workspace, Record Type, Record ID and field), lifetime attributes (`lifetime`, `retention_trigger`, `retention_class`, `disposal_action`, `archive_readers`, `hold`) on Record Type definitions, and `athena_key_versions`, `athena_disposal_register` and `athena_holds` tables.

## Options considered

1. **References plus provider interface, key versions, bound field encryption, disposal register with holds (chosen).** The database holds references and ciphertext only; values come from the owner-held Workspace key via a provider interface with a local file provider. Key versions rotate without rewriting history. Disposal previews candidates, skips held items and dependent evidence, and records outcomes; a restore reapplies the register before reopening.
2. **Secret values in database rows.** Rejected: values would leak into backups, diffs, logs, spans and Pack source, violating Requirement 8.13 and the Z4 custody ruling.
3. **External KMS only, no local provider.** Rejected for beta: the single-binary install must work with no operator dependency. The provider interface keeps OpenBao/AWS as an extension point.

## Decision (proposed)

- Migration 22 `secrets_retention` creates `athena_secret_refs` (references, never values), `athena_key_versions` (key versions for rotation), `athena_disposal_register` (disposal outcomes and exceptions) and `athena_holds` (holds that disposal must skip). It seeds nothing.
- The Z4 schedule above is baked in exactly: 7 years ledger/invoices/stock/audit with NO beta auto-delete of this set; guest personal data anonymised 90 days after the event; Access Log 1 year; outbox/events 30 days; job runs 90 days.
- Workspace key custody follows the ruling: env or OS keychain, owner-held, never in the database or backups; `athena secrets rotate` rotates keys.
- A secret value stays out of Record CRUD, Pack source, Starlark, URLs, frontend responses, diffs, Reports, logs, spans and errors. A protected value copied into another Record fails to decrypt (context binding).
- Disposal previews candidates, skips held items and dependent evidence, and records outcome and exceptions; a restored backup reapplies the disposal register before reopening; expired Challenges and sessions purge as a job with a minimal Access Log event.

## Consequences

- C2 secrets slices 2–5 build `internal/secrets/`, Property 16, lifetimes/disposal and `athena secrets` on this decision; Desk secret-reference bindings stay with G18.
- Operators get `athena secrets rotate` and retention behaviour they can explain from the Z4 schedule; beta never auto-deletes the 7-year set.
- ADR-0028's Z4-open notes are superseded by this ruling for secrets and retention; ADR-0028 itself still needs its status notes reconciled.

## Revisit when

The sponsor changes the retention schedule or custody model, a custody incident forces rotation design changes, or G18 binds secret references to Desk fields.
