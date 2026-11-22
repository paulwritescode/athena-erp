# Review brief: B11 slice 1, CLI install path + guide

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b11, branch feat/b11-install
(commit 2c30c84). Brief: ../athena-erp/.agents/tasks/beta-b11/brief.md
(slice 1 only). Follow .kiro/specs/athena-beta/design.md section 5 reviewer
workflow and AGENTS.md (runtime shape: init/new/run, home, one Workspace
per user).

Review the branch diff against main (git diff main...HEAD):

1. `athena new` accepts `--driver`/`--logo` after the workspace name as
   well as before (trailing flags no longer swallowed into the name);
   `--logo` is read before the interactive name prompt so a bad path fails
   fast; `athena run` rejects positional arguments. Errors are plain
   language. No behavior change to init/new/run beyond these fixes; B5
   identity + B8 root behavior untouched.
2. `scripts/e2e-install.sh`: init/new/run + root sign-in, password rotation
   and first-owner invite on throwaway homes, databases and ports (28
   checks). Run it (it must use throwaway state only — verify no dev
   database, no real home) and confirm 28/28 green.
3. `docs/guides/getting-started.md` is written FROM the tested path: every
   command copy-paste reproducible from the repository root; README/docs
   index rows added. Read it against the script for drift.
4. Scope: CLI + script + docs only; no Desk, proto, migration or server
   changes.
5. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test (throwaway test_athena_* databases only, never the dev
   database in .env.local), make generate twice with no diff. No browser
   pass (slice 2 owns the Desk pass).

Merge-small rule: only blockers stop the merge; the rest are follow-ups.
Write findings to ../athena-erp/.agents/tasks/beta-b11/review-slice-1.md
with a verdict line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers
first with file:line. Then report: hc send master "B11 SLICE 1 REVIEW: PASS
or NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and athena-erp-*
folders, nothing destructive.
