# Summary: C2 slices 4–6 — workflows + delivery + events surface (split)

Merged 2026-11-24 as 2dd8bbf..87db25e (7 commits: split brief,
sub-slice A, 4× sub-slice B, wiring fix; built at b877fbc..
f4e57fd on 58948de, cherry-picked cleanly onto 4acb8d4).

- Sub-slice A (8.6+8.7+8.11): `internal/events/workflow.go`
  (new, 743 lines): instances (definition version, phase,
  facts, waits + deadlines, due time, Role/Scope, cycle
  budget, evidence), gated task creation (duplicate-key
  idempotent), cycle budget 25; invite/reset delivery via
  outbox where a destination exists, manual code path
  otherwise. Property 14 green.
- Wiring fix (tip): consumer declared on WaitFor + Serve
  wiring so the dispatcher wakes waits (the split's own
  found gap, integration-tested).
- Sub-slice B (slice 6): `events.proto` (EventService +
  JobService, 9 procedures) + handlers + gate policies
  (all present) + `athena jobs|events` CLI (no secrets
  CLI) + Jobs page (replaces stub) + Settings background-
  work section (extends existing) + docs.
- Crash-between-commit-and-dispatch test green.
- No migration; Z4 secrets/lifetimes (8.8/8.9/8.10, mail
  custody) deferred to C2-secrets slices; Z2 untouched.
- Review: PASS (review-split-4-6.md) — 7/7 verified;
  browser evidence interim-rule (scripted harness-only
  failure, 12 real shots eyeballed, light/dark caveat
  checked).
- Merge-time verification: full make test exit 0 (Go ok,
  web 624/624) on 87db25e.
- Follow-ups F1–F6: no `athena workflows` CLI; no Desk
  Workflows page; row-level exhaust behavior (pre-existing);
  light-shot caveat over-generalized (harmless); harness
  Background-work wait + retake 375-light shot; silent
  unregistered-handler skips need an operator counter at G10.
