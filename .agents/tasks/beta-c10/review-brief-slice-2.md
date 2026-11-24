# Review brief: C10 slice 2 provider inbox (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c10-s2-r, detached at 47dd6e4 (on f1fb4a0). Verify clean + diff is slice-2 scope only: internal/integrations inbox + secrets version helpers + tests (+ README row).
Read: .agents/tasks/beta-c10/brief.md slice 2 (15.2: HMAC key window, dedupe, evidence, Operation_ID mapping, secrets for keys).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. HMAC real: SHA256 verify with 72h previous-key window — verify the window logic (old key accepted inside, rejected outside) with a test; merchant/amount/identity claims validated, not trusted.
2. Dedupe real: PK-atomic dedupe via bus inbox, key-independent — verify double-delivery collapses (run or read the test).
3. Operation_ID deterministic: same event maps to same ID across runs (verify mapping is pure, no randomness/time).
4. Evidence: provider.received events recorded with enough to audit (verify one); no mark-paid path exists by construction (verify absence — grep).
5. Secrets: ActiveVersionInfo/KeyForVersion correct; key material never in logs/errors/spans (read one error path).
6. Carry claim: worker carried the affected green across a rebase (files byte-identical, README delta main-side) — verify the carry (empty diff of slice files vs 54430ee) and reuse-or-rerun affected (cmd/athena + integrations + secrets) through the main-repo suite.
7. Scope: the files above ONLY (no migration 25 file — still held; no proto/server/CLI/Desk; fail if any).

Report: say master "C10 SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c10/review-slice-2.md with the verdict line.
