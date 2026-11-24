# Review: C2 secrets slice 5 — CLI + docs

Verdict: PASS

Tree: `../athena-erp-c2-s5-r`, detached at 518be6e (on eb04345), clean.
Scope diff is exactly 10 files: `cmd/athena/secrets.go` (new), `cmd/athena/main.go`
(F7 registry entry + usage), `cmd/athena/secrets_test.go` (new),
`internal/secrets/keys.go` (Rotate), `internal/secrets/provider.go` (WriteKey),
`internal/secrets/secrets_test.go`, plus AGENTS.md / README.md / overview.md /
decision-log.md rows. Zero out-of-scope files; no Desk, proto, server or ledger
changes.

1. CLI real — ran `save` and `rotate` against throwaway `test_athena_review_s5`
   (dropped after) with a throwaway key dir: bootstrap prints
   `rotated workspace to version 1 (first version)`, second run retires
   (`version 2 (version 1 retired)`); key files land 0600 (`100600`), 32 bytes,
   byte-identical to `--from`. Crash-retry verified end to end: pre-existing v4
   file with identical bytes rotates cleanly to v4; pre-existing v5 with
   different bytes fails loud (`already exists with different bytes`), writes no
   v5 row and leaves the file untouched. Short/ghost/missing-dir/foreign-provider
   paths all exit 1 with clean errors and no partial rows. `save --json` and
   `rotate --json` outputs parse as valid JSON.
2. Custody — full dumps of `athena_secret_refs` (id/name/provider/locator) and
   `athena_key_versions` (key/version/status) show names only, no material.
   `material` flows only into length checks, byte comparison, `WriteFile` and
   `Rotate`→`WriteKey`; no format string, log, span or SQL carries it. Every
   error in the new paths names the locator/key only (read `Key`/`WriteKey`/
   `Rotate` in full). Key dir is operator-held via `--key-dir`/`ATHENA_SECRETS_DIR`,
   refused when empty, documented as outside the Workspace and its backups.
3. Registry — `secrets` is in the F7 command table and top-level usage (once);
   `secrets`, `secrets --help`, `events`, `jobs`, `post`, `doctor`, `rules`
   `--help` all dispatch. main.go diff is additive only.
4. Docs — all four rows say "references only" (AGENTS.md, README.md,
   overview.md, decision-log ADR-0033). Spot-checked against `--help` and
   observed behavior: (a) `save --key name --ref locator` registers a reference,
   never a value — matches usage and `saved reference workspace
   (file:workspace)`; (b) a first rotation bootstraps version 1 — matches usage
   and observed output.
5. Generate — worker's claim verified, not trusted: with node v26.7.0 on PATH,
   `make generate` exits 0 and `git status` stays empty. No drift; the
   missing-node failure is purely environmental.
6. Gates — `affected` reuses w-c2's green `go test ./cmd/athena
   ./internal/secrets` on clean 518be6e (log 1124-171907-w-c2.log); since the
   secrets half was a Go-cache hit, I re-ran the new tests fresh with the DB:
   `TestRotate`, `TestWriteKeyRefusesClobber`, `TestSecretsCommands` all PASS
   (8–9 s each, real MariaDB runs). `gofmt -l`, `go vet`, `go build` clean.
7. Scope — PASS (see file list above).

Notes (not fails, possible follow-ups): concurrent same-key rotations can
TOCTOU in WriteKey (last-writer-wins on disk, one tx commits) — loud, not
silent, and out of scope for a single-operator beta CLI; O_EXCL create or a
file lock would close it. A key with versions but no active version refuses
rotation until an operator reactivates one (only reachable via manual status
edits). An orphan key file with different bytes blocks that version number
until removed (loud error). Outside-workspace placement of the key dir is
documented operator responsibility, not CLI-enforced.
