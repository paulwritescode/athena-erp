# System migration numbers

Master reserves a number here before a worker writes the migration. Merged numbers are final. A reserved number that is abandoned is marked **released** and not reused.

| Number | Task | State |
| --- | --- | --- |
| 1 to 16 | earlier work | merged |
| 17 | C2 slice 1 (events, outbox, jobs tables, ADR-0028) | merged |
| 18 | C4 (trees, effective dating, UoM; ADR-0029) | merged 2026-11-23 (d54d6e4) |
| 19 | C2 slice-3 fix F3 (widen athena_jobs.occurrence_key) | merged 2026-11-23 (f3d547f) |
| 20 | C5 (party ledger, ADR-0031) | merged 2026-11-24 (f00fc58) |
| 21 | C7 (stock ledger, ADR-0032) | reserved 2026-11-24 |
