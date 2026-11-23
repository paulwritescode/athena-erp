# Summary: C2 slice 3 — job runner + occurrences + worker startup

Merged 2026-11-23 as 415fed7+dce52a3+461633f+f3d547f in
train1 (built at f186fe7/b866c41/13c7973/c18ae67 on
f02a4b7; rebased cleanly, C2 content byte-identical).

- `internal/jobs/`: leased claims with expiry + fencing,
  backoff, owner assignment, restart recovery, occurrence
  keys (timezone + catch-up), server worker startup,
  scheduled purge of spent Challenges/dead sessions.
- Review round 1 NEEDS_CHANGES (B1 restart-recovery
  threshold + F1–F5); fixes: row-first CASE recovery +
  test, Errors removal, payload clone, release attempt
  counting, claim comment, migration 19 (occurrence_key
  VARCHAR(700), 602-char key tested end to end).
- Property 15 (occurrence uniqueness) green; purge deletes
  only dead rows with one minimal log row, no secrets.
- Z4 still deferred (8.8/8.9/8.10 absent), Z2 untouched.
- Review: round-2 PASS (review-slice-3-round2.md); 6/6 fix
  tests rerun directly; affected reused green on c18ae67.
- Merge-time verification: train1 tip gate (full make test
  exit 0, web 613/613) on 919bab4.
- Follow-ups: none new (round-1 F-items all fixed).
