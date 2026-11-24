# Summary: C2 secrets slice 2 (core)

Merged 2026-11-24 as 4616729 (cherry-pick of 57ad54b onto post-F3s1 main), 1 commit, 5 files.

Scope: `internal/secrets/` (new): references CRUD, provider interface + local file provider (traversal/length refused), key versions (active/retired/revoked), AES-256-GCM envelopes bound to Workspace+type+id+field+key (cross-context copies fail), redaction (errors/spans carry no material). No wiring into other packages yet (slice 3+).

Evidence: worker affected green (secrets scope); merge gate: first run RED on environment (npm registry flaked mid-install → corrupt node_modules → vitest startup error; all Go green incl. secrets 259s) → node_modules rebuilt + shared-store rule recorded → retry GREEN 32s (Go cached from the real DB-backed run, web 646 fresh). Merged onto post-F3s1 main under the Stylus disjoint-additive ruling (verified: secrets-only delta, zero shared files, clean pick, build+vet green on the combined tip).

Review: r-c2s2 PASS (review-secrets-slice-2.md) — refs/provider/versions/binding/redaction verified, 4 advisories (non-blocking, in review file).
