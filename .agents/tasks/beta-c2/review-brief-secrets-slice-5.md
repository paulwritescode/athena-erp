# Review brief: C2 secrets slice 5 CLI + docs (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c2-s5-r, detached at 518be6e (on eb04345). Verify clean + diff is slice-5 scope only: cmd/athena/secrets.go (new) + secrets.Rotate + FileProvider.WriteKey + tests + docs rows.
Read: .agents/tasks/beta-c2/brief-secrets.md slice 5 (CLI + docs) + Z4 custody rule + ADR-0033.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. CLI real: `athena secrets save|rotate` work against a throwaway dir (run both; rotation bootstrap/retire/tx correct; 0600 key files verified on disk; idempotent retry verified by running twice; --json valid).
2. Custody: ATHENA_SECRETS_DIR outside the Workspace; key material NEVER in DB/backups/logs (grep + read one error path).
3. Registry: secrets registered in the F7 command table (post-rebase resolution) — verify dispatch works and no other command drifted.
4. Docs: overview/README/AGENTS/log rows say "references only" and match the built CLI (spot-check 2 claims against --help).
5. Generate: worker claims "stable by construction, make generate fails environmentally on missing node" — VERIFY: run make generate with node on PATH (or confirm the failure is environmental, not a drift); fail if generated files drift.
6. Gates: affected green (reuse-or-rerun cmd/athena + secrets through the main-repo suite on 518be6e); gofmt/build/vet clean.
7. Scope: the files above ONLY (no Desk per brief — fail if any; no proto/server/ledger changes).

Report: say master "C2 SECRETS SLICE 5 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c2/review-secrets-slice-5.md with the verdict line.
