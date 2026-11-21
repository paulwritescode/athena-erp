# Handover: B5 Workspace identity and logo, slice 1 (server + CLI)

Paused for laptop restart. No commit made. No temp servers running, no temp
databases created, nothing to stop.

## Location

- Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b5`
- Branch: `feat/b5-identity` (based on main incl. B6 merge b18f915 + tick c9378f3)
- Brief: `/Users/iampaulking/projects/duchess/athena-erp/.agents/tasks/beta-b5/brief.md`
- Scope now: slice 1 only (server + CLI). Slice 2 (Desk) not started.

## Done and verified

- Root logo copies removed (`athena-black-logo`, `athena-white-logo` deleted
  from worktree root). Verified byte-identical (md5) to B2's
  `web/src/assets/brand/athena-black.png` / `athena-white.png`, so reused,
  not duplicated.
- `proto/athena/v1/workspace.proto`: `Identity`, `LogoUpload`,
  `GetIdentity`/`SetIdentity` RPCs. `buf generate` run; Go + TS generated
  code updated (`gen/athena/v1/workspace.pb.go`,
  `gen/athena/v1/athenav1connect/workspace.connect.go`,
  `web/src/gen/athena/v1/workspace_pb.ts`).
- `internal/auth/principal.go`: `PermissionWorkspace = "settings:workspace"`,
  added to `SystemPermissions` (role validation picks it up automatically;
  deliberately NOT in `BuildAccess`, mirroring `settings:appearance`).
- `internal/server/gate.go`: `GetIdentity` public, `SetIdentity`
  policySystem `settings:workspace`.
- `internal/files` (`path.go`, `validate.go`, `store.go`): `brand/` folder,
  `.png`/`.svg` extensions, `AllowBinary` (PNG under `brand/` skips the
  text check in `Validate` and `read`).
- `internal/brand/` (new): `logo.go` (ValidateLogo: ext/size/dims/full PNG
  decode), `svg.go` (SanitizeSVG: drops script/style/foreignObject/filter/
  unknown elements, unwraps `a`, strips on*/unsafe href/src, filters style
  declarations, drops comments/doctype/procinsts, requires `<svg>` root with
  width/height or viewBox), `favicon.go` (area-average `Favicon`,
  pre-rendered embedded `mark32.png`/`mark180.png` via `MarkFavicon`),
  `urls.go` (`LogoURL`, `FaviconURL`, `MarkHash`).
- `internal/brand/brand_test.go`: ext/size/corrupt/dims rejections, 14 SVG
  sanitize cases, SVG dims, favicon determinism/dims, mark drift pin against
  `web/src/assets/brand/athena-black.png`.
- `internal/ledger/identity.go` (new): `brand.identity` JSON doc in the G5
  Settings chain (`GetIdentityDoc`/`SaveIdentityDoc`), mirroring
  `appearance.go`.
- Verified: `go build ./internal/brand/`, `go vet`, `gofmt -l` clean,
  `go test ./internal/brand/` ok. Full `go build ./...` NOT yet run (server
  RPCs unwritten, so nothing references the new proto/service surface yet).

## In progress (not written yet)

- `internal/server/workspace.go`: extend `workspaceService` with display-name
  default + `files.Store` (`newWorkspaceService`, mirror `newFileService`);
  `GetIdentity` (public, builds `v1.Identity` from doc + `brand.*URL`);
  `SetIdentity` (requireSystem `settings:workspace`, trim + 1..80 rune name,
  `brand.ValidateLogo` both uploads before any write, `files.Put` with
  `Expect` from current disk hash, `SaveIdentityDoc`, then delete stale
  ext/cleared paths; map files errors with `fileError`).
- `internal/server/kernel.go` + `server.go`: construct via
  `newWorkspaceService` (mountRPC uses `filesWorkspace(config)`); close the
  workspace store in `Kernel.Close`.
- `GET /brand/` handler (new file in `internal/server`, mount in `Serve`):
  `logo-<64hex>.<png|svg>` (match stored hash+ext, verify disk bytes hash,
  long immutable cache, nosniff, `Content-Security-Policy: sandbox` for SVG)
  and `favicon-<hash|mark>-<32|180>.png` (mark bytes embedded, else resize
  current light PNG logo; 404 otherwise). Mark favicons must work with nil
  db/files.
- `internal/server/files.go`: FileService `List` skips `brand/`;
  Put/Move/Delete touching `brand/` refused (PermissionDenied, "managed
  through the Workspace identity"); Get/Versions allowed.
- CLI `cmd/athena/appearance.go`: extend `workspaceCommand` with `identity
  show|set --name --logo-light --logo-dark` (read files, call kernel
  Workspace RPCs); update usage + `main.go` help text (check
  `appearance_test.go` expectations first). `cmd/athena/main.go` `newCommand`:
  `--logo` flag → after `Create`, `OpenKernel` + identity save (name from
  manifest + logo); logo-less `new` writes nothing (lazy default).
- Go tests: identity RPC roundtrip (testdb + temp files dir), public Get
  shape, permission denial, name validation, replace/remove, `/brand/`
  handler headers/404s via httptest, FileService brand refusals, CLI tests.
- Docs: `docs/architecture/overview.md` services row (identity), README only
  if it enumerates workspace commands. No ADR number assigned; design §3
  suggests one for the new public surface — flag to master.

## Key decisions already taken

- Favicons render at serve time from the current light PNG logo; SVG logos
  and unset logos fall back to the pre-rendered Athena mark (no SVG
  rasterizer available; brief's "generate favicons" honored for PNG).
- `go:embed` cannot use `..`, so the mark favicons are pre-rendered
  (`mark32.png` 910 B, `mark180.png` 8 KB) with bytes pinned by test to the
  Desk PNG; generator was `tmp-brandgen/`, already removed.
- Display name defaults to the manifest name at read time (no creation-time
  write, avoids migration ordering); `new --logo` is the only creation path
  that writes identity.
- Puts → settings save → deletes ordering, so a crash never leaves settings
  pointing at missing files.

## Commands to verify

```sh
cd /Users/iampaulking/projects/duchess/athena-erp-b5
export PATH="$HOME/.nvm/versions/node/v26.7.0/bin:/opt/homebrew/bin:$PATH"
gofmt -l cmd internal            # empty
go build ./... && go vet ./...
make test                        # sets ATHENA_TEST_DATABASE_URL
make generate                    # twice, no diff second time
CI=true pnpm --dir web run typecheck && CI=true pnpm --dir web run test
CI=true pnpm --dir web run build # then: git checkout -- internal/webui/dist/.gitkeep
```

## Fresh-session notes

- Disk was 99% full (`/` 141 Mi free); ran `pnpm store prune` (safe,
  regenerable cache) which freed 5.4 GB. If `make generate`/`pnpm install`
  fails with "disk is full" again, prune first. Do NOT delete user data in
  `~/Library/Containers`, `~/Library/Caches`, or other sessions' `/tmp`
  files to free space.
- `web/node_modules` in this worktree is installed (via Makefile
  `web-deps`, store in `.cache/pnpm`). The old `athena-erp-b6-words`
  worktree is gone (merged); do not reference it.
- Pattern references: `internal/server/workspace.go` (appearance RPCs),
  `internal/ledger/appearance.go` (settings upsert), `cmd/athena/
  appearance.go` (CLI), `internal/server/files.go` + `fileError`.
- Master workflow: slice 1 merges before slice 2 starts; report with
  `hc send t6 "B5 SLICE 1: PASS or BLOCKED"`. Do not commit, merge, push,
  or start agents.
