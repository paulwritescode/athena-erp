# Summary: C4 slice 5 (CLI tree+uom, determination as-of, Desk flags, docs)

Merged 2026-11-24 as ede4a20 (rebase of c29d2c9 onto post-C2s5 main; decision-log union only), 1 commit, 7 files.

Scope: 13.5–13.7 remainder — `athena ledger tree|uom` subcommands + determination effective-from/to with as-of resolve (append-only ledger.go) + tree/effective-dated flags in RecordTypeEditorPage settings (exact Stylus plain words, additive types/api mapping) + record-type-and-pack-model + decision-log rows. As-of Desk view deferred per ruling (follow-up filed). FieldSettingsDialog untouched (F3 owns); fieldRules already in step.

Evidence: worker full suite green + browser 26/26 both themes (0 page errors, shots eyeballed). Merge gate on c29d2c9: exit 0, 24m (19 Go ok all fresh under suite -p 2, 684/684 web, zero FAIL, log 1124-181313). Merged during C5s5's gate per Stylus order (trial pick: log-only conflict, 5/5 code files identical, build+vet green).

Review: r-c4s5 PASS (review-slice-5.md) — CLI + as-of verified live, flags per ruling, gates green.
