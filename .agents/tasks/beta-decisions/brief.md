# Brief: night-shift decisions docs commit (Muse, L5 after FU batch 2)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-fu, branch `feat/fu-batch-2` (commit on top after the batch-2 work; this is a docs-only commit, no gate needed, but rebase on main before reporting).
Read: `.agents/stylus/night-shift-orders.md` (Decisions table — the exact decision texts, with sources), `docs/decisions/adr-template.md`, `AGENTS.md` (Keep documentation current).

No ADR files in this task and no migration. NOTE (2026-11-24): ADR-0032 moved to the C7 worker and ADR-0033 moved to the C2 worker (each writes its ADR as its slice 1) — do NOT write either ADR here; still record the Z3 + Z4 decision rows (pointing at the worker-owned ADRs).

Task (one docs commit; report with say master "DECISIONS DOCS: PASS or BLOCKED <one line>"):
1. `tasks.md` section Z: rewrite rows Z1–Z7, Z10, Z11, Z13 as `DECIDED 2026-11-24: <decision>` (follow the Z0 row format; keep the Blocks column), using the exact decision texts from night-shift-orders.md. B7/B10 have no Z rows — note B7 descoped + B10 accepted in the rows/notes where they appear (B7 open question, B10 slice-1 follow-up).
2. `docs/decisions/decision-log.md`: one row per decision (Z1–Z7, Z10, Z11, Z13, B7, B10) with date 2026-11-24, the decision, and the ADR link where one exists.
3. Amend `docs/decisions/adr-0023-*` (Z2): O-011/O-012 become open-periods-only posting, soft-close 10th (ledger:close adjustments with reason), hard-close after the VAT return (20th, refuses all), reopen rules (ledger:close + reason + later periods + audit), corrections as current-period reversals, one statutory KES book + named books. ADR stays Proposed.
4. New ADR-0032 (Z3, Proposed, from the template): perpetual weighted-average valuation (IAS 2.25), FEFO picking for perishables, rentals tracked-not-consumed, negative stock refused. It unblocks C7.
5. New ADR-0033 (Z4, Proposed, from the template): retention schedule (7yr ledger/invoice/stock/audit no beta auto-delete; guest data anonymised 90d post-event + dietary consent; Access Log 1yr; outbox/events 30d; job runs 90d) + key custody (Workspace key from env/OS keychain, owner-held, never in DB/backups; `athena secrets rotate`). It unblocks C2 secrets (8.8–8.10).
6. Z5: add `LICENSE` (Apache-2.0 full text), `NOTICE`, SPDX headers in NEW files only (do not touch existing files), README licence section. Note C3 + E3 unblocked (tasks.md C3/E3 lines stay unticked — implementation still pending).
7. Z11: `AGENTS.md` "Working product name" line → product name Athena is final (keep the rest of the line intact).
8. Z1/Z6/Z7/Z10/Z13/B7/B10 need no code files — tasks.md Z + decision-log rows only. Z10: record DECIDED (Satori on public) PLUS the licence outcome (checked 2026-11-24: Personal-Use trial only, no licence/binaries in repo → user must buy a web licence or relent, question open; interim: system stack on public, slice 4 as-is, ship no Satori file; Desk loads Satori via local() only). Z13: note the Workspace setting default.

Rules: ADRs stay Proposed (binding — never mark accepted; tasks.md Z carries the DECIDED record). Transcribe decisions exactly as ordered; do not extend or reinterpret them (flag anything ambiguous with QUESTION instead of guessing). Docs-only: no suite runs (but do not commit while the worker's own batch-2 gate runs — commit after it lands). Restore internal/webui/dist/.gitkeep before committing. You never merge, push or start agents.
Checks: markdown links resolve (ADR/section links), headings match existing formats, `git status` shows only the intended files.
Report: say master "DECISIONS DOCS: PASS or BLOCKED <one line>" + files changed. Then wait for instructions (F4 design brief follows).
