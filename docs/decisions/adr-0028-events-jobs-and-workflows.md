# ADR-0028: Events, jobs and workflows

- **Status:** Proposed
- **Date:** 2026-11-22
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0023, ADR-0006, ADR-0012, Z4 (retention periods and key custody, open)

## Context

Committed facts must reach their consumers exactly once per consumer and effect, long waits must survive restarts, and Report schedules and delivery (roadmap P14) need durable jobs before balances and reports land. The batch-2 plan therefore runs G8 (roadmap P16: jobs, outbox, events, workflows, schedules) before G9 (roadmap P14): the roadmap order P14 then P16 is superseded for this spec only. Retention periods and key custody are undecided (Z4), so secret references, field encryption and Record lifetimes stay out of this decision entirely: no secret or disposal tables, no second ADR for them.

## Options considered

1. **Transactional outbox plus inbox with effect keys, leased jobs, stored workflow instances (chosen).** The producer writes the event envelope and its outbox row in the business transaction; a dispatcher delivers after commit; each consumer records its inbox row and local effect in one transaction and skips repeats by consumer plus effect key. Jobs lease with expiry and a fencing counter; schedules own one occurrence row per Workspace, schedule, scope and period; workflows persist instances, waits, gated tasks and a cycle budget.
2. **Direct in-transaction delivery.** Consumers would run inside the business transaction, coupling producer latency and failure to every subscriber and losing delivery on crash between commit and dispatch.
3. **External queue.** Adds an operator dependency the single-binary install does not have; the database already orders and durably holds the rows.

## Decision (proposed)

- Migration 17 `events_jobs_workflows` creates `athena_events` (immutable id, recorded sequence, type, schema version, Workspace, producer, subject type/id/revision, actor, Operation_ID, correlation and causation ids, minimal payload), `athena_inbox` (consumer, event id, effect key, result), `athena_event_subscriptions` (consumer, event type, mode, action, effect key, exhaustion behaviour, declaring Pack), `athena_jobs` (leases with expiry and fencing, backoff, occurrence key, failure owner), `athena_schedule_occurrences` (unique Workspace, schedule, scope and period with timezone, catch-up policy, lease, watermark and result), `athena_workflow_instances` (definition version, phase, expected and received facts, wait, due time, assigned Role and Scope, cycle budget, completion evidence) and `athena_tasks` (unique instance plus task key for gated creation). It extends the minimal `athena_outbox` from migration 14 with dispatch columns and keeps its id, event type and payload columns untouched, as ADR-0023 requires.
- Producers (`internal/posting`, `internal/lifecycle`) write the envelope and outbox row in the caller's transaction and emit nothing before commit. The dispatcher recovers pending rows after a crash between commit and dispatch. Replays rebuild projections without sending messages or running business commands.
- A reaction that would raise a cycle past its declared budget stops with an operator-visible state. A task gated by two conditions is created exactly once in either arrival order. A scheduled expiry rechecks its subject under lock before changing it.
- OPEN (Z4): retention periods and key custody. Batch-2 items 8.8/8.9 (secret references and field encryption) and 8.10 (lifetimes, holds and disposal) are deferred until the sponsor rules; their tables and ADR are not part of migration 17 or this decision.

## Consequences

- Posting and lifecycle submits grow an event write inside their transaction; the server starts job workers on boot.
- Operators get Jobs, Events and Workflows pages plus `athena jobs|events|workflows` commands; invite and reset delivery moves to the outbox where a mail destination exists and keeps the manual code path otherwise.
- Purge of expired Challenges and sessions runs as a job with a minimal Access Log event once the runner exists.

## Revisit when

The sponsor rules on Z4 (retention periods and key custody), G9 designs Report schedules on the runner, or G10 declares Pack subscriptions at install.
