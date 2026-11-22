# Review brief: B8 slice 1, root administrator server + CLI

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b8, branch
feat/b8-root-buildview (commit 6c702a3). Brief: ../athena-erp/.agents/
tasks/beta-b8/brief.md (slice 1 only). Follow .kiro/specs/athena-beta/
design.md section 5 reviewer workflow, AGENTS.md (root paragraph) and
ADR-0018 (Accepted — confirm behavior, do not redesign it).

Review the branch diff against main (git diff main...HEAD):

1. Root is created at `athena new` / first boot with the database password
   (first-boot seeding covered, no other account can take the name).
2. Root bypasses all gates (HTTP gate bypass proven, not just unit-called).
3. Root cannot be suspended, edited (display name), role/assignment-changed
   or deleted through the API, the CLI or the Desk — one test per path;
   API/CLI/Desk share one refusal with the existing suspension guard
   (UpdateUser + SaveAssignment + gate + CLI), errors are plain language.
4. Every root action is audit-logged as root (attribution asserted);
   change-password rotates only root's own credential.
5. No new permissions, migrations or ADRs (none expected); no scope creep
   beyond the refusals + tests.
6. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test (throwaway test_athena_* databases only, never the dev
   database in .env.local), make generate twice with no diff. No browser
   pass needed (Desk row/sheet refusals are unit-covered; slice 2 does the
   Desk browser pass).

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-b8/review-slice-1.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "B8 SLICE 1 REVIEW: PASS
or NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
