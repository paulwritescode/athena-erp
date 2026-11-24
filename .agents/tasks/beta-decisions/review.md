# Review: night-shift decisions docs (r-dec, L5)

Verdict: PASS

Tree: `/Users/iampaulking/projects/duchess/athena-erp-dec-r`, detached at
`7d7b93a` ("docs: night-shift decisions Z1-Z7/Z10/Z11/Z13/B7/B10 + Apache-2.0
licence"), 1 commit on `a6dec5c`. Tree verified clean; `git show --stat HEAD`
is docs-only, exactly the 7 files: `tasks.md`, `decision-log.md`,
`adr-0023-posting-engine-and-operations.md`, `LICENSE`, `NOTICE`, `README.md`,
`AGENTS.md`. No source, no ADR-0032/0033 files (confirmed absent on disk).
Authority: `night-shift-orders.md` decisions table + `beta-decisions/brief.md`.

## Checks (review-brief items)

1. Section-Z transcription (the crux — checked word against orders):
   - Z1: Circle Space / event management / Kenya / KES / 16 % VAT (+
     zero-rated/exempt) / Jan–Dec FY / IFRS-for-SMEs chart / M-Pesa-bank-cash
     recorded-only / eTIMS out of beta as C10 follow-up — all present.
   - Z2: open-periods-only, no day window; soft-close 10th (`ledger:close`
     adjustments with reason); hard-close after the VAT return (20th, refuses
     all); reopen (`ledger:close` + reason + later periods + audit);
     current-period reversals; one statutory KES book + named books; ADR-0023
     amended (O-011/O-012) — all numbers and scope exact.
   - Z3: perpetual weighted-average for all stock (IAS 2.25); FEFO picking;
     rentals tracked-not-consumed; negative stock refused; ADR-0032
     (C7-owned) pointer — matches orders + brief NOTE (no ADR files here).
   - Z4: 7 yr / 90 d / 1 yr / 30 d / 90 d figures all exact; guest-data
     anonymisation + dietary consent; env/OS-keychain owner-held Workspace
     key; `athena secrets rotate`; ADR-0033 (C2-owned) pointer — matches
     orders + brief NOTE.
   - Z5: Apache-2.0 (LICENSE, NOTICE, README section); C3 + E3 unblocked.
   - Z6: `vite-plugin-pwa`; single server on loopback; nightly
     `mariadb-dump`, 4 h restore / 24 h data-loss targets — numbers exact.
   - Z7: first industry Pack is Event Management (Circle Space).
   - Z10: Satori on public too; licence checked 2026-11-24 (Personal-Use
     trial only) → no Satori file bundled until a web licence is bought;
     interim system stack on public, Desk via `local()` only — matches brief
     item 8. Corroborated: no `*satori*` file anywhere in the tree;
     `web/src/assets/brand/` holds only the two Athena PNGs.
   - Z11: product name Athena (final). Z13: setting name + default quoted
     verbatim from orders. Blocks columns untouched.
   - B7: tabs bullet carries "(playbook §8; descoped 2026-11-24 until a
     switch flow exists, B7)". B10: slice-1 note carries "Custom
     illustrations accepted 2026-11-24 (B10; see F6 for the logo)". C3/E3
     lines unticked with "(Unblocked 2026-11-24: Apache-2.0 decided, Z5;
     implementation pending.)" — per brief item 6.
2. Decision-log: 12 new rows (Z1–Z7, Z10, Z11, Z13, B7, B10), all dated
   2026-11-24 with correct decisions and ADR pointers (Z2→ADR-0023,
   Z3→ADR-0032 C7-owned, Z4→ADR-0033 C2-owned). O-009/O-011/O-012 updated to
   Decided with correct substance and ADR-0023 links (links resolve).
   ADR-0023 row (O-011/O-012 decided, Z2), ADR-0028 row (Z4 decided,
   ADR-0033 C2-owned, implementation pending) and ADR-0031 row (Z2 decided,
   sentinels byte-identical) all correct against the orders.
3. ADR-0023 amendment: O-011 (open periods, no day window, soft-close 10th
   with clerk/`ledger:close` split, hard-close after the 20th VAT return,
   reopen rules, current-period reversals) and O-012 (one statutory KES book
   + named books, unnamed refused) transcribed fully; "Revisit when" drops
   O-011/O-012 with a dated note. Status stays Proposed.
4. LICENSE: all 9 canonical section titles + appendix, 202 lines
   (canonical length), appendix placeholder `[yyyy] [name of copyright
   owner]` unfilled. NOTICE: product title line + the exact holder line
   `Copyright 2026 Athena contributors` + the required Apache notice block.
   README licence section (Z5 date, LICENSE/NOTICE links resolve, SPDX-new-
   files-only rule) and AGENTS.md ("The product name Athena is final.",
   rest of line intact) match the brief.
5. No gate run (docs-only, per brief). Full diff re-read: every hunk in all
   7 files is an intended edit; no stray changes, no `.agents/` or
   `dist/.gitkeep` noise. Section-Z header still reads
   "## Z. Decisions the user must make (OPEN)" — confirmed the only
   intentional leftover, per brief scope.
6. Remaining effort went to item 1; no number, date or scope deviation found.

## Advisories (not verdict-changing)

- Z3/Z4 rows necessarily compress the orders ("one formula for items of
  similar nature"; "at least 7 years"; "aggregate counts kept"; "sessions
  follow the existing session policy"; "never in the database or backups").
  The full text must land in worker-owned ADR-0032 (C7) / ADR-0033 (C2) —
  flag for those slice reviewers.
- NOTICE's `Athena` first line is the conventional product title, not a
  second holder; conforming.
- Section-Z header still says OPEN while every row is DECIDED; header
  cleanup is out of this brief's scope.

Review tree left clean and untouched (this report written to the main repo
only, not the worktree).
