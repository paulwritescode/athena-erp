# Review: C2 slices 4–6 — workflows + delivery + events surface (split)

Verdict: PASS

Reviewer: r-c2. Worktree `/Users/iampaulking/projects/duchess/athena-erp-c2-r`,
detached at f4e57fd, verified clean on arrival (`git status --porcelain`
empty) and clean before every gate. Reviewed the 7-commit stack on 58948de
(b877fbc split brief, a1c42c5 sub-slice A, 85f3de1/adc7d5c/5fdf914/b640864
sub-slice B, f4e57fd wiring fix). No blockers. Follow-ups are numbered F1–F6
at the end.

## 1. Workflows (A) — PASS

`internal/events/workflow.go` (743 lines, new) matches G8/R8.7/R8.8 and
ADR-0028: instances carry definition version, phase, expected/received facts,
wait event + deadline, due time, assigned Role/Scope, cycle count/budget and
completion evidence (`Instance`, workflow.go:93-112); the schema agrees
(migration 17, `sysmigrations.go:617-654`, incl. `UNIQUE KEY uq_tasks_gate
(instance_id, task_key)` and cycle default 25 = `DefaultCycleBudget`).

- Gated creation: `insertTaskOnce` (workflow.go:583-592) turns a duplicate-key
  on the unique gate into "already there", so two racing signals create one
  task. `TestWorkflowSignalCreatesGatedTaskInEitherOrder`
  (workflow_test.go:123-159) pins both arrival orders; repeats cost no cycle.
- Cycle budget: `signalLocked` (workflow.go:246-306) stops past the budget
  with operator-visible `budget_exhausted` + `stopped_fact` evidence; terminal
  instances are no-ops. `TestWorkflowSignalStopsPastTheBudget`
  (workflow_test.go:161-211) pins stop, evidence, and no-op transitions after.
- Property 14: `TestGatedTaskCreationProperty` (workflow_test.go:386-498)
  draws gates/arrivals/racers over MariaDB, asserts exactly-one-task per
  satisfied gate and `CycleCount == distinct facts`. Reran green in this
  worktree (27.2s), plus the four other workflow tests green
  (creates-either-order, budget, wait/wake/due, dispatcher wake).

## 2. Wiring fix (tip f4e57fd) — PASS

- `WaitFor` now takes the classifier and `Declare`s the `workflows`
  subscription for the waited event type in the same transaction
  (workflow.go:370-398); `Serve` registers `WorkflowHandler` under
  `ConsumerWorkflows` (server.go:129-131), so the ticking dispatcher delivers.
- `TestWorkflowEventWakesWaitThroughDispatcher` (workflow_test.go:502-540)
  pins it end to end: WaitFor → Emit → `Dispatcher.DispatchNext` →
  wait cleared, fact recorded, gated task created. Reran green.
- No other consumer stays unwired: the only `Consumer:` in product code is
  `ConsumerWorkflows` (workflow.go:393); `Declare` has no other product
  caller; `Serve` is the only `EventHandlers` wiring and it holds workflows.
  (Test-only `probe` consumers wire their own dispatchers.)

## 3. Invite/reset delivery (A) — PASS, no secret material

`internal/auth/service.go` (+170): `CreateUserWithDelivery`, `ResendInvite`
and `StartReset` call `emitDelivery` in the same transaction; `manual`/`setup`
keep the show-once code path, any other channel enqueues `auth.invite_created`
/ `auth.reset_started` via `events.Emit`. Payload is destination alias, kind,
`challenge_id`, `expires_at` only — the Challenge value stays out (Z4),
with `UpdateUser` delivery validation (`checkDelivery`, 1–20 chars, `setup`
reserved, root immutable).

`internal/auth/outbox_delivery_test.go` (226 lines, new): destination mode
enqueues with the reference and asserts `!strings.Contains(payloadRaw, token)`
for invite and reset; manual mode enqueues nothing; the returned code still
completes the flow. Reran all four tests green. No token/password in spans or
logs (no otel in the auth service; no new log lines on the delivery path).

## 4. Events surface (B) — PASS

- `proto/athena/v1/events.proto` (new, own file): `EventService` (ListEvents,
  GetEvent, ListOutbox, RequeueOutbox, ListSubscriptions) + `JobService`
  (ListJobs, GetJob, RetryJob, ListOccurrences) = 9 procedures; messages carry
  ids/counts/JSON, never full Record values. `make generate` twice with no
  diff (tree clean after both; Buf on PATH, node via nvm for web-deps).
- Handlers (`internal/server/events.go`, 545 lines, new): all 9 procedures
  implemented with `requireSystem(build)`; `RequeueOutbox` re-emits through
  `events.Emit` (refuses non-dead and legacy rows, keeps correlation, names
  the dead event as cause); `RetryJob` re-enqueues through `jobs.Enqueue`;
  both leave the dead row as history (asserted in
  `internal/server/events_test.go`: "dead row moved" checks).
  `TestQueueProceduresNeedBuild`, list/page/get, subscriptions, occurrences,
  and crash tests cover the surface.
- Gate policies: 9 entries appended at the END of `procedurePolicies`
  (`gate.go`, after the Ledger block), all `policySystem` + `build`.
  `TestEveryProcedureHasAPolicy` reran green. Mount in `server.go` is two
  additive register lines.
- CLI: `cmd/athena/jobs.go` (list/show/retry/occurrences) and
  `cmd/athena/events.go` (list/show/outbox/requeue/subscriptions) call the
  handlers as the operator and print next page tokens; `main.go` change is
  wire + help text only. NO secrets CLI (`grep secrets cmd/athena` empty).

## 5. Desk (B) — PASS

- Jobs page REPLACES the stub: same file `web/src/pages/JobsPage.vue`
  (716 lines; old stub was 13 lines), no duplicate page/route; tabs for jobs,
  events, outbox, schedules, subscriptions with server keyset paging, status
  filters, detail sheets, dead-row retry, Sonner toasts. `registry.ts` and
  `App.vue` untouched (brief allowed registry props, none needed).
- Settings EXTENDS: `SettingsPage.vue` diff keeps every existing section and
  adds a build-gated Background work section (recent dead jobs/rows + Open
  Jobs), with loading/error/empty states. No secrets UI; Z4 comment present.
- `web/src/api.ts`: two client exports appended at the end. Web tests
  `jobsPage.test.ts` (5) + `settingsWork.test.ts` (3) ran green in the full
  run. Design posture: semantic tokens only (no literal colors — the tokens
  test scans components and passed), PageHeader/DataTable/DetailSheet/
  ActionButton/Badge/Tabs, Lucide icons with words, loading/error/empty per
  tab. Both themes proven by screenshots (item 8).

## 6. Docs (B) — PASS, accurate

b640864: AGENTS.md gains the durable-events row (9 procedures, CLI names,
dead-rows-stay, Z4 open) and Jobs in Build; README gains the jobs/events row;
overview gains the events/jobs/workflows row and drops the stale "no
workflows yet"; ADR-0028 stays Proposed with a status-only implementation
note; decision-log C2 row lists implemented slices with 8.8/8.9/8.10 deferred.
All claims match the code.

## 7. Crash-between-commit-and-dispatch — PASS

`TestCrashBetweenCommitAndDispatch` (`internal/server/events_test.go:442-484`)
covers the window: commit without dispatch → next `DispatchNext` delivers →
inbox claimed → forced redelivery skips the effect (applies once). Reran
green. (R8.5; the dispatcher claim/recover path itself is pre-existing.)

## 8. Browser evidence — PASS (12/12 eyeballed, harness claim verified)

Shots: c2-jobs-1280-dark/light, c2-jobs-375-dark/light, c2-jobs-sheet-1280-
light, c2-settings-1280-dark/light, c2-settings-375-dark/light,
manual-jobs-1280-light, manual-settings-1280-dark/light. No shot shows
user-visible breakage. Findings:

- The caveat is half right: suite `c2-jobs-1280-light` and the jobs sheet ARE
  actually dark (headless), and manual-*-light are the true light shots — but
  suite `c2-settings-1280-light` IS truly light. Either way every page is
  proven in both themes: Jobs dark (suite) + light (manual), detail sheet
  (dark), Settings top (dark + true light), Background work section with
  listed failures (manual dark + manual light), 375 cards (dark loaded both
  pages; Jobs light loaded).
- `c2-settings-375-light` caught the page mid-load (skeleton placeholders).
  The loading state renders correctly; it is a harness-timing artifact like
  the one below, not app breakage — but it does not prove loaded 375-light
  (covered instead by loaded 375-dark + loaded 1280-light + token-only
  styling). Not a blocker; see F5.
- Suite log `1124-033604-sw-c2-2.log` (final run on f4e57fd): build green,
  `ok` on sign-in, five tabs, seeded jobs, all tabs load, retry enabled,
  `ok: no page errors` (zero app fails), and the ONLY failure is
  `FAIL: settings lists failures`. The harness
  (`/tmp/athena-c2-surface/cdp/drive.mjs:222-223`) waits for the 'Background
  work' heading then asserts the failure text with NO wait for the async
  `loadWork()` RPCs — a no-wait assert race. The manual settings shots prove
  the failures DO render given time. Characterization VERIFIED; harness
  issue, not app breakage. Earlier runs died on sign-in/theme-toggle timeouts
  (environment flakiness), superseded by the final run.

## 9. No-migration / no-secrets / Z2 / lanes — PASS

- NO migration: `git diff 58948de..f4e57fd -- internal/server/sysmigrations.go`
  empty. (Claim verified, not just trusted.)
- NO secrets/key/disposal code or tables: every secret/retention/disposal
  mention in the diff is a deferral note in docs/comments/split brief; no new
  tables, no `internal/secrets`, no key/AES code. 8.8/8.9/8.10 absent. Z4
  open in AGENTS.md, README, overview, ADR-0028, decision log.
- Z2 untouched: no `internal/posting` or `internal/ledger` changes.
- No other lane's files: 28 changed files are all in the split allowlists
  (A: workflow.go/workflow_test.go/service.go/outbox_delivery_test.go;
  B: events.proto + generated, server events/events_test, gate/server appends,
  jobs/events CLI + tests + main wiring, JobsPage replace, SettingsPage
  extend, api.ts append, 2 web tests, 5 docs) plus split.md itself. Shared
  files (`gate.go`, `main.go`, `api.ts`) are end-appends; `server.go` has the
  mount append (B) plus the EventHandlers wiring (tip fix, additive).
  `App.vue` untouched.

## 10. Gates — all green

- Tree clean before any gate; `gofmt -l cmd internal` empty.
- `go build ./...` OK, `go vet ./...` OK (this worktree).
- `affected` (absolute main-repo path): plan = full `make test` (proto
  fallback); suite reports "already green on f4e57fd (clean tree) by sm-c2
  at 02:33, log 1124-022635-sm-c2.log; not rerun" — reused per the brief.
  Read that log: all Go packages ok (auth/events/jobs/server/cmd/athena),
  web 624/624 passed incl. jobsPage (5) and settingsWork (3); no FAIL/panic.
- Narrow reruns in THIS worktree, all green: 5 workflow tests incl.
  Property 14 (105s), 4 delivery tests (19s), crash + policy tests (19s).
- `make generate` twice, no diff either time (node via nvm on PATH; first
  attempt without node failed on web-deps with Error 127 — environment, not
  code — then green twice).
- No full browser re-run per the brief; item 8 is the evidence.
- `internal/webui/dist` holds only the tracked `.gitkeep`; tree clean after
  all gates.

## Follow-ups (non-blocking)

- F1: `athena workflows` CLI is not built (R8.21 names
  `jobs|events|workflows|secrets|retention`; brief scoped this split to
  jobs+events, secrets+retention deferred on Z4). Workflow instances/waits/
  tasks have no CLI surface.
- F2: Workflow instances, waits and gated tasks have no Desk surface (Jobs
  page covers jobs/events/outbox/schedules/subscriptions; R8.20 names a
  Workflows page). The manual failure text proves the data path; the page is
  future work.
- F3: Per-subscription exhaust behavior is still row-level by design
  (`dispatch.go:227-231` comment: one row cannot express one outcome per
  consumer). Pre-existing limit, unchanged by this split; note for G10 Pack
  subscriptions.
- F4: Suite `c2-settings-1280-light` is truly light, so the worker's "suite
  1280 light shots are actually dark" caveat over-generalizes (true for the
  Jobs shots, false for Settings). Harmless; next brief need not repeat it.
- F5: Harness should wait for the Background-work list (not just its heading)
  before asserting, and the 375-light Settings shot should be retaken loaded;
  both are harness timing, already proven manually.
- F6: `dispatch.go` skips subscriptions without a registered handler
  silently (by design, "handlers are code"). If a Pack ever declares a
  subscription no deploy registers, the only signal is `dispatched_noop` in
  spans. Consider an operator-visible counter when G10 lands.
