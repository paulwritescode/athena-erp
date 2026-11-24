# Review brief: C2 secrets slice 3 Property 16 (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c2-s3-r, detached at bcb44e7 (slice 3 on unmerged slice-2 57ad54b; review ONLY the slice-3 delta 57ad54b..bcb44e7 — slice 2 is a separate review). Verify clean.
Read: .agents/tasks/beta-c2/brief-secrets.md slice 3 (8.9); batch-2 requirements 8.14–8.15; internal/secrets (the slice-2 code under test).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Property REAL (rapid, 100 iterations claimed — verify by running): round trip + 4-axis binding + fresh-nonce. Binding axes must be the REAL context (Workspace+type+id+field+key) with per-axis negative cases; nonce freshness statistically meaningful (no fixed nonces).
2. Mutation evidence: worker claims empty-AAD fails + corpus removed — verify the mutation was genuine (re-run the property with a broken binding yourself if cheap, or confirm the test structure forces it).
3. Tests green + affected green (reuse-or-rerun secrets scope through suite); gofmt/vet clean.
4. Scope: internal/secrets/keys_test.go ONLY (no product-code changes in slice 3 — fail if any); no other lane's files. Validates Req 8.14/8.15 (say which criterion each property arm covers).

Report: say master "C2 SECRETS SLICE 3 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c2/review-secrets-slice-3.md with the verdict line.
