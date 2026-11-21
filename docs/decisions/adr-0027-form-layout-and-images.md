# ADR-0027: Form layout storage, Image fields and image files

- **Status:** Proposed
- **Date:** 2026-11-21
- **Decision owner:** not assigned
- **Related decisions:** ADR-0013 (CEL), ADR-0015 (Workspace files), R9, B9 slice 1

## Context

Beta slice B9 needs, on the server: a form layout per Record Type (sections of
fields, text, images and art, §19), a first-class Image field type, and an
image upload path with thumbnails. The questions are where the layout lives,
how image values reference files, where uploads land, and how far to process
uploads (metadata, thumbnails).

## Options considered

1. Layout in its own table with a foreign key to the Record Type — relational
   purity, but a migration, joins on every List, and a second write path next
   to the journaled publisher for data that versions with the definition.
2. Layout stored with the definition JSON — no migration, versions and
   journals with the definition for free, List serves it from the same row.
3. Image values embedding bytes — self-contained rows, but multi-megabyte
   rows, no versions, no reuse across Records.
4. Image values referencing Workspace files (path plus hash) — files keep
   their versions, audit and concurrency machinery; Records stay small.
5. Thumbnails stored in a new table vs generated on read — stored is faster
   per read; on-read has no stale state and no migration.

## Decision

- The layout is part of the Record Type definition (`FormLayout` on the
  proto, stored in `athena_record_types.definition`). No new table and no
  migration. A layout-only save runs the journaled publisher with zero
  migration steps: no confirmation, a revision bump and a History row.
- Validation at save: at most 50 sections and 500 blocks; columns 1 or 2;
  field keys exist and appear at most once across all sections, widths full
  or half; section visibility is CEL returning true or false over the
  Record's fields (display-only, like `depends_on`, so `today()` is
  allowed); image blocks name recorded files under `images/`; text and art
  blocks carry bounded strings (art keys are validated by the desk, which
  owns the registry). Fields the layout does not place are appended to the
  last section at full width; an empty layout normalizes to none.
- The Image field type stores a JSON reference `{path, hash, alt_text}` in a
  JSON column. Alt text is required (1–1000 characters). The hash pins what
  the writer saw; reads follow the file's current content. Record writes
  check the reference shape only; existence is checked where it matters
  (layout image blocks) and through GetFile on read. Formulas cannot target
  Image fields, and Image fields take no index, default pattern or numeric
  attributes. Deleting a Record never touches file versions.
- Images live under a new `images/` Workspace folder, scoped both ways:
  image extensions (PNG, JPEG, WebP) only there, and only images there. A
  new `PutImage` RPC (files:write, step-up like PutFile) validates size
  (5 MiB), sniffs content rather than trusting the name, requires the name
  to match the sniffed type, decodes to prove validity, strips metadata and
  stores through the existing versioned, audited, concurrency-checked file
  path. `GetFile` gains a `thumbnail` flag returning a 256 px thumbnail
  (long edge, never upscaled) generated on read: PNG and JPEG keep their
  format, WebP thumbnails are PNG.
- JPEG EXIF orientation is parsed and applied before stripping, because
  dropping the tag without rotating would lay portrait photos sideways;
  other JPEGs keep byte-identical content apart from dropped Exif/XMP
  segments. PNG metadata chunks are dropped the same way. WebP EXIF, XMP
  and ICC chunks are dropped with their VP8X feature bits; orientation is
  not applied to WebP, whose encoders bake rotation into the pixels.
- PNG and JPEG use the standard image packages. WebP decoding uses
  golang.org/x/image/webp, pure Go with no cgo, so the single binary stays
  a single binary; it is the only new dependency this slice takes.

Open for the desk slice: mirroring Image value rules in fieldRules.ts and
the shared rules table, whose value shape is string-only today.

## Consequences

- RecordType gains field 21 (layout), FieldValue gains `image_value`,
  FileService gains PutImage; all additive. No migration: 17 stays unused.
- Lists render thumbnails through GetFile with no new tables; if lists get
  slow, a thumbnail cache table is the documented next step, not a redesign.
- A hash mismatch between a stored reference and the current file is
  possible after re-upload; readers show current content.

## Revisit when

- Lists need faster thumbnails (add the cache table), the desk needs
  server-side art-key validation, or a second image consumer needs
  orientation applied to WebP.
