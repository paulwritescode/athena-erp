# Review: C2 secrets slice 3 Property 16 (8.9 `internal/secrets/keys_test.go`)

Verdict: PASS

- Tree: `../athena-erp-c2-s3-r`, detached at `bcb44e7` (on unmerged slice-2 `57ad54b`), clean (`git status --short` empty at start and end).
- Diff `57ad54b..bcb44e7`: 1 file, `internal/secrets/keys_test.go` (+108). No product-code changes, no other lane's files, Z4 untouched.
- Read: `brief-secrets.md` slice 3 (8.9), batch-2 tasks G8.9 + design.md Property 16 + Requirement 8 criteria 8.14/8.15, slice-2 `internal/secrets` under test.

## 1. Property REAL — round trip + 4-axis binding + fresh nonce, 100 iterations run

- `TestFieldEncryptionRoundTripAndBinding` is a genuine rapid property over drawn keys (fresh random 32-byte material per version), drawn contexts (Workspace, Record Type, Record ID, field, key name), drawn plaintexts (0–64 bytes, empty included), drawn version counts (1–3), and a drawn moved axis per iteration. Not an example list.
- Round trip: decrypt under the exact context returns the identical plaintext (`string(back) != string(plaintext)` fails).
- Binding: one randomly drawn axis (0–3) is moved to a guaranteed-different value per iteration and decrypt must fail. Over 100 iterations each axis is hit ~25 times (P(missing an axis) = 4·(3/4)^100 ≈ 0).
- Fresh nonce: two seals of the same plaintext/context must differ as full envelopes (envelopes differ iff the crypto/rand nonce differs). 100 independent fresh-nonce checks, no fixed nonces anywhere.
- Iteration count verified by running: `[rapid] OK, passed 100 tests` (RAPID_CHECKS unset, rapid v1.3.0 default 100 confirmed in module source). My direct DB-backed run: PASS, 48.5s.

## 2. Binding axes are the real context; key-axis note (not blocking)

- The 4 caller-supplied decrypt axes (Workspace, Record Type, Record ID, field) each have negative cases with realistic drawn values. Decrypt takes KeyName from the envelope (`keys.go:200` overwrites `c.KeyName`), ignoring the caller's, so a key-axis negative via caller context is impossible by API design; the key axis is exercised positively (drawn key names/material/versions per iteration).
- This matches the spec exactly: design.md Property 16 says "a different Workspace, Record, or field context fails", Req 8.14 binds "Workspace, Record and field", Req 8.15 is "copied into another Record". The brief's "+key" parenthetical is satisfied in the only way the API allows.
- Optional hardening for a later slice (not required by Property 16): an envelope key-swap arm (register key B, rewrite the envelope's key part A→B, decrypt must fail) to pin key separation. Key binding itself is enforced by wrong-key-material GCM failure today.

## 3. Mutation evidence — genuine, re-verified by the reviewer

- Worker claim (empty-AAD fails, corpus removed): confirmed both. No `testdata/rapid` corpus in the tree or the delta.
- I re-ran the mutation myself: patched `contextBytes` to return nil, ran the property (`-rapid.checks=10`) → FAIL on iteration 0: `decrypt under a moved context succeeded with "", want a binding failure`. Reverted via `git checkout`, removed the shrink corpus, `git status` empty after. The test structure forces binding: any AAD break makes moved-context decrypt succeed, which is a hard `t.Fatalf`.

## 4. Tests green + affected green, gofmt/vet clean

- `affected --print`: Go `./internal/secrets`, no web. `affected` reuses the worker's green run on this exact clean commit (`bcb44e7`, log `1124-102131-w-c2.log`: `ok athena-erp/internal/secrets 47.310s` — full package, no FAIL/SKIP). No rerun needed per the one-green-run rule; my own direct property run corroborates it.
- `gofmt -l internal/secrets/` empty, `go vet ./internal/secrets/` clean. No proto changes (`make generate` N/A); `web/` untouched.

## 5. Scope — clean

- `internal/secrets/keys_test.go` ONLY (+108, test code). Zero product-code changes — slice 3 is pure test as briefed. Nothing outside the slice.

## 6. Requirement coverage per property arm

- Round-trip arm → Req 8.14: the key service (algorithm, key reference, nonce, authenticated context) yields decryptable values.
- 4-axis moved-context negatives → Req 8.15 (a value copied into another Record/field/type/Workspace fails to decrypt) + Req 8.14 (authenticated context binding).
- Fresh-nonce arm (two seals differ) → Req 8.14 (nonce).
