# ADR-0021: Atomic child and parent Record Type save

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0006, ADR-0010

## Context

A child Record Type and its parent save in one `SaveRecordType` request so a rollup formula over a new or changed child needs no second save. The publisher saved the child first and the parent after, but a parent failure left the already-saved child behind, and a crash between the two left the same half-state. The roadmap tracked making the pair atomic. One transaction cannot do it: MariaDB commits each DDL statement implicitly, so the two table changes can never share a transaction.

## Options considered

1. **Compensating rollback in the journaled publisher (chosen).** A `save_group` journal row records both members, the revisions edited, the previous child definition and the member operation IDs before either member runs. A parent failure compensates the child; a crash between members rolls back at startup through `resumeOperations`.
2. **One database transaction.** Impossible: DDL autocommits statement by statement.
3. **Parent first.** Rejected: the parent's validation reads the child's saved definition, so the child must commit first. The child's first position also makes compensation cheap, because the compensated member is usually new.

## Decision (proposed)

- The request validates the parent's expressions against the child as it will be, prepares the child, checks the parent revision, journals the group, commits the child, then prepares and commits the parent, all under the schema lock.
- Compensation: a new child loses its table (refused when Records were written after its save) and its metadata row; an updated child gets its previous definition and revision back with its schema migrated back through the same direction-agnostic migration engine. Dropped columns come back empty: their values died with the drop and are not restored.
- Resume never runs a member as a lone save. A group whose members both landed is marked completed; a member that moved on without the group, or a parent that saved without its child, is left for an operator; anything else compensates. A failed compensation marks the group and stops retrying, so a bad rollback does not wedge every boot.
- The desk already sends one request and shows one toast; the failure toast now reports the rollback. No journal migration was needed: the group reuses the operations table with a JSON payload.

## Consequences

- Removes the roadmap tracked item for atomic pair saves. The pair API and the desk flow are unchanged apart from the failure message.
- Operators gain a new state to know: `compensation_failed` means the pair is half-applied and needs a human; the group row names the cause.

## Revisit when

Record Type migrations grow cross-type steps that need more than one compensation, or Pack install needs multi-type atomicity beyond a pair.
