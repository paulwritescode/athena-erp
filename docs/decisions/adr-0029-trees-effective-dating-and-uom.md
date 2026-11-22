# ADR-0029: Trees, effective dating and UoM

- **Status:** Proposed
- **Date:** 2026-11-22
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0022, ADR-0009, ADR-0010, D-007 (kernel masters relation open)

## Context

Ledger masters already form trees (accounts, branches, departments, cost centres link through `parent_name`), but descendants need recursive walks and nothing stores the path. Account determination has no time dimension, so a remapping rewrites history instead of dating it. Quantities carry a UoM key (`internal/types.Quantity`) with no master behind it: no families, no factors, no cross-family guard. Batch-2 requirement R13 asks for hierarchies, dated references and unit families so that history stays reproducible. Record Type level tree and effective-date flags ride on the same mechanism: the publisher adds the columns when the flag is set.

## Options considered

1. **Materialised path plus depth, end-exclusive effective ranges, UoM families with exact factors (chosen).** Each tree row stores a slash-joined `tree_path` (`/root/child/`, root depth 0) and `tree_depth`; names and ids hold no slashes, so the join is unambiguous. Effective ranges are `[effective_from, effective_to)` dates; NULL from means the beginning of time, NULL to means open-ended. UoM families name a base unit; conversions hold an exact factor (base units per one of the unit) with item-specific overrides; conversions across families are rejected.
2. **Adjacency list only with recursive queries.** Keeps one column but pushes every subtree read into a recursive CTE and leaves depth unindexed; the path prefix index answers "all descendants" directly.
3. **Nested sets.** Fast subtree reads but every insert rewrites the numbering of its siblings; path updates touch only the moved subtree and stay readable in debugging.

## Decision (proposed)

- Migration 18 `trees_effective_uom` creates `athena_uom_families` (name, label, base unit, disabled, revision) and `athena_uom_conversions` (family, unit, item key, exact factor, scale, disabled, revision; an empty item key is the family default, a filled one overrides it for that item). It adds `tree_path` and `tree_depth` to `athena_accounts`, `athena_branches`, `athena_departments` and `athena_cost_centres`, and `effective_from`/`effective_to` (end-exclusive) to `athena_account_determinations`. It backfills path and depth from the stored parent links and fails naming the table when a row is left without a path (cycle or dangling parent, only possible through hand edits: saves already refuse both). Nothing is seeded.
- A Record Type flagged as a tree stores `tree_parent_id`, `tree_path` and `tree_depth` on its rows (`tree_parent_id`, because `parent_id` already belongs to child Record Types); the kernel refuses cycles and cross-company links. A Record Type flagged as effective-dated stores an end-exclusive range per row, refuses overlapping ranges, and resolves "as of" lookups end-exclusive.
- Determination resolution prefers the most specific effective match within company and book and stores the resolved account and mapping revision with the posting.
- Quantity lines store entered unit, factor and base quantity; converting to base units and back returns the entered quantity within the declared scale.

## Consequences

- Ledger saves recompute path and depth for the saved row and its descendants; subtree reads use the path prefix index.
- `RecordTypeService` gains `tree` and `effective_dated` attributes; the publisher plans their columns like any other schema change.
- `athena tree|uom|determination` commands and Desk field settings expose the new attributes.
- Property 25 pins the range and round-trip behaviour.

## Revisit when

The sponsor confirms or rejects the D-007 relation of kernel masters, or a Record Type tree outgrows the 2048-character path (deeper than about 100 levels of long names).
