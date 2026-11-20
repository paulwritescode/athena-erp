# P4 summary: record kinds and system fields

Review verdict: APPROVED, no findings (review pass 2). Nothing committed yet.

## What changed
- Record kinds: master, document (posting/non_posting/work/batch), child (fixed parent), setting. Kernel kinds are rejected. Kind and parent are fixed after first save.
- System fields (created_by, modified_by, def_version, request_id, owner, deleted_at, parent_id, idx), all reserved as field keys. `revision` serves as the spec's `version` (ADR-0010).
- Behavior: master soft delete, setting singleton and undeletable, child needs a live parent and gets next idx, parent delete blocked while children exist (including Master parents, fixed in iteration 2).
- Field attributes enforced in Go and mirrored in `web/src/fieldRules.ts`: unique, default, min, max, pattern, max_length, read_only, set_once, index. `no_copy` is stored only.
- New types: Money (minor units), Quantity, Enum. Changing currency, scale, unit or enum list goes through the confirmed data-loss plan.
- Migration 6 `record_kinds_and_system_fields` (ran against the dev DB, rerun-safe).
- Dialect additions, proto changes (regenerated), spans `athena.<op>` with no Record values, CLI `athena record-types` and `athena records`, desk UI (kind selectors, field-settings dialog, new inputs, shadcn dialog and checkbox).
- Docs: ADR-0010 (Proposed), decision log, overview, README.

## Verification
- go build, go vet, gofmt: clean.
- go test -count=1 ./... on MariaDB 11.8: all pass (rerun after iteration 2).
- make generate twice: no diff.
- pnpm typecheck, build, test: pass (64 Vitest cases).
- make restart: healthz Connected. Browser pass in light and dark themes OK.

## Open points
1. Should a soft-deleted master release its unique values?
2. New reserved keys (owner, idx, parent_id) make migration 6 refuse legacy fields using them (none exist now).
3. Deferred attributes (docstate, naming, parent_type, currency_field, required_if) come in later phases.
4. Child Records are entered by parent ID in the desk.

## Next: P5, lifecycle
Document states and transitions, per the spec.
