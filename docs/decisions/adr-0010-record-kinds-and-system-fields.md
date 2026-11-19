# ADR-0010: Record kinds, system fields and field attributes
- **Status:** Proposed
- **Date:** 2026-11-19
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0003, ADR-0006, ADR-0007, ADR-0008, ADR-0009, spec sections 2, 3.2, 24.1 to 24.5, roadmap phase P4

## Context
Every Record Type had one implicit kind and four system columns. The spec makes the kind decide what the kernel enforces, adds system fields to every Record, and lets each field declare attributes the kernel enforces uniformly.

## Options considered
1. **Add the spec's `version` column next to `revision`.** Two counters for one idea.
2. **Keep `revision` as the spec's `version` (chosen).** No new column; the spec name maps to the existing optimistic-concurrency counter.
3. **Enforce unique values only in Go.** Friendly errors but racy, and a soft-deleted row would still need excluding.
4. **Go check plus a database unique index (chosen).** Go names the field in the message; the index closes the race.

## Decision (proposed)
- **Kinds.** `kind` is stored in the Record Type definition. Users can create `master`, `document` (with a `class`: posting, non_posting, work, batch), `child` (with a fixed `parent_record_type`) and `setting`. `ledger`, `balance`, `log`, `snapshot` and `meta` are rejected until the kernel phases that own them land. An empty kind means `master`. Kind and parent are fixed after the first save.
- **System columns** (added by the kernel, reserved field keys, never written from a request): `created_by`, `modified_by`, `def_version` (Record Type revision at the last write), `request_id` (UUIDv7 per write call, also on the kernel span) for every kind; `owner` for master and document; `deleted_at` for master; `parent_id` and `idx` for child. Until sign-in exists the actor is `Local administrator`. `docstate`, `name`, `company`, `parent_type`, `parent_field` and `amended_from` are not added yet; they arrive with lifecycle (P5), numbering (P6) and the `table` field type.
- **Behavior.** Master delete is a soft delete (`deleted_at`, revision bump, history `deleted`); listed and read paths hide it. A deleted master keeps its unique values reserved. Setting holds one Record (serialized by a lock on the definition row) and cannot be deleted. Child Records need an existing live `parent_id`, get the next `idx` per parent, and block deleting the parent. Document Records are plain drafts until P5.
- **Field types added:** `Money` (BIGINT minor units; fixed `currency` and `scale`; values with more places than the scale are rejected, never rounded), `Quantity` (DECIMAL(18,6); fixed `uom`, `scale` up to 6), `Enum` (permanent lowercase values with labels). A `currency_field` or UoM master arrives with P11 and P20.
- **Attributes enforced on every write:** `unique`, `default` (literal; Checkbox true/false), `min`, `max` (exact decimals or dates), `pattern` (RE2, whole value), `max_length`, `read_only` (only the default sets it), `set_once`, `index`. `no_copy` is stored and validated but has no effect until duplicate or amend exist. Changing a Money currency or scale, a Quantity unit, or an Enum list clears values that no longer fit, through the existing confirmed data-loss plan.
- **Migration 6** (`record_kinds_and_system_fields`) turns existing Record Types into masters, adds the columns, and backfills the local actor and the revision. It stops with a clear error when a saved field already uses a now-reserved key.

## Consequences
- The proto gained kind, class, parent and the attributes; `RecordService` gained `parent_id` on create and list. This is a breaking change nothing outside this repository consumes.
- Soft-deleted Masters keep their unique values: accepted by default as D-014; the owner may override.
- Reserved keys include `owner`, `idx` and `parent_id`, which a user might have wanted as field keys.

## Revisit when
P5 (documents become immutable once submitted), P8 (actors become user ids), or the owner wants deleted values released.
