# Review brief: C5 slice 6 (7.6 backend only)

Worker: w-c1. Branch `feat/c5-party`, commit d1923c5 on origin/main ff18cb2.
Scope: proto `ApplySettlement`/`ReverseSettlement`/`ListOpenItems` + generate-twice stable,
`internal/server/ledger.go` + `gate.go` entries, `cmd/athena/settle.go` (`apply|reverse|open-items`).
NO Desk work in this slice (slice 7); NO RecordsPage.vue touch expected.

Verify: worker evidence (full gate green pre-rebase; rebase clean, only main-side delta
is the ff18cb2 markers fix; carry per disjoint rule), build/vet/gofmt, generate stable,
gate entries + destructive-reverse handling, settle CLI live, Z2 edges (no sentinels in new code).
Run affected through the main-repo suite by absolute path.

Verdict line: `Verdict: PASS` or `Verdict: NEEDS_CHANGES`.
Report to master with `say master "C5 SLICE 6 REVIEW: PASS/NEEDS_CHANGES <one line>"`.
Reads only in the detached tree; never merge/push/spawn.
