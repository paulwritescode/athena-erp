# Report: security settings RPC

**Status: Final. Verdict: PASS.**

Worktree: `/Users/iampaulking/projects/duchess/athena-erp-security-rpc`, branch `feat/security-rpc`.
Nothing committed, merged or pushed, no agents started. No `web/src` changes outside `web/src/gen/`.

## Choice: AuthService, not a new service

The settings live in `auth.Service`, the CLI path is auth-level, and two RPCs do not justify a
new service, so both calls went on `AuthService`.

## What changed

- `proto/athena/v1/auth.proto`: `GetSecuritySettings` / `SetSecuritySettings` on `AuthService`
  plus `SecuritySetting` (key, value, default, min, max, unit, description),
  `GetSecuritySettingsRequest/Response`, `SetSecuritySettingsRequest` (`map<string, int32>`,
  partial update) and `SetSecuritySettingsResponse` (full list after the change). Additively
  appended; nothing renumbered or removed.
- `internal/auth/principal.go`: new `settings:security` permission in `SystemPermissions`.
- `internal/auth/security.go`: bounds gain unit + one-line Desk description;
  `SecurityInfo` returns all eight settings sorted by key; `SetSecuritySettings` validates
  every entry first, then stores all rows plus the audit row in one transaction;
  `SetSecurity` (the CLI's function) delegates to it, so CLI and RPC share one write path;
  `settings:security` counts as build access so step-up is truly required.
- `internal/auth/accesslog.go`: new `security_settings_changed` event; the detail names each
  key with old and new values (policy numbers, not secrets), with the actor's username.
- `internal/server/gate.go`: Get is `policySession` (handler enforces `access:sessions` OR
  `settings:security`, following the ListSessions precedent); Set is `policySystem`
  `settings:security` plus a `destructiveProcedures` entry.
- `internal/server/auth_rpc.go`: both handlers (`securitySettingMessage` converter; Set runs
  under a `set_security_settings` span and returns the full list).
- `internal/server/session_policy_test.go`: `SetSecuritySettings` added to the destructive
  coverage verbs.
- Generated (`make generate` twice, checksums identical): `gen/athena/v1/auth.pb.go`,
  `gen/athena/v1/athenav1connect/auth.connect.go`, `web/src/gen/athena/v1/auth_pb.ts`.
- Docs: ADR-0026 addendum, decision-log row, README security line, overview permission list
  (also picked up the previously missing `settings:appearance`).

## Behavior notes

- `athena security set` now writes a `security_settings_changed` audit row too (operator actor,
  null user, same as other operator rows): same function, same audit. Existing CLI tests pass
  unchanged.
- Empty update map and unknown keys are `InvalidArgument`; a mixed valid+invalid batch stores
  nothing (tested).
- Step-up for Set follows B7 semantics (builders/root with stale proof); since
  `settings:security` counts as build access, every holder of the write permission is covered.
  A side effect of that choice: security-only admins also get the 15-minute idle policy and no
  remember-device, same as other administrators.

## Tests (new `internal/server/security_settings_test.go`)

- Get permissions: anonymous → Unauthenticated; no-permission user → PermissionDenied;
  `access:sessions` and `settings:security` holders → ok with all 8 settings and metadata.
- Set bounds + atomicity: unknown key, below-min, above-max, empty map → InvalidArgument;
  rejected batches change nothing; valid partial update applies and the audit row carries
  `key: old -> new` for each key with the actor's username.
- Set permission + step-up: builder without the permission → PermissionDenied; stale step-up
  → FailedPrecondition `step_up_required`; after StepUp → ok.
- CLI shared path: `SetSecurity` writes the same audit row with a null operator user.

## Checks (exact output)

- `gofmt -l cmd internal` → empty.
- `go build ./...` → exit 0. `go vet ./...` → exit 0.
- `make generate` twice → second run byte-identical (`auth.pb.go`, `auth.connect.go`,
  `auth_pb.ts` checksums match).
- New tests: 4/4 pass plus `TestDestructiveProcedureCoverage` and
  `TestEveryProcedureHasAPolicy`.
- `make test` Go: every package ok (`cmd/athena` 168.4s, `internal/auth` 303.0s,
  `internal/server` 246.4s, `internal/files` 111.2s, `internal/ledger` 147.9s,
  `internal/store` 6.6s, rest < 2s or no test files). No FAIL lines.
- Web: typecheck exit 0, 24 files / 348 tests passed.
- Only throwaway `test_athena_*` databases used.
