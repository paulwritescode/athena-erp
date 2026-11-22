# Summary: C2 slice 1 — migration 17 + ADR-0028 (8.1 adjusted)

Merged 2026-11-22 as 5d1e55f (reviewed at fd7e372, rebased cleanly onto
172c749). Scope: batch-2 G8.1 adjusted for open Z4.

- Migration 17 `events_jobs_workflows`: new `athena_events`,
  `athena_inbox`, `athena_event_subscriptions`, `athena_jobs`,
  `athena_schedule_occurrences`, `athena_workflow_instances`,
  `athena_tasks`; extends `athena_outbox` with dispatch columns only
  (id/type/payload preserved, proven by
  `TestEventsJobsWorkflowsMigrationExtendsTheOutbox`).
- Deferred on open Z4 (retention periods, key custody undecided): 8.8/8.9
  secrets tables, 8.10 lifetimes/disposal tables, secrets ADR. Absence
  pinned by test.
- ADR-0028 "Events, jobs and workflows" (Proposed): P16-before-P14
  reorder for this spec, retention + key custody OPEN, no invented
  policy. Decision-log + README rows added.
- Review: PASS, no blockers (review-slice-1.md). Notes: platform has no
  down-migration mechanism (future briefs: "fresh full up + incremental
  up"); new tables index-only, no FKs (needs a later migration if wanted).
- Merge-time verification on 5d1e55f: gofmt empty, go build/vet clean,
  make test exit 0 (563 web tests), make generate twice with no diff.
- Open: slices 2+ (8.2 Pack subscriptions, outbox worker, jobs, workflows,
  secrets/lifetimes once Z4 is decided).
