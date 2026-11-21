# Brief: B9 slice 1, form layout and Image field on the server (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b9-server, branch feat/b9-form-layout-server. Server only (plus generated web/src/gen). Merge-small: this slice merges on its own when it works end to end through the CLI.
Read: AGENTS.md; requirements R9; tasks.md B9; design-system sections 9 and 19; ADR-0013 (CEL); ADR-0015 (Workspace files); internal/server/record_types.go (journaled publisher); internal/files.
Numbers: migration 17 if a table is needed (prefer none, by storing the layout with the definition); ADR-0027 "Record Type form layout and Image fields" (Proposed).

1. Layout on RecordType (new proto fields, additive): sections (title, optional description, columns 1 or 2), items in order (a field key with width full or half and help text; or a static text block; or a static image block that references a Workspace file; or an art block referencing an art-registry key, stored as a string the Desk validates), and an optional per-section visibility CEL expression checked with internal/expr.
2. Validation on save: every field key exists; each field appears at most once; fields not placed are appended to the last section, so nothing is lost; CEL expressions type-check; images reference existing files of an image type; size limits (at most 50 sections and 500 items).
3. A layout-only change saves through the journaled publisher without DDL, bumps the revision, and is recorded in History. It never needs confirm_data_loss.
4. Image field type: stores a Workspace file reference (path plus hash). Upload uses the existing FileService (add an image upload path if FileService is text-only: PNG, JPEG or WebP up to 5 MB, content sniffed rather than trusted from the name, EXIF stripped, a 256 px thumbnail generated in Go with the standard image packages, served same-origin). Alt text is required and stored with the value. RecordService accepts and returns the reference. Deleting a Record keeps the file version history (files already keep versions).
5. CLI: `athena record-types save --file` accepts the layout. Add `athena files put-image`, or extend `put`, for images.
6. Tests: layout validation, round trip, layout-only save without DDL, Image field create, update and clear, upload validation (wrong type, too big, renamed executable), and the thumbnail.
Checks: gofmt -l, go build, go vet, make test, make generate twice with no diff. Do not commit, merge, push or start agents.
Report: .agents/tasks/beta-b9/server-report.md, then hc send t6 "B9 SERVER: PASS or BLOCKED".
