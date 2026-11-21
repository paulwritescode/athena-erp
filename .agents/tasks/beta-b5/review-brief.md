# Review brief: B5 slice 1, Workspace identity server + CLI

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-b5, branch feat/b5-identity
(commit 2e5595e). Brief: ../athena-erp/.agents/tasks/beta-b5/brief.md
(slice 1 only). Follow .kiro/specs/athena-beta/design.md section 5
reviewer workflow, AGENTS.md and .kiro/steering/athena-design.md.

Review the branch diff against main (git diff main...HEAD):

1. WorkspaceService.GetIdentity is public and returns name + logo URLs
   only (nothing else leaks); SetIdentity requires permission
   settings:workspace (new, in SystemPermissions, not in BuildAccess).
2. Logo validation: PNG or SVG only, ≤512 KB, image dimensions checked;
   SVG sanitized (no scripts, event handlers, external references,
   foreignObject) — this is a security surface, probe it.
3. Storage through internal/files under brand/ with versions and audit;
   GET /brand/logo-<hash>.<ext> same-origin with long cache headers,
   nosniff, Content-Security-Policy sandbox for SVG.
4. Favicons (32 + 180 px) generated from the light logo, Athena mark
   fallback; root logo copies removed from the worktree, B2 brand assets
   reused not duplicated.
5. CLI: `athena workspace identity show|set --name --logo-light --logo-dark`,
   `athena new --logo`; plain-language errors.
6. Proto additive only; procedurePolicies entries; `make generate` twice
   with no diff; migration/ADR numbers only if used (none expected).
7. Run: gofmt -l cmd internal (empty), go build ./..., go vet ./...,
   make test (throwaway test_athena_* databases only, never the dev
   database in .env.local), plus a CLI end-to-end on a throwaway Workspace.
   Note: only one Athena server runs per $HOME (install lock in
   internal/workspace/lock.go); other agents hold the real-HOME lock, so run
   any throwaway server with a fake HOME (e.g. HOME=/tmp/athena-b5-fakehome)
   and never stop another agent's server.

Merge-small rule: only blockers stop the merge (breaks behavior, loses
data, opens a security hole); everything else is a follow-up. Write
findings to ../athena-erp/.agents/tasks/beta-b5/review.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first with
file:line. Then report: hc send master "B5 SLICE 1 REVIEW: PASS or
NEEDS_CHANGES <one line>". Do not edit product code, commit, merge, push
or start agents. Permission policy: stay inside athena-erp and
athena-erp-* folders, nothing destructive.
