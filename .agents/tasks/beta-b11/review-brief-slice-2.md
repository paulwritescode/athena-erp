# Review brief: B11 slice 2, Setup wizard + readiness view

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b11, branch
feat/b11-install (commit f0870ce). Brief:
../athena-erp/.agents/tasks/beta-b11/brief.md (slice 2 only). Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and
AGENTS.md. Design system: docs/design/athena-design-system.md.

Review the slice-2 diff (git diff 172c749...HEAD):

1. Setup wizard: Setup stub replaced by a six-step wizard (company,
   logo, base money, first period, owner invite, readiness) built from
   new setup components + B5 IdentitySection + direct InviteUser RPC.
   Steps mirror server validation and handle denied/load-error states.
2. Readiness view merges wizard state with GetReadiness books checks.
3. Slice-1 follow-ups folded in: README guide link, error timestamp
   note, e2e SLUG override; wizard documented in getting-started.
4. Walk the full wizard end to end on a THROWAWAY workspace (never the
   dev database in .env.local): all six steps, back/forward, denied +
   load-error states, owner invite delivery, readiness list. Cover 4
   sizes x 2 themes with screenshots; design-system states apply
   (rest/hover/focus-visible/pressed/disabled-with-reason/loading/
   error/empty).
5. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test with MariaDB enabled (throwaway test_athena_* only),
   make generate twice with no diff. If node is missing on PATH,
   source nvm first; do not skip checks.

Merge-small rule: only blockers stop the merge (broken wizard flow,
lost data, wrong books checks, red suite); the rest are follow-ups.
Write findings to
../athena-erp/.agents/tasks/beta-b11/review-slice-2.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: hc send master "B11 SLICE 2 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge,
push or start agents. Permission policy: stay inside athena-erp and
athena-erp-* folders, nothing destructive.
