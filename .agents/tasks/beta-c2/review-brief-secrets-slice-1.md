# Review brief: C2 secrets slice 1 (migration 22 + ADR-0033)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c2-s1-r, detached at e3ba394 (docs ce0bfc2 + migration 28a9b5f rebased = e3ba394 on 6c871c9). Verify clean + diff is slice-1 scope only: migration 22 (secret refs, key versions, disposal register, holds) + sysmigrations_test + ADR-0033 + decision-log/README rows.
Read: .agents/tasks/beta-c2/brief-secrets.md slice 1; orders Z4 (binding text); ADR-0032/0031 (neighbour migrations, contiguity context).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Migration 22 applies cleanly up AND down on MariaDB (fresh chain incl. merged 21 — contiguity holds); tables/columns/keys match the slice scope; no data migration (nothing to migrate); no secret MATERIAL in the migration (structure only).
2. ADR-0033 (Proposed) transcribes Z4 EXACTLY (7yr/no-beta-delete, 90d anonymise + dietary consent, 1yr access log, 30d outbox/events, 90d job runs, env/keychain custody never-in-DB, rotate) — transcription is the crux; a wrong number here becomes law.
3. Log/README rows accurate; no other lane's files; no secret values in code/docs/tests.
4. Gates: worker affected green on 1f462ed + CODE-DISJOINT rebases to e3ba394 (shared file only docs/README.md, no test reads it — verify with git); rerun the migration tests directly yourself + affected reuse-or-rerun through suite; gofmt/build/vet clean.

Report: say master "C2 SECRETS SLICE 1 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c2/review-secrets-slice-1.md with the verdict line.
