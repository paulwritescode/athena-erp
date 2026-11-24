# Review brief: C2 slices 4–6 — workflows + delivery + events surface (split)

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-c2-r (detached at f4e57fd;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-c2/brief.md (slices 4–6) + split.md in
the same dir. Follow .kiro/specs/athena-beta/design.md section 5
reviewer workflow and AGENTS.md. Design: batch-2 design.md G8; R8.7/
R8.8/R8.11; ADR-0028. Z4 secrets/lifetimes are DEFERRED (8.8/8.9/8.10
out — verify absent).

Review the 7-commit stack on 58948de (oldest b877fbc split brief, then
a1c42c5 sub-slice A, 85f3de1/adc7d5c/5fdf914/b640864 sub-slice B, tip
f4e57fd wiring fix):

1. Workflows (A): instances, waits, gated task creation, cycle
   budget in `internal/events/workflow.go` match G8/R8.7/R8.8;
   Property 14 (gated task creation) rerun green.
2. Wiring fix (tip): consumer declared on WaitFor + Serve wiring
   so the dispatcher wakes waits (the split's own found gap);
   verify the integration test pins it and no other consumer
   stays unwired.
3. Invite/reset delivery (A): outbox path where a destination
   exists, manual code path otherwise (`internal/auth/
   service.go`); no secret material in rows/logs.
4. Events surface (B): `events.proto` (new — verify service/
   message shape + generate twice stable); handlers + gate.go
   policies (EVERY new procedure needs one — count them);
   `cmd/athena/{jobs,events}.go` (verify NO secrets CLI).
5. Desk (B): Jobs page REPLACES the stub (verify no duplicate
   page/route); Settings page EXTENDS the existing page/registry
   (verify not recreated); design-system posture (tokens,
   spacing, states, both themes).
6. Docs (B): overview/README/AGENTS.md/log rows accurate.
7. Crash-between-commit-and-dispatch test: brief requires it —
   verify it exists, covers the window, and passes.
8. Interim browser evidence: 12 shots in /tmp/c2-screens —
   EYEBALL ALL with the image tool. Caveats from the worker:
   scripted run failed ONLY on harness 'settings lists
   failures' (no-wait assert, zero app fails — verify that
   characterization in the suite log); suite 1280 'light'
   shots are actually dark (headless), manual-*-light are the
   true light shots. Rule harness-vs-real: any shot showing
   user-visible breakage or any app failure in the log =
   NEEDS_CHANGES.
9. Confirm: NO migration (sysmigrations.go untouched —
   verified claim); NO secrets/key/disposal code or tables
   (8.8/8.9/8.10 absent); Z2 untouched; no other lane's files.
10. Gates: gofmt -l cmd internal (empty), go build ./..., go vet
    ./..., .agents/bin/affected by the ABSOLUTE main-repo path
    (same lock; protos → full fallback — reuse of the worker's
    green run on f4e57fd is fine, read that log), make generate
    twice with no diff. Tree clean before any gate. No full
    browser re-run (item 8 is the evidence).

Merge-small rule: only blockers stop the merge (wrong
workflow/gating semantics, unwired consumer remaining,
missing gate policies, duplicated/recreated pages, secret
material, faked or failing browser evidence, red gates);
the rest are follow-ups. Write findings to
../athena-erp/.agents/tasks/beta-c2/review-split-4-6.md with
a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`,
blockers first with file:line. Then report: say master
"C2 SPLIT REVIEW: PASS or NEEDS_CHANGES <one line>". Then
wait for instructions. Do not edit product code, commit,
merge, push or start agents. Send UPDATE every 10 minutes.
