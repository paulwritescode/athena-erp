# Report: B9 slice 1 server — form layout and Image field

**Status: Final. Verdict: PASS.**

Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b9-server`, branch
`feat/b9-form-layout-server` (from main `5fe3b69`). Nothing committed, merged
or pushed, no agents started. No migration: 17 stays unused (layout lives in
the definition JSON; thumbnails generate on read).

## What was built

- **Layout proto** (`record_types.proto`): `FormLayout`/`FormSection`/
  `FormLayoutItem` (oneof field/text/image/art) plus item messages, on
  `RecordType.layout` (field 21, additive). Widths and kinds are validated
  strings, matching the existing kind/class convention.
- **Layout storage/validation** (`internal/server/layout.go`, new): stored
  with the definition; unplaced fields append to the last section at full
  width on save; empty layout normalizes to none. Validation: ≤50 sections,
  ≤500 blocks, columns strictly 1|2, widths "full"/"half"/"", field keys
  exist and appear at most once, text 1–10000 chars, image blocks name
  recorded files under `images/`, art keys non-empty ≤200 (Desk-validated),
  title ≤255, description/help ≤2000. Visibility is CEL returning boolean
  over the Record's fields (≤1000 chars, `today()` allowed, display-only
  like `depends_on`) type-checked through the existing expression
  environment. A layout-only save plans zero steps, needs no confirmation,
  bumps the revision and journals normally.
- **Layout permissions**: `visibleDefinition` strips layout blocks of
  fields above the caller's level and drops emptied sections, so filtered
  definitions never name hidden fields.
- **Image field type**: `Image` maps to a JSON column (both dialects);
  `ImageValue{path,hash,alt_text}` rides a new `FieldValue.image_value`
  case; values validate shape only (images/ path, 64-hex hash, alt text
  1–1000 chars) and store as JSON; reads parse back; audit renders the
  path; formulas cannot target Image fields; unique/index/pattern/numeric
  attributes reject it through the existing type lists. Deleting a Record
  never touches file versions (no FK, no cleanup code).
- **Images folder + upload**: new `images/` folder scoped both ways
  (image extensions only there, only images there); new `PutImage` RPC
  (files:write, step-up like PutFile) validating 5 MiB, sniffed PNG/JPEG/
  WebP with name matching the sniffed bytes, and decodability; content is
  cleaned (EXIF orientation applied to JPEG with re-encode, else lossless
  metadata surgery; PNG chunk surgery; WebP RIFF surgery) and stored
  through the existing versioned/audited/concurrency-checked file path.
  `GetFile` gains `thumbnail` returning an on-read 256 px thumbnail
  (long edge, never upscaled; same format except WebP→PNG).
- **Dependency**: `golang.org/x/image v0.48.0` (new, pure Go, no cgo) for
  WebP decode and high-quality scaling only; everything else is stdlib.
  Documented in ADR-0027 with rationale.
- **CLI**: `athena files put-image` (file or stdin, same guards as put),
  `records create --values` accepts image objects, `record-types save
  --file` accepts layout JSON via protojson (verified by test).
- **Docs**: ADR-0027 (Proposed), decision-log row, docs/README row, README
  command updates. No ADR-0028 needed (no migration, no second decision).

## Numbers used

Migration 17: not needed (see above). ADR-0027 only. The filesystem
flaked intermittently during this task (fresh-worktree ENOENT bursts);
worked around with retries, no content affected.

## Tests (all new, all passing)

- `internal/files/image_test.go`: sniff table, validation table (wrong
  type, too big, renamed executable, name/type mismatch, truncated),
  PNG text-strip with pixel equality, clean PNG/JPEG byte-identity,
  EXIF orientation-6 rotation with pixel proof, WebP chunk surgery,
  thumbnail dims/formats/passthrough, all 8 orientations.
- `internal/files/files_test.go`: PutImage round trip (dims, versions,
  stale guard, folder scoping both ways).
- `internal/server/layout_test.go`: save round trip with normalization,
  14 validation errors, layout-only save (zero-step plan, revision bump,
  unchanged columns), permission-filtered layout.
- `internal/server/image_test.go`: value round trip, 5 validation errors,
  missing-file read, delete-keeps-versions, audit rendering, formula
  rejection.
- `internal/server/files_test.go`: PutImage over RPC (round trip,
  thumbnail decode+dims, thumbnail-on-text and bad-upload rejections),
  folders assertion updated (4→5), denial count 18→21.
- `cmd/athena`: put-image round trip + rejections, layout JSON
  save/show, records create with image object + missing-alt refusal.

## Checks

- `gofmt -l cmd internal` → empty.
- `go build ./...` → exit 0. `go vet ./...` → exit 0.
- `make generate` 3×, checksums identical across runs (Go + TS).
- `make test` → exit 0: every Go package ok (cmd 108 s, auth 320 s,
  files 111 s, ledger 146 s, server 262 s, store forced fresh 1.6 s),
  web 397/397 passed. Only throwaway `test_athena_*` databases used.

## For the desk slice

- Mirror Image value rules in `fieldRules.ts` + `field_rules.json`
  (server deliberately left the string-only table shape untouched).
- Thumbnails come from `GetFile(thumbnail=true)`; originals from `GetFile`.
- Layout `width: ""` means full; sections arrive with unplaced fields
  already appended.
