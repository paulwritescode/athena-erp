# Summary: C5 slice 1 — migration 20 + ADR-0031

Merged 2026-11-24 as f00fc58 (built at 6c77049, rebased
cleanly twice — f466742 on 46bf4fd, f00fc58 on b9d08a2;
patch byte-identical across the second rebase).

- Migration 20 `party_ledger`: `l_party` append-only facts
  (no updated_at, reversal mirrors, party keys, control
  account, NULL-able item_id for unapplied cash, base+txn
  DECIMAL(28,6) pairs as in l_gl); `athena_party_items`
  (one row per instalment, unique key, per-instalment
  currency/side/original+residual); `athena_settlement_
  applications` (per line+item, both currencies, base
  amount, reversal link, group). Number matches the
  reservation; purely additive; Target-upgrade test green.
- ADR-0031 (Proposed): tables, append-only/endpoint/
  unapplied-cash rules, ceiling/lock honestly scoped to
  G7.2+; every claim verified against code (l_gl
  decimals, sentinels byte-identical, G9 consistent);
  nothing seeded; no invented policy.
- Decision-log + README rows link correctly.
- No proto/gate/web/cmd changes; Z2 untouched (nothing
  skipped — no posting behaviour in this slice).
- Review: PASS (review-slice-1.md) — verified on f466742,
  worker's full green run read in full; reviewer's own
  scoped run terminated as redundant per reuse (the 76s
  stale event), lock passed cleanly.
- Merge-time verification: full make test exit 0 (Go ok,
  web 616/616) on f00fc58.
- Follow-ups: ADR "mirrors negate" → "mirrors swap sides"
  wording; tooling note (sysmigrations.go to affected
  FULL_PATHS?) for Stylus.
