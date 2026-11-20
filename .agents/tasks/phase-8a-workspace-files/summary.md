# P8a summary: Workspace files

## What changed
- New `FileService` (ListFiles, GetFile, PutFile, MoveFile, DeleteFile, ListFileVersions) in `proto/athena/v1/files.proto`, with permissions `files:read` and `files:write` and a policy for every procedure.
- New `internal/files`: strict path rules (relative, lowercase, no hidden segments, no symlinks, allowed folders and extensions), `os.Root` disk access, atomic writes, SHA-256 hashes, optimistic `expected_hash`, 1 MiB and 1000-file limits, Record Type file validation with line numbers.
- Migration 10: `athena_files` and `athena_file_versions` (history survives rename and delete; outside edits are kept as `external` versions). Each change writes one Audit Log row with no content.
- CLI `athena files list|get|put|mv|rm|history`.
- Desk: `FilesPage.vue` (shadcn components, Sonner toasts, mapping tab), a Files item in Build, `web/src/filePaths.ts` sharing a test table with Go.
- ADR-0015 (Proposed), plus decision log, overview, model doc, README and AGENTS.md updates. No dependencies added.

## Verification
- `gofmt`, `go build`, `go vet` clean. `go test -count=1 ./...` passes in all packages (MariaDB 11.8).
- `make generate` is reproducible. Web typecheck, build and 115 tests pass.
- CLI and a 38-check headless Chrome run (light and dark) pass, including path refusals, stale-hash conflict and read-only user.
- Independent review: APPROVED, no findings (it read the recorded evidence and did not rerun the suites).

## Caveats
ADR-0015 is Proposed, so O-002 and D-009 stay open. The generator and conflict report are not built. External edits are detected on access, not by a watcher. `files:write` should be treated like `build`. Only MariaDB is tested.

## Next: P9, Starlark host
Embed `go.starlark.net` as the host for Rules: sandboxed, bounded execution (step and time limits, no I/O), calls back into Go only through permission-checked, transactional APIs, and Rule sources loaded from the `rules/` Workspace files.
