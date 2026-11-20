# P8a notes: Workspace files (iteration 1)
Date: 2026-11-20. Branch `main`, base 16a58f1. Nothing committed (the approval step commits).

## What was built
- `proto/athena/v1/files.proto`: `FileService` with ListFiles, GetFile (also the download; `revision` reads a recorded version), PutFile (create, replace, upload, with `expected_hash`), MoveFile, DeleteFile, ListFileVersions. No separate DownloadFile RPC: it would only wrap GetFile.
- `internal/files` (new): `CleanPath` (relative, lowercase `a-z0-9_.-`, no dot-leading segment, max 8 segments and 240 bytes, folders `generated packs record_types rules`, extensions `.go .json .md .star .txt .yaml .yml`), `Store` (all disk access through `os.Root`, no symbolic links at any level, atomic temp file plus rename, SHA-256 hash, optimistic `Expect`), validation (UTF-8 text, 1 MiB, 1000 files, YAML and JSON parse, Record Type file format of spec 24 with line numbers and `record_type` equal to the file name), `Backing` (file to Record Type, Rule, Report or generated source).
- History: migration 10 `workspace_files` (`athena_files`, `athena_file_versions` with content per revision). History follows renames and survives deletes. Content found changed outside Athena is kept as an `external` version before the next change.
- Audit: each create, update, move, delete appends an Audit Log row (`athena:file`, file id as record id, `file_*` action, changes = path, hashes, size; actor, source, request id). Never content.
- `internal/server/files.go`: handler (also used by the CLI through `Kernel.Files`), error mapping (stale = aborted, exists = already exists, invalid = invalid argument), spans with operation, request id, hashed path id, byte and file counts, outcome. `Workspace.Root` added to `server.Workspace` so the server knows the directory; `athena serve` has none and FileService answers "no Workspace directory".
- Permissions: new system permissions `files:read` and `files:write` (`auth.SystemPermissions`), policies in `gate.go` for all six procedures. Denials go to the Access Log. Write does not include read.
- CLI `athena files list|get|put|mv|rm|history` (`cmd/athena/files.go`), flags `--workspace`, `--json`; no `--addr` (no server starts, same as the other kernel commands). `put` fetches the current hash unless `--new` or `--expected-hash` is given; `rm` needs `--yes`.
- Desk: `web/src/pages/FilesPage.vue` (shadcn Tabs, Table, Dialog, AlertDialog, Textarea, Badge; Sonner toasts; tokens only), `Files` nav item in Build, `fileClient` in `api.ts`, `web/src/filePaths.ts` mirroring `CleanPath` (shared table `internal/files/testdata/paths.json` in both test suites). `fieldRules.ts` unchanged (no field rule changed). Users with `files:read` but not `build` can open Build and see only Files. The mapping tab lists each file with what it backs and whether a saved Record Type exists, plus Record Types with no file.
- Docs: ADR-0015 (Proposed), decision log (ADR-0015 row, O-002 updated, D-009 left as is until acceptance), overview, model doc, README, docs/README, AGENTS.md (Build list, API list).
- Dependencies: none added. `os.Root` is in the standard library (go 1.26), `gopkg.in/yaml.v3` was already direct.

## Invariant tests
`internal/files/files_test.go`: shared path table; every operation rejects absolute, `..`, backslash, encoded, uppercase and hidden paths and writes nothing; symbolic links (file, folder, folder inside the Workspace) are refused everywhere and the target is untouched; `.env*`, `.git`, `.athena`, `athena.yaml`, `storage/` and temporary files are never listed or readable; size and type limits; Record Type validation with line numbers; stale hash fails and leaves the file unchanged; an interrupted write (fault between temp file and rename) leaves the old content and no temp file; history preserved across edit, outside edit, move, delete, recreate; one audit row per change with path, hash, actor, no content, chain still verifies.
`internal/server/files_test.go`: no permission means deny and the denial is logged (18 denials), write-only cannot read, read-only cannot write, signed out refused; API round trip with backing and mapping; spans and log carry no content or file name; no Workspace directory.
`cmd/athena/files_test.go`: all six subcommands against a throwaway Workspace. `TestEveryProcedureHasAPolicy` covers the new procedures.

## Verification (exact commands, repository root)
| Command | Result |
| --- | --- |
| `gofmt -l cmd internal gen` | no output |
| `go build ./...`, `go vet ./...` | ok |
| `go test -count=1 ./...` (ATHENA_TEST_DATABASE_URL already in the environment, MariaDB 11.8; `.env.local` not read or printed) | all packages ok: cmd/athena 29.8s, internal/files 32.9s, internal/server 200.2s, auth, expr, lifecycle, numbering, store, types, workspace |
| `make generate` twice, comparing `git status`, `git diff --stat` and sha of generated files | identical |
| `CI=true pnpm --dir web run typecheck`, `build`, `test` | ok; 115 tests (adds filePaths cases) |
| `make restart` (brew mariadb@11.8) | `/healthz` returns `{"status":"ok","database":"Connected"}`; `internal/webui/dist/assets/index-DznoOS4R.js` and `.css` rebuilt at 01:25 |
| CLI on a throwaway Workspace (`/tmp/p8a`, database `test_athena_p8a`): `put --new`, `list`, `put ../escape.yaml`, `history` | saved r1, listed with `record_type widget (saved)`, escaping path refused (rc 1), history shows `created` |
| Browser, headless Chrome over CDP (`/tmp/p8a/drive.mjs`, `athena run` on 127.0.0.1:3010 against the throwaway Workspace), light and dark | 38 of 38 checks: theme, sign-in, Files page lists and opens a file, save toast, stale-hash conflict toast and file unchanged, discard reloads the outside edit, invalid Record Type shows `line 4`, `../x.yaml`, `/etc/passwd`, `record_types/.env` refused in the dialog, create, history, mapping, delete toast, `files:read`-only user sees a read-only editor and no New, Upload or Save. Screenshots in `/tmp/p8a/*.png` |
The throwaway database was dropped, its server stopped and the server log held no passphrase. The dev server is running again on 127.0.0.1:3000.

## Changed files
Modified: AGENTS.md, README.md, cmd/athena/main.go, migrate.go, migrate_test.go, docs/README.md, docs/architecture/overview.md, record-type-and-pack-model.md, docs/decisions/decision-log.md, gen/athena/v1/auth.pb.go, internal/auth/principal.go, internal/server/access_test.go, gate.go, kernel.go, server.go, sysmigrations.go, proto/athena/v1/auth.proto (comment), web/src/App.vue, api.ts, style.css, types.ts, web/src/gen/athena/v1/auth_pb.ts (comment).
New: cmd/athena/files.go, files_test.go, docs/decisions/adr-0015-workspace-files-and-generated-source.md, gen/athena/v1/files.pb.go, athenav1connect/files.connect.go, internal/files/{path,validate,store,export_test,files_test}.go, internal/files/testdata/paths.json, internal/server/files.go, files_test.go, proto/athena/v1/files.proto, web/src/filePaths.ts, filePaths.test.ts, gen/athena/v1/files_pb.ts, pages/FilesPage.vue. `internal/webui/dist/.gitkeep` was deleted by the Vite build and restored.

## Caveats
- ADR-0015 is Proposed. The generator, the publisher's recorded hashes and the conflict report are not built; O-002 and D-009 stay open until the user accepts it.
- External edits are noticed on list, read or the next change, not by a watcher. They produce an `external` version but no Audit Log row (author unknown).
- `files:write` lets a user change Rule sources and generated Go that later phases run or compile. Treat it like `build`.
- The desk opens in View after sign-in (existing behaviour); click Build to reach Files.
- MariaDB is the only tested database; PostgreSQL is untested.
