# Review: C4 slice 3 — UoM families + determination effective ranges (13.3)

Verdict: PASS

Commit: 8df6b0c + fix ac31fc1 (cherry-picked onto the review tree as
abbf6e8; picked files byte-identical to the branch). Tree clean at
every gate. Diff is the briefed 8 files plus the 2-file fix; no
proto, gate.go, migration, web, or slice-2 file touched. Slice-2
files (attributes/records/record_types/migrations.go, store.go,
record_types.proto) untouched; server/CLI caller tweaks are the
required as-of threading.

## Blockers

1. FIXED in ac31fc1 (verified): conversion factors with more than 12
   fraction digits were silently corrupted at rest (R13.4 "exact
   factors") — saving `0.0000000000001` (13 dp) succeeded, then
   `GetUoMConversion` returned `0.000000000000`. The fix rejects
   `Factor.Scale() > 12` with an input error in `SaveUoMConversion`
   (`internal/ledger/uom.go:110-112`), covering insert and update,
   and adds `TestUoMConversionFactorPrecision` (12-dp round-trip +
   13-dp rejection). The 13-dp case fails pre-fix (proven by the
   earlier probe with the same value), so it is a valid regression
   test. Re-reviewed the delta, re-ran gates on abbf6e8 (below).

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
   Fix re-gates on abbf6e8: `gofmt -l cmd internal` empty;
   `go build ./...` ok; `go vet` on ledger+types ok;
   `TestUoMConversionFactorPrecision` green direct;
   full `affected` green 7/7 (cmd/athena 303s, expr, ledger 253s,
   posting 566s, rules, server 485s, types), no FAIL. Generate not
   re-run on the fix (no proto/gen inputs in a 3-line + test delta;
   stable on the parent).
