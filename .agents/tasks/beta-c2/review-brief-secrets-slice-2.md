# Review brief: C2 secrets slice 2 core (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-c2-s2-r, detached at 57ad54b (on 96acb15). Verify clean + diff is slice-2 scope only: `internal/secrets/` (new) + its tests.
Read: .agents/tasks/beta-c2/brief-secrets.md slice 2 (8.8); batch-2 requirements 8.13–8.15; ADR-0033.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. References CRUD correct; provider interface clean; local file provider refuses traversal + overlong paths (try to break it by reading: `..`, absolute, long, symlink?).
2. Key versions: active/retired/revoked lifecycle sound (revoked never decrypts, retired decrypts-only, rotation path sane).
3. Crypto: AES-256-GCM with authenticated context bound to Workspace+type+id+field+key; copies across contexts FAIL (verify the binding test actually exercises a cross-context copy); nonces unique (no reuse); no ECB/countersign footguns.
4. Redaction: errors + spans carry NO secret material (grep the package for leak paths; check error strings + span attributes).
5. Tests REAL (refs/provider/versions/envelope/redaction pinned, not shapes); affected green (reuse-or-rerun secrets scope through suite); gofmt/build/vet clean.
6. Scope: internal/secrets/** ONLY (keys_test.go Property 16 is slice 3 — fail if present here... note: worker says untouched; verify); no other lane's files; Z4 schedule untouched (slice 1 owns it).

Report: say master "C2 SECRETS SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-c2/review-secrets-slice-2.md with the verdict line.
