# Review: C1 6.10 — Desk voucher view

Verdict: PASS

Commit: 9a299ca (detached -r tree, verified clean on arrival and after every gate).
Diff: RecordsPage.vue + PageSwitch.vue (1 line) + pageSwitchBuild.test.ts (1 line) + RecordsPage.test.ts (new). No server/proto/migration/gate changes.

## Blockers

None.

## Item verdicts (per review-brief-6.10)

1. Voucher view — PASS. Preview dialog lists plans+legs and its Submit carries
   `postingPreviewHash` (`runTransition('submit', previewHash)`); plain Submit
   sends none (empty claims no approval — pre-existing server path, unchanged).
   Hash binding verified server-side: preview (`internal/server/ledger.go:928`)
   and submit (`internal/server/lifecycle.go:340`) both hash with
   `posting.HashPlans`, and a set hash that no longer matches refuses the
   submit with FailedPrecondition "changed since it was previewed" — the
   client maps exactly that string to a re-review toast and refreshes the
   preview (`RecordsPage.vue` postingBlockedMessage + runTransition catch).
   Sheet shows ledger rows + posting log with paging; blocked submit/cancel
   paths toast (`submit-blocked-light` shot proves it — still Draft, toast
   visible). Nothing fails silently.
2. View vocabulary — PASS. "Recorded in the books" badge via
   `gloss('posted')`; Submitted/Cancelled/Reversed/Document/Books entries all
   glossary words. No doc./field-key jargon anywhere in the new UI.
3. Every state — PASS (code + shots). Dialog and sheet: loading skeletons
   (aria-busy), error + Try again per section, denied (permission) states,
   quiet empty states (no-plan, booksEmpty, per-section). No custom
   interactive elements and no new CSS — rest/hover/focus-visible/pressed
   inherit from ActionButton/shadcn Dialog/Sheet/Table/Badge; no disabled
   states exist (buttons hidden instead, e.g. Submit only when ready+plans),
   `:busy` spinners on submits and Show-more.
4. Shots — PASS, all 15 eyeballed: submitted, preview-blocked,
   submit-blocked, cancelled-dark, preview-light 375/768/1280/1440 (drawer
   below 1280, all clean), sheet-dark 375/768/1280/1440, hidden, sheet-empty,
   sheet-reversed. Dark coherent, no layout regression. Disclosed gaps match
   exactly: no sheet-light-* files (flow died at the harness assert before
   capturing them), reversed shot mid-animation (mirror rows + Reversal
   badges still legible behind it), 375 credit column clipped. No
   undisclosed breakage.
5. Browser evidence — PASS. Suite log `1124-050815-w-c1.log` (retry 6):
   "25 passed, 1 failed", the ONLY failure "light flow" from
   `DRIVE FAIL: timeout waiting for sheet Cash`. Harness-side confirmed:
   the sheet renders `row.account` (server name "cash"); `LedgerRow`
   (ledger.proto) carries no `account_label`, unlike `PostingPlanLine` which
   is why the preview assert on 'Cash' passed and the sheet one could not.
   The sequential waits show Status/Lines/Attempts all rendered — only the
   capital-C assert failed. Lowercase ruling: NO copy-rule violation —
   account names are workspace data, the glossary sets no casing rule for
   data, and the "no doc./field-key jargon" bar holds.
6. PageSwitch prop + mock — PASS. One-line `:can-ledger` prop threading the
   real `useSession.canLedger` (ledger:read); RecordsPage's single mounter
   is PageSwitch. Mock fix adds `canLedger: ref(false)` next to the
   existing `canWriteLedger`, mirroring the real session shape — principled,
   not mock-theatre. Mock green (pageSwitch 3/3 in the rerun).
7. Scope — PASS. Four web files only; RecordsPage.vue + PageSwitch.vue +
   tests. Z2 untouched: preview/submit/cancel go through the existing
   reviewed server paths, zero posting-behavior change. F1 disjoint holds
   (F1 owns DataTable files, untouched here).
8. Gates — PASS, all on 9a299ca, tree clean throughout:
   - `pnpm run typecheck` (rerun): green
   - touched Vitest `RecordsPage.test.ts` + `pageSwitchBuild.test.ts`
     (rerun): 11/11 green
   - `pnpm run build` (rerun): green in 1.32s (`.gitkeep` restored after)
   - `.agents/bin/affected` (absolute main-repo path, through the lock):
     green, 64 files / 624 tests
   - `make generate` twice: both exit 0, tree clean = stable
   - No full browser re-run per the brief.

## Follow-ups (non-blocking)

1. `LedgerRow` has no `account_label`, so the sheet shows lowercase account
   names while the preview shows labels — add the label server-side
   (proto change, out of 6.10 scope) and re-capture the light sheet widths.
2. Blocked reasons leak the engine prefix "Posting: invalid plan:" into
   View (glossary word is "Books entry", not "Posting"). Consider
   stripping/normalizing that prefix in `postingReason` or mapping common
   reasons to plain words. Lead sentences are already plain, so not a
   blocker. (`RecordsPage.vue` postingReason)
3. Sheet legs table clips the Credit column at 375 (disclosed) — consider
   horizontal scroll or stacked legs at narrow widths.
4. `StatusBadge` renders `reversed` as a quiet outline (unknown-status
   fallback, by design). Consider a known variant if Reversed should carry
   color. Cosmetic only.
5. Re-capture `sheet-reversed` settled (disclosed mid-animation) when the
   harness assert is fixed.

## Notes

- Worker PASS claims "content identical to tested d13f4e0" after rebase;
   the -r tree at 9a299ca is what I reviewed and gated — all green here,
   so no reliance on that claim.
- pass.sh seeds the closed period via raw SQL, citing a pre-existing CLI
   scan bug (`period close/list` DATE parse) — environment note, not 6.10's
   concern; worth a ticket for whoever owns the ledger CLI.
- Posted amounts render as exact server decimals ("100.000000" in the
   sheet vs "100" in preview) — the display-exactly policy, engine shapes
   differ. Consistent with the drive comment; not flagged.
