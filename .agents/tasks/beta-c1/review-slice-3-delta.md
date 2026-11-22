# Review: C1 slice 3 DELTA re-review (rebase adaptation only)

Branch: `feat/c1-posting` in `../athena-erp-c1`, commit `ccaf482`
(reviewed delta `git diff 722e183 981ee78` vs reviewed `git diff 3dece91 5bbc94e`,
verified clean tree at `ccaf482` before any gate). Scope: the rebase adaptation
only, against the delta brief. The full slice-3 review (`review-slice-3.md`,
Verdict PASS for `5bbc94e`) stands for the slice logic and was not redone.

Verdict: PASS

No blockers. The adapted insertOutbox routes through events.Emit with a
parameterized event type and a payload byte-equal to the base's inline map on
the post path plus reversal_of_group_id on the reverse path; envelope fields
match the base; everything else is context shifts; posting suite green.

## Checks run (all in the worktree, MariaDB enabled)

- `git diff 981ee78 ccaf482 --stat` → `.agents/` docs only (21 files); the
  candidate is product-identical to `981ee78` as the brief states.
- `diff /tmp/rev.patch /tmp/cand.patch` (reproduced per brief) → only index
  lines, hunk headers, the `OutboxEventType` const alias, the two `insertOutbox`
  call sites gaining `(e.Workspace, ..., opts)`, and the `insertOutbox` /
  `outboxPayload` adaptation in `write.go`. All other hunks header-only.
- `gofmt -l cmd internal` → empty.
- `go build ./...` → ok. `go vet ./...` → ok.
- `go test -count=1 -timeout 20m ./internal/posting/` with
  `ATHENA_TEST_DATABASE_URL` (throwaway `test_athena_*` only) → ok in 225s
  (duration proves MariaDB tests ran, not skipped).
- `go test -v -run 'TestReverseHappyPath|TestPostEmitsEnvelopeAndOutbox'`
  → both PASS unskipped (3.42s / 4.74s).
- `make generate` twice (node via nvm) → both exit 0, tree clean, no diff.
- No full `make test` (master runs it at merge time, per brief). No browser
  pass: no Desk in this slice, per brief.

## What was verified by reading

1. Post-path payload (`write.go:458-474` vs base `722e183:write.go` inline
   map): `outboxPayload(nil, ...)` builds exactly the base's 9 keys with the
   same values (`voucher_type`, `voucher_id`, `voucher_revision`, `event`,
   `book`, `posting_date`, `posting_group_id`, `row_count`, `plan_hash`);
   key order irrelevant, values identical. The helper is also line-identical
   to the reviewed `5bbc94e` version.
2. Reverse-path payload (`write.go:203`): same 9 base keys plus
   `reversal_of_group_id`, via the `extra` merge loop; pinned by
   `posting_test.go:1288` (`decoded["reversal_of_group_id"]`,
   `decoded["event"] == EventCancel`).
3. Envelope (`write.go:443-456` vs base): `EventType` is the only
   parameterized field; `Workspace`, `Producer "posting"`, `SubjectType`,
   `SubjectID`, `SubjectRevision`, `Actor`, `CorrelationID` are field-for-field
   identical to the base's `events.Emit` call. The post path passes
   `OutboxEventType` = `events.TypePostingPosted` = `"posting.posted"`, the
   same string the base emitted. `events.Emit` (`events.go:90-94`) requires
   only a non-empty event type, so the reversed type needs no registry entry.
4. Consts (`posting.go:39-43`): `OutboxEventType` aliases
   `events.TypePostingPosted` (no fork). `OutboxEventTypeReversed` is a
   posting-local `"posting.reversed"` literal — the events package defines no
   `TypePostingReversed` const to alias (see follow-up 1). The value matches
   the reviewed slice exactly, so semantics are correct.
5. Call sites: both live in `write.go`, not `reverse.go` (the brief's wording
   aside): line 176 (post path, `OutboxEventType`, nil extra) and line 203
   (`writeReversal`, `OutboxEventTypeReversed`, `reversal_of_group_id` extra).
   `reverse.go` is byte-identical between `5bbc94e` and `981ee78` and reaches
   line 203 through the shared `commitPost` (`reverse.go:105`). Right type
   each.
6. Nothing else semantic: `reverse.go` byte-identical across the rebase;
   `posting_test.go` differs only by the base's added
   `TestPostEmitsEnvelopeAndOutbox` (C2 slice-2 content, pins the envelope the
   adaptation preserves); all remaining patch hunks are context-line shifts.
7. Z2 untouched: `ErrCorrectionWindowOpen` intact (`posting.go:64`,
   `reverse.go:209`, `posting.go:444`); no day counts, date arithmetic or book
   defaulting in the delta (the delta adds no such code — `grep` over the
   candidate `internal/posting` finds no new policy text).
8. No new rapid `.fail` seed: the three `.fail` files under
   `internal/{ledger,rules,events}/testdata/rapid/` are tracked in prior
   commits (e.g. `6be771e`), tree clean; no `internal/posting` rapid dir.

## Follow-ups (non-blocking)

1. Consider adding `TypePostingReversed = "posting.reversed"` to
   `internal/events/events.go` and aliasing `OutboxEventTypeReversed` to it,
   so both posting event types are owned by the bus package. Cosmetic: the
   current literal is value-correct and `Emit` accepts it.
2. Carried from the slice-3 review, unchanged by this delta: follow-ups 1–5
   of `review-slice-3.md` (same-op-id submit/cancel test gap, no
   concurrent-reversal test, `assertMirror` bijectivity, `checkIdentity`
   wording, earlier-round F-items).
