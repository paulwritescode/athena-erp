# Review: C4 slice 3 — UoM families + determination effective ranges (13.3)

Verdict: NEEDS_CHANGES

Commit: 8df6b0c, tree clean on arrival and at every gate. Diff is the
briefed 8 files; no proto, gate.go, migration, web, or slice-2 file
touched. Slice-2 files (attributes/records/record_types/migrations.go,
store.go, record_types.proto) untouched; server/CLI caller tweaks are the
required as-of threading.

## Blockers

1. Conversion factors with more than 12 fraction digits are silently
   corrupted at rest (R13.4 "exact factors").
   `internal/ledger/uom.go:107-109` checks the factor is positive and
   `uom.go:110-112` bounds the rounding scale, but nothing checks the
   factor's own decimal scale against the `DECIMAL(30,12)` column from
   migration 18. Probed with a throwaway test (removed after): saving
   factor `0.0000000000001` (13 dp) succeeds, then
   `GetUoMConversion` returns `0.000000000000` — silently zero, and 0
   is not even a legal factor. Fix: reject `Factor.Scale() > 12` with
   an input error at save (factors at/above 1e18 already fail loudly
   under strict mode, but a clean input error there is nicer too),
   plus a save/load round-trip test for a 12-dp factor and rejection
   of a 13-dp one.

## Follow-ups (do not block)

1. When posting adopts `ResolveAccount`, it must pass the voucher's
   business date, not "today" (`internal/server/ledger.go:574`,
   `cmd/athena/ledger.go:413`). Today-as-default matches G13/R13.3
   (neither names a default; the request carries no date) and is safe
   in this slice — only the resolve RPC and CLI commands call it —
   but resolving old postings as of today would break R13's
   "history stays reproducible". The server comment already notes a
   later slice may add an as-of field to the request.
2. Effective dates are not exposed over RPC/CLI yet (no proto
   change, verified). Expected; slice 5 owns CLI/Desk.
3. `checkEffectiveRange`/`checkAsOf` (`determination.go:39-62`)
   reject untrimmed input while slice-2 `parseEffectiveRange`/
   `parseAsOf` trim first. Safe direction (reject, never misread);
   trim for consistency if touched.
4. `ConvertQuantity` across two non-base units rounds twice (once
   per leg at each row's scale); R13.6 pins only entered→base→entered.
   Slice 4's Property 25 should pin the exact two-leg bound. This
   slice's round-trip tests cover clean values only, which is the
   briefed preview.
5. Minor test gaps: `FromBaseQuantity` with an item override, and a
   g→mg two-leg conversion. Otherwise coverage is good (validation
   tables, override/disabled fallback, cross-family on both paths,
   ambiguous unit, disabled family, identity-without-rounding,
   inclusive-from/exclusive-to boundaries, open ranges, bad/empty
   as-of, revision reporting).

## Checked per the brief

1. UoM: families with base unit, default + item-override rows,
   disabled-override fallback, cross-family rejection naming both
   families — match G13/R13.4/R13.5. Math is exact big-int with one
   rounding per leg (`Decimal.Mul` exact then `Round`; `Div` once).
   Unknown unit → `ErrNotFound`, multi-family unit → `ErrAmbiguous`,
   cross-family → typed `ErrCrossFamily`. Base-unit rows refused.
2. Determination: `ResolveAccount`/`MatchDeterminations` both require
   as-of (`checkAsOf` rejects empty/bad); only two callers, both pass
   today. Contract is total — no silent skip. Pre-migration rows read
   NULL/NULL = fully open, so old behavior is preserved.
3. Boundaries match slice 2 exactly: `[from, to)`, NULL/empty open,
   from strictly before to, strict `YYYY-MM-DD`
   (`effectiveOn` ≡ `asOfPredicate` in `records.go:647`).
4. Tests rerun green (see gates). Coverage judged good apart from
   the blocker and follow-up 5.
5. Confirmed: no proto/gate/migration/web changes; Z2/Z4 untouched
   (no ADR/decision-log/posting changes); no `time.Now` outside the
   two briefed caller tweaks.
6. Gates: `gofmt -l cmd internal` empty; `go build ./...` ok;
   `go vet ./...` ok; `affected` reused worker green run on 8df6b0c
   (7/7 packages ok, no FAIL/panic in
   `suite-logs/1123-223933-w-c4.log`); direct rerun of
   `TestUoM*`/`TestDetermination*` (ledger, 123s) and `TestQuantity`
   (types) green; `make generate` twice with no diff (needed node
   from the suite toolchain path on PATH); tree clean throughout.
   No browser pass (no Desk in slice).
