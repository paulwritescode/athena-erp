# Summary: C4 slice 3 — UoM families + determination effective ranges

Merged 2026-11-24 as 21e5973+bc9b1e3 (built at
8df6b0c+ac31fc1; cherry-picked cleanly onto 46bf4fd,
skipping stacked 13.3b).

- New `internal/ledger/uom.go` (+224-line tests): families
  with base unit, default + item-override rows,
  disabled-override fallback, cross-family rejection naming
  both families; exact big-int math (one rounding per leg);
  typed errors (NotFound/Ambiguous/CrossFamily); base-unit
  rows refused; factors over 12dp rejected at save (review
  fix — 13dp silently read back 0 against DECIMAL(30,12)).
- `determination.go`: ResolveAccount/MatchDeterminations
  require as-of (total contract, no silent skip); server/CLI
  callers pass today per G13; pre-migration NULL/NULL reads
  fully open (old behavior preserved).
- Boundaries match slice 2 exactly ([from, to), NULL open,
  strict dates); quantity round-trips clean values (full
  Property 25 stays with slice 4).
- No proto/gate/migration/web changes; Z2/Z4 untouched.
- Review: NEEDS_CHANGES then PASS (review-slice-3.md) —
  one real blocker (the 12dp corruption), fixed with
  round-trip + rejection tests; picked files verified
  identical; gates 7/7 rerun green.
- Merge-time verification: full make test exit 0 (Go ok,
  web 616/616) on bc9b1e3.
- Follow-ups: posting must pass voucher date (not today)
  when adopting ResolveAccount; as-of not yet on RPC/CLI;
  trim consistency; two-leg rounding bound for Property 25;
  override + g→mg test gaps.
