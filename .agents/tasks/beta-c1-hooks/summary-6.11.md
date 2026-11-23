# Summary: C1 6.11 — G6.8–6.9 docs

Merged 2026-11-23 as fdf98c7+d897d01 in train1 (built at
203b758/bdce628 on f02a4b7; rebased cleanly, patch
identical — no overlap with main).

- AGENTS.md, README.md, overview.md, decision-log.md:
  lifecycle hooks, Posting Rule type + activation gate,
  preview/status/log RPCs, `athena post` CLI; every claim
  verified against merged code (paths, names, grants,
  R-citations); "not production-ready until accepted"
  everywhere; Z2 open with no invented default.
- Review: NEEDS_CHANGES then PASS (review-6.11.md) — one
  false mechanism claim (overview:98 said posting Rules
  run through the lifecycle registry; fixed to "the
  posting path, after the registry hooks").
- Docs-only: affected no-op, no web gates.
- Merge-time verification: train1 tip gate (full make test
  exit 0, web 613/613) on 919bab4.
- Follow-ups: overview:101 log-write timing wording.
