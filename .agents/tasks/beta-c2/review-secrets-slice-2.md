# Review: C2 secrets slice 2 core (8.8 `internal/secrets/`)

Verdict: PASS

- Tree: `../athena-erp-c2-s2-r`, detached at `57ad54b` (on `96acb15`), clean (`git status --short` empty apart from branch line).
- Diff `96acb15..57ad54b`: 5 files, all `internal/secrets/`: `secrets.go`, `provider.go`, `keys.go`, `redact.go`, `secrets_test.go` (+799). No `keys_test.go` (slice 3, correctly absent), no other lane's files, Z4 schedule untouched.
- Read: `brief-secrets.md` slice 2 (8.8), batch-2 tasks G8.8 + Requirement 8 criteria 8.13–8.15, ADR-0033, migration 22 schema in `sysmigrations.go`.

## 1. References CRUD + provider — sound

- `SaveRef`/`GetRef`/`DeleteRef` validate (empty/overlong/colon-bearing names refused; provider and locator empty/overlong refused) and match the migration 22 column widths (140/40/600). Upsert preserves the row id (test pins `updated.ID == ref.ID`); missing rows map to not-found on get and delete.
- `Provider` interface is minimal (`Name` + `Key`); the file provider is the only implementation, OpenBao/AWS stay extension points per ADR-0033.
- Adversarial probe (throwaway test, since deleted): `..`, `../escape`, `sub/../../escape`, `a/../../../etc/passwd`, `./../x`, `x/..`, `x/../..`, `/etc/passwd`, empty and NUL-byte locators are all refused; overlong locators (600- and 5000-char) are refused via the OS (`file name too long`). `SaveRef` does not pre-validate `providerRef` for traversal, but refusal happens at the `Key()` read point, so a hostile stored locator still cannot escape.

## 2. Key versions — lifecycle sound

- `AddVersion` requires positive versions; duplicates fail on the unique key. `ActiveVersion` is MAX over `active` only, so rotation (add new active, retire old) and the dual-active window both encrypt under the newest.
- `decrypt` refuses `revoked` before touching key material, decrypts `retired`, and errors on unknown versions — pinned by `TestDecryptRefusesRevokedAndUnknownVersions`, including malformed envelopes.
- Advisory (not verdict-changing): `SetVersionStatus` allows `revoked -> active`. Consider a one-way guard when slice 5 wires Rotate/Revoke.

## 3. Crypto — correct binding, unique nonces

- AES-256-GCM via `crypto/aes` + `cipher.NewGCM`; 32-byte keys enforced both at the file provider and at cipher build; no ECB/countersign anywhere.
- AAD is length-prefixed over Workspace + Record Type + Record ID + field + key name + version, so no two contexts serialize alike. Key names cannot contain `:`, and `RawURLEncoding` never emits it, so the 5-part envelope split is exact.
- `TestDecryptRejectsACopiedValue` genuinely exercises cross-context copies: one envelope decrypted under four altered contexts (other record, other field, other type, other workspace), all must fail, and errors must not contain the plaintext. Verified by reading, not by shape.
- Nonce uniqueness probe-verified: two encryptions of the same plaintext under the same context produce different nonces and different ciphertexts (`athsec1:workspace:1:TLUP8tkddf…` vs `…:8wBuBLYNPQ…`).

## 4. Redaction — no secret material observable

- No log calls in the package. Spans carry only `athena.secrets.encrypt/decrypt` names, `athena.outcome` ok/error, and `RecordError` of the audited errors.
- Audited every `Errorf`/attribute call: strings reference only names, locator paths, versions, statuses, or fixed text (`decryption failed`, `envelope is not a field-encryption value` — the envelope itself is never echoed). Plaintext, key bytes, nonces and ciphertext appear in none. `RedactValue`/`RedactString` are pinned by tests for slice 4/5 callers.
- Advisory: `Key()` read errors echo the full (unbounded) locator and wrap `os` errors containing the absolute server path. Paths are not secret material, but consider truncating the locator and stripping the root in errors.

## 5. Tests real, gates green

- Tests are DB-backed behavioral pins (real MariaDB via `testdb`, real temp-dir key files, real GCM round trips): CRUD round trip, validation rejects, version lifecycle, provider happy/refusal paths, encrypt/decrypt round trip with envelope-prefix and no-plaintext assertions, 4-way binding, retired/revoked/unknown/malformed decrypt, provider mismatch, error redaction.
- `affected --print`: Go `./internal/secrets`, no web. `affected` reuses the worker's green run on this exact clean commit (`57ad54b`, log `1124-101726-w-c2.log`: `ok athena-erp/internal/secrets 124.606s` — a real DB run, not skips). My own DB-backed nonce probe also passed in this worktree. No rerun needed per the one-green-run rule.
- `gofmt -l cmd internal` empty, `go build ./...` OK, `go vet ./...` OK. No proto changes (`make generate` N/A); `web/` untouched. `ON DUPLICATE KEY` matches existing precedent (`internal/ledger`, `internal/auth`); `uuid`/`otel` already in `go.mod` (no new dependency); no old-vocabulary terms in the new package.

## 6. Scope — clean

`internal/secrets/**` only; `keys_test.go` (Property 16, slice 3) absent as required; Z4 untouched; nothing outside the slice.

## Advisories for later slices (none blocking)

1. The file provider follows symlinks (probe-verified: an in-root symlink to an outside 32-byte file reads successfully). No privilege boundary is crossed — the root is operator-managed, and planting a symlink implies key-file write already — but consider `O_NOFOLLOW`/containment hardening when slice 5 wires the provider.
2. One-way version guard (`revoked -> active`), see §2.
3. Error path hygiene (locator echo + absolute path), see §4.
4. Key bytes are not zeroed after use (standard Go practice; outside the stated threat model).
