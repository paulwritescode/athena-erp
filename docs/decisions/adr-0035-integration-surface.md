# ADR-0035: Integration surface

- **Status:** Proposed
- **Date:** 2026-11-24
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0028 (events, outbox, Operation_ID), ADR-0033 (secrets hold the HMAC keys), ADR-0015 (workspace files)
- **Implementation notes (status only):** migration 25 is reserved and lands once migrations 23+24 (F4) merge; `internal/integrations/`, Property 26, attachments/search/comments/import, the RPC/CLI/Desk surface and docs follow in C10 slices 2–6. The decision stays Proposed until the sponsor accepts it.

## Context

Outside input must reach the Core through the same permission gate as everything else (Requirement 15): verified provider callbacks, content-addressed attachments, scoped search, comments that inherit Record visibility, and CSV/JSON import/export with dry run. The batch-2 plan numbers this migration 23 with ADR-0032; both numbers are taken (F4 lists kernel table, C7 stock), so this decision uses migration 25 with ADR-0035 and changes nothing else about the plan.

## Options considered

1. **Verified provider inbox plus Core-owned attachments, search, comments and import/export (chosen).** A provider inbox verifies webhook signatures (HMAC with a bounded previous-key window), merchant, amount and identity before recording evidence, dedupes by provider event id, and maps each event to one Operation_ID. Attachments store content-addressed with owner, digest, media type, size and immutable version through the B9 Image storage, which enforces the workspace blob path rules; every download authorises. Search runs through a store dialect method (MariaDB FULLTEXT) filtered by the reader's Scope. Comments and mentions inherit Record visibility. Import/export runs CSV and JSON with dry run through the Gate. An abandoned-upload job removes orphaned uploads.
2. **Unverified callbacks straight into business commands.** Rejected: outside input would bypass Core checks, duplicate deliveries would double-apply, and there would be no evidence row to audit.
3. **Attachments as plain workspace files.** Rejected: files lack content addressing, per-download authorisation and the orphan lifecycle the inbox needs.

## Decision (proposed)

- Migration 25 `integrations` creates `athena_provider_inbox` (provider, provider event id, signature evidence, Operation_ID), `athena_attachments` (owner, digest, media type, size, immutable version), `athena_search_index` and `athena_comments` (Record-visibility comments and mentions). It seeds nothing.
- `internal/integrations/` verifies provider events with HMAC keys from `internal/secrets` (never inline key material), accepts the previous signing key for a bounded window after rotation, dedupes events independently of the key used, and maps each provider event to exactly one Operation_ID. There is no unsupported "mark paid" path from a provider callback.
- Attachments reuse the B9 Image storage (`Store.PutImage` path: validated bytes, metadata stripped, versioned, audited), which sits on the workspace blob path rules (`CleanPath`, os.Root confinement); downloads authorise per call.
- Search, comments, import/export and the abandoned-upload job behave per Requirements 15.4–15.8. The RPC surface (`IntegrationService`), gate policies, `athena inbox|attachments|search|import|export` commands and the RecordsPage attachments/comments UI land with the later slices; Desk edits wait for the RecordsPage GO.
- Spans, logs and errors carry ids, counts and outcome codes only; HMAC keys, signatures and file bytes never appear in them.

## Consequences

- C10 slices 2–6 build the inbox (with Property 26), attachments/search/comments/import, the RPC/CLI/Desk surface and docs on this decision.
- Integrators get one verified door with evidence rows; operators get inbox and attachment CLIs plus Record-page attachments and comments.
- The provider key lifecycle (registration, rotation window) reuses the secrets service instead of inventing a second one.

## Revisit when

The sponsor adds a provider family the HMAC model cannot verify, attachment scale outgrows workspace-local blob storage, or search needs more than FULLTEXT relevance.
