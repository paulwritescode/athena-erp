# ADR-0022: Ledger foundation and kernel masters

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** D-007, ADR-0006, ADR-0010

## Context

Postings need valid targets: companies, dimensions, periods, currencies, accounts, determination mappings and Settings. These masters must exist before the posting engine (G6), but D-007 says Athena is a reusable platform core without preconfigured industry content. The question is which masters the kernel may own without becoming an industry ERP.

## Options considered

1. **Kernel owns the ledger mechanism, nobody owns content (chosen).** Migration 13 creates the master tables; the kernel validates structure (trees without cycles, non-overlapping periods, keyed rates, specificity resolution). No jurisdiction, chart, tax rate or currency row is seeded: the sponsor supplies them.
2. **Packs own all masters.** Every deployment reinstalls companies and periods from a Pack; the kernel cannot validate a posting without Pack plumbing it does not have yet (Pack install is G10).
3. **Kernel seeds a starter chart.** Fastest to demo, but it bakes one jurisdiction's choices into every Workspace and contradicts D-007 directly.

## Decision (proposed)

- The kernel owns these masters and no others: Company, Branch, Department, Cost Centre, Dimension Definition, Fiscal Year, Accounting Period, Currency, Exchange Rate, Account Type, Account, Account Determination, Setting and Setting Value (`internal/ledger`, migration 13 `ledger_foundation`).
- Relation to D-007 is OPEN until the sponsor confirms it: the claim is that ledger masters are platform mechanism, like Record Types and the Audit Log, not industry content. If the sponsor disagrees, the masters move behind Pack install and this ADR is revisited.
- Nothing is seeded. A fresh Workspace has empty masters; the setup pages and `athena ledger` commands exist so the sponsor's choices can be entered. Jurisdiction, chart and tax rates are explicitly OPEN (Requirement 5.15).
- Ledger and Balance Record Type kinds stay kernel-only: only kernel-owned definitions may use them, and `RecordService` refuses direct writes to ledger-kind tables.

## Consequences

- G6 can validate postings against real masters. The readiness view names each unresolved mapping so setup order is visible.
- Accepting this ADR narrows D-007 to industry content (Packs, charts, rates) and excludes ledger mechanism. Until accepted, the masters exist but the D-007 question stays open in the log.

## Revisit when

The sponsor rules on D-007, or Pack install (G10) offers a home the masters should move to.
