# Summary: C2 secrets slice 1 (migration 22 + ADR-0033)

Merged 2026-11-24 as bfb9efa/65e59cc (cherry-picks of f336a60/e3ba394 onto post-F2s1 main), 2 commits.

Scope: migration 22 (secret refs, key versions, disposal register, holds tables; seeds nothing; Z4 contiguity test now asserts absence at 17 + presence at 22) + migration tests + ADR-0033 "Secrets and retention" (Proposed, transcribes Z4 exactly: 7yr/no-beta-delete, 90d anonymise + dietary consent, 1yr access log, 30d outbox/events, 90d job runs, env/keychain custody never-in-DB, rotate) + decision-log/README rows. No secret material anywhere (structure only).

Evidence: affected green on 1f462ed (cmd/athena + server exit 0); two clean rebases (F1/F6/docs, then F2s1/docs) carried under the Stylus disjoint rule (shared files docs-only, verified by the master, no test reads them); worker correctly refused a stale PASS once. Merge gate on 0f855ed: 1150s, 17 Go pkgs ok (posting 1072s, server 1028s, cmd/athena 637s DB-backed), 646 web passed, zero FAIL. Migration 22 unblocks F4's DB gates (23/24).

Review: r-c2s1 PASS (review-secrets-slice-1.md) — migration 22 + ADR-0033, Z4 transcription exact, affected green. No follow-ups recorded.
