# B7 verification report (Muse verify worker, 2026-11-22)

Worktree: `/Users/iampaulking/projects/duchess/athena-erp-b7verify`, branch `feat/b7-verify`
(f6f2fc0). REPORT ONLY — no product code, tests or docs were changed.

Overall: **GAPS** — items 1, 2, 4, 5, 6 PASS; item 3 is a GAP (dead
`workspace-changed` path, no switch flow, no cache clearing).

Throwaway environment used for live checks (all dropped/stopped afterwards):
fake athena home `/tmp/b7verify-home`, workspace `test_athena_b7verify`
(database `test_athena_b7verify`, dropped), server on `127.0.0.1:3123` with a
fake user-HOME (separate install lock). The server on `:3000` and the dev
database were never touched. JUnit XMLs: `/tmp/athena-bruno-rate.xml`,
`/tmp/athena-bruno-idle.xml`.

## 1. Idle warning 2 min before expiry; any-tab activity resets it — PASS

- `WARN_BEFORE_MS = 2 * 60 * 1000` (`web/src/idleWarning.ts:13`);
  `rescheduleIdleWarning` opens the dialog exactly at expiry−2 min, or at once
  when already inside the window (`web/src/idleWarning.ts:31-45`).
- Dialog offers "Stay signed in" (primary, calls `Touch`, reschedules from the
  fresh `idleExpiresAt`, broadcasts `activity`) and "Sign out"
  (`web/src/components/IdleWarningDialog.vue:24-25`,
  `web/src/idleWarning.ts:90-107`). Mounted in `web/src/App.vue:183`.
- Local activity (`pointerdown`/`keydown`) dismisses the warning and broadcasts
  `activity` throttled to once a minute (`web/src/idleWarning.ts:75-81`).
  Every tab's receiver maps `activity` to `noteAlive()`
  (`web/src/composables/useSession.ts:136-137`), which stands the warning down
  and re-reads the truth with passive `GetSession` (never extends idle), then
  reschedules from the new expiry (`web/src/idleWarning.ts:56-73`).
- Semantics note (by design, matches "server is the authority",
  design.md §4): mouse-only activity dismisses the warning but the recheck
  reopens it if the server expiry is still inside 2 min ("wiggling the mouse
  is not activity the server counts", `web/src/idleWarning.ts:1-8`). A durable
  reset in all tabs happens when the other tab's activity extended server-side
  idle — the recheck picks the new expiry up. `Touch` is the one active call
  and is pinned non-passive by `TestPassiveProcedureCoverage`.
- Evidence: `CI=true pnpm vitest run src/sessionChannel.test.ts
  src/idleWarning.test.ts` → 12 passed (idleWarning 8/8, incl. "opens two
  minutes before idle expiry" and "staying touches once and reschedules").
- Observation (not a gap): no test pins the `useSession` receiver wiring
  (`activity`→`noteAlive`, `signed-out`→sign-out); both halves are unit-tested,
  the 3-line branch is verified by reading only. A `useSession` Desk test
  would close this.

## 2. "Keep me signed in" only for non-builders; remembered → 7-day absolute — PASS

- Server enforces the restriction: builders/root get the build idle and the
  flag is ignored; non-builders with `RememberDevice` get `hours = 168` and
  `Remembered = true` (`internal/auth/security.go:222-227`; root counts as
  build access via `HasSystem`, `internal/auth/principal.go:146-148`,
  `internal/auth/security.go:163-171`). Remembered sessions get a persistent
  cookie (`MaxAge`), others a browser-session cookie
  (`internal/server/gate.go:352-368`).
- Evidence (Go, all four cells): `go test ./internal/server/ -run
  'TestSessionPolicySnapshots|…'` with `ATHENA_TEST_DATABASE_URL` →
  `--- PASS: TestSessionPolicySnapshots` (ordinary+remember → 30 min idle /
  168 h absolute / Remembered; builder+remember and root+remember → 15 min /
  12 h / not remembered; `internal/server/session_policy_test.go:27-44`).
- Evidence (live, throwaway server :3123):
  - `administrator` + `rememberDevice:true` → 200, session cookie (no
    `Max-Age`), idle 0.25 h, absolute **12.00 h** (remember ignored).
  - Fresh ordinary user `orduser` (CLI invite + `CompleteChallenge`) +
    `rememberDevice:true` → `Max-Age=604799`, idle 0.50 h, absolute
    **168.00 h**.
- Desk offers the checkbox to everyone at sign-in
  (`web/src/pages/SignInPage.vue:139`), which is unavoidable and correct:
  builder status is unknown before authentication, and a pre-auth oracle would
  leak it. The help text discloses "Builders and root always use shorter
  sessions", and the server ignores the flag for them. "Offered only for
  non-builder sessions" holds in effect, enforced server-side and pinned by
  tests.

## 3. Workspace switch clears caches in all tabs — GAP

- There is **no workspace-switch flow in the Desk** (single-workspace server:
  one Workspace runs per user). Nothing in the product posts
  `workspace-changed`: repo-wide grep finds the string only in the
  `SessionChannelKind` type, the `readable` validator
  (`web/src/sessionChannel.ts:5,17`) and the receiver
  (`web/src/composables/useSession.ts:132`).
- The receiver does `window.location.reload()` and clears **nothing** — no
  Cache API, localStorage, lookup-label (`web/src/lookup.ts`), appearance
  (`athena-appearance`) or policy/search/data clearing, despite the playbook
  line ("A Workspace switch clears the prior policy, search and data caches
  before loading the next shell",
  `docs/revamp/Athena-ERP-Playbook.md:183`) and tasks.md B7 ("a Workspace
  switch clears caches").
- Suggested fix (pick one; owning file `web/src/composables/useSession.ts`,
  plus `web/src/sessionChannel.ts` for the message kind):
  1. If Desk workspace switching is planned: add a shared `clearDeskCaches()`
     (lookup labels, cached appearance, table/column prefs as appropriate),
     call it before reload in the `workspace-changed` receiver, post the
     message from the switch flow, and add a Desk test.
  2. If single-workspace servers stand: record a decision that B7's workspace
     bullet is descoped and remove the dead receiver/kind so the next reader
     is not misled.
- Severity: low — unreachable dead code, no user-visible misbehavior, since no
  switch exists to trigger it. But the checklist item as written does not hold.

## 4. Cross-tab sign-out covered by sessionChannel.test.ts — PASS

- Evidence: `CI=true pnpm vitest run src/sessionChannel.test.ts` → 4/4 pass:
  - BroadcastChannel delivery of `signed-out` + `activity` in order with
    numeric `at` (same-context jsdom delivery; cross-tab fan-out is browser
    semantics);
  - corrupt payloads (bogus kind, bare string) ignored;
  - storage-event fallback without BroadcastChannel (posts to
    `athena-session-message`, ignores other keys);
  - unsubscribe stops delivery.
- "Means what it says": yes for the channel transport. Limitation (follow-up,
  not a gap): the test pins message delivery, not the cross-tab sign-out
  *reaction* — `useSession.ts:128-131` (`applySession(undefined)` + "Signed
  out in another tab.") has no test (no `useSession` test file exists). A Desk
  test posting `signed-out` through the channel and asserting signed-out state
  + note would make the coverage literal.

## 5. Expensive-read limits against a real expensive procedure — PASS (+ C6 follow-up)

- Reports bucket IS exercised end-to-end against a real expensive procedure:
  `SystemService.VerifyAuditLog` is classified to `auth.SessionReports`
  (`internal/server/ratelimit.go:118`) and `TestRPCRateLimitsAndAudit`
  (`internal/server/session_policy_test.go:378-383`) calls it once OK, then
  asserts the second call within the window returns `ResourceExhausted` with
  `Retry-After` plus a redacted `rate_limited` Access Log row. Evidence: `go
  test ./internal/server/ -run TestRPCRateLimitsAndAudit` → `--- PASS`.
  Classification-only coverage exists too (`TestRateClassesAndBoundedBuckets`
  → PASS).
- `ReportService` / `RunReport`: no such service or RPC exists in `proto/`
  (only classifier strings in `ratelimit.go:118` and test fixtures) — there
  are no stubs to call, so this half is untestable now. Likewise no `rpc
  Export*` exists anywhere in `proto/`, so the exports bucket is covered only
  at bucket-unit level, never end-to-end.
- Verdict: PASS for B7 (a real expensive-read limit is proven live); per the
  brief, ReportService/RunReport/export end-to-end limits become a **C6
  follow-up, not a B7 blocker**.

## 6. Bruno 09-idle-expiry and 10-sign-in-limit on a throwaway Workspace — PASS

- Runner: `bru` 4.3.0 executed directly from the local pnpm store
  (`~/Library/pnpm/store/…/@usebruno/cli/4.3.0/…/bin/bru.js`, `bru
  --version` → 4.3.0) — zero network, no install; same runner and collection
  the Makefile uses. Target `http://127.0.0.1:3123`, throwaway workspace
  `test_athena_b7verify`, fake user-HOME (separate install lock; `:3000`
  untouched, verified 200 afterwards).
- 10-sign-in-limit (`--env-var checkRateLimit=true`): **PASS** — five
  `CompleteChallenge` attempts → 400 `invalid_argument`, sixth → **429
  `resource_exhausted`** with `retry-after > 0`; all six iterations'
  assertions pass.
- 09-idle-expiry (`--env-var checkIdleExpiry=true`,
  `security.session.build_idle_minutes = 5` via `athena security set`):
  **PASS** — 03 signs in, jumps to 09, waits out the 5-min deadline, then
  `GetSession` → **401 `unauthenticated`** with the `idle` detail; run
  summary 6/6 tests, 5 min wall time as designed.
- Non-B7 observation: in the rate run, unrelated request 05-List-Record-Types
  fails on the empty throwaway workspace (`ListRecordTypes` returns `{}` —
  protojson omits the empty `recordTypes` field — while 05 asserts an array).
  Environmental artifact of the empty workspace, not a B7 regression; all
  B7-relevant requests (01–04, 06–08, 09, 10) pass. A collection nit for its
  owner: tolerate `{}` or seed one Record Type on throwaway workspaces.

## Suggested follow-ups (none is a B7 blocker except the item-3 decision)

1. Item 3: implement cache-clearing + switch post, or record the descope
   decision and remove the dead receiver (`web/src/composables/useSession.ts`,
   `web/src/sessionChannel.ts`).
2. C6: end-to-end rate-limit coverage for ReportService/RunReport and a real
   `Export*` procedure.
3. Desk test for the cross-tab `signed-out` reaction and the
   `activity`→`noteAlive` wiring in `useSession`.
4. Bruno 05: tolerate `{}` on empty workspaces.
