# Brief: B11 install and first-run flow (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b11, branch feat/b11-install (from main 0afe93c).
Read: tasks.md B11; AGENTS.md runtime shape (init/new/run, home, Workspace-per-user); README install path; cmd/athena init/new/run; design-system §10 (logo), §13 (touch), §14 (actions) for the wizard.

Document and test the path: install binary → `athena init` → `athena new "Name" [--logo]` → `athena run` → sign in as root → Setup wizard (company, logo, currency, period, invite first owner) → readiness view. Every step has a CLI and Desk equivalent and a plain-language error.

Slices, each merging on its own (report after each):
1. CLI path + docs: verify and fix `init`/`new`/`run` end to end on throwaway homes (flags, --logo, errors, first-boot root seeding handoff); write `docs/guides/getting-started.md` FROM the tested path (every command copy-paste reproducible). Keep B5 identity + B8 root behavior as is — no redesign.
2. Desk Setup wizard + readiness view (starts only when the master says so): wizard steps (company, logo upload, currency, period, invite first owner) + readiness checklist. NEW wizard components only — do NOT touch AccessPage, App.vue, shell or mode files (B8 owns them until it merges); reuse the B5 IdentitySection read-only and call InviteUser RPC directly from the wizard.

Rules: follow the design system + steering for the wizard (everyday words, one primary action, all states, both themes, 375/768/1280/1440). Test on throwaway homes/Workspaces only (install into /tmp scratch + test_athena_* databases), never the dev database or the real home. Commit on your own branch feat/b11-install; you never merge, push or start agents.
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test, make generate stable; CI=true pnpm --dir web run typecheck, test (exit 0), build for slice 2; CLI e2e script for slice 1; browser pass for slice 2 with screenshots in .cache/screens/b11-*.
Report: hc send master "B11 SLICE n: PASS or BLOCKED <one line>".
