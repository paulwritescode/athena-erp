# ADR-0011: Document lifecycle, versions and the activity timeline
- **Status:** Proposed
- **Date:** 2026-11-19
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0006, ADR-0008, ADR-0010, spec sections 2.2, 5.1 to 5.3, 12.1, 16.2, roadmap phase P5
## Context
Document Records were plain drafts. The spec gives documents a kernel-owned state (draft, submitted, cancelled), a fixed event order, immutability after submit, amendment, and a snapshot and timeline trail.
## Options considered
1. **Business status as the state.** Module-owned and derived, so it can disagree with the data. Rejected (spec 5.1).
2. **A `docstate` column owned by the kernel (chosen).** Only the transition RPCs change it.
3. **Run every event inside the transaction.** Simple, but a notification hook could block or undo a commit. Rejected: `after_*` events run after the commit.
4. **Copy unique and read-only fields on amend.** Fails on the unique index, because the cancelled original still holds the value. Rejected; those fields stay empty on the successor.
## Decision (proposed)
- **Flags.** A Record Type of kind `document` may be `submittable` (not for class `work`) and `amendable` (needs submittable). A field may be `allow_on_submit`. Disabling `submittable` or `amendable` is refused once a document is submitted or amended.
- **System columns** on a submittable document: `docstate` (0 draft, 1 submitted, 2 cancelled), `name` (starts as the Record ID, unique), and `amended_from` when amendable. The keys `docstate` and `amended_from` are reserved for every Record Type; `name` is reserved on submittable documents. Records that predate the flag become drafts named by their ID. P6 replaces the name source with numbering.
- **Transitions** (`internal/lifecycle`, RPCs `SubmitRecord`, `CancelRecord`, `AmendRecord` on `RecordService`). Each runs in one transaction with a revision check (`expected_revision`), the state change, a history row, a Document Version and an Activity row. A failing in-transaction hook rolls all of it back.
- **Events** fire in the fixed order of spec 5.2 (`lifecycle.Sequence`): drafts fire `before_insert`, `before_validate`, `validate`, `before_save`; submit fires `before_submit`, `on_submit`, `on_status_change` in the transaction, then `after_submit`, `after_commit`; cancel mirrors it; amend fires `on_amend` then `after_commit`; delete of a draft fires `on_trash`. Go hooks register on `server.Kernel.Hooks`. Starlark attaches in P9 and cannot bypass these checks. `on_link` and `on_unlink` wait for document links.
- **Immutability.** A submitted document changes only in `allow_on_submit` fields, and each such edit adds an `edited_after_submit` activity row naming the field keys. A cancelled document never changes. Only drafts can be deleted. Child Records follow the parent: none can be added or removed after submit, and only their `allow_on_submit` fields can change.
- **Cancel** is refused while a submitted document links to the document, directly or through a child Record's Link field.
- **Amend** needs a cancelled, amendable document with no successor. It creates a draft with `amended_from` set, the name `<root>-1`, `<root>-2` and so on, and copies the values and child Records except unique, read-only and `no_copy` fields. A required field left empty by that rule makes the amend fail with the field named. The original stays cancelled.
- **Versions and timeline.** `athena_document_versions` holds a snapshot (values and child Records as JSON) and a diff from the previous version at submit, cancel and amend (the amend snapshot is the successor's). `athena_activity` holds state changes, amendments and edits after submit. Both are written by the kernel only (system migration 7). Not yet tamper-evident: the hash chain is P6.
- **Not built.** Ledger reversal on cancel (no ledgers yet), period locks, workflows and business status.
## Consequences
- Records of a submittable document cannot be edited through any RPC once submitted, including by Go hooks that skip the service.
- Amending a document with a required unique field is impossible until the field is not required or not unique. The owner may want a rule for this.
- Version snapshots hold Record values, so access to them needs the same permissions as the Records (P8).
## Revisit when
P6 (numbering replaces `name`, audit hash chain), P8 (actors and permissions), P9 (Starlark hooks), P11 (ledger reversal on cancel).
