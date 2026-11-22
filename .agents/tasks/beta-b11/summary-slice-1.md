# Summary: B11 slice 1, CLI install path + guide (merged b27b6c3)

## What shipped

`athena new` accepts `--driver`/`--logo` after the workspace name as well as
before (trailing flags no longer swallowed into the name); `--logo` reads
before the interactive prompt (bad paths fail fast); `athena run` rejects
positionals like `init` does. `scripts/e2e-install.sh` proves init/new/run
plus root sign-in, password rotation and first-owner invite on throwaway
homes/databases/ports (28 checks, all green, throwaway-only verified).
`docs/guides/getting-started.md` written from the tested path, every quoted
error and step verified against source, indexed from docs/README.

## History

- Worker w-b11 committed 2c30c84 (message reworded at merge, content
  identical).
- Review round 1 PASS: e2e 28/28 re-run green with throwaway-only proof
  (scratch gone, DB dropped, server stopped), guide line-checked against
  source (errors, seeding rule, scaffolding, desk refs), full suite green
  (529 web), 3 doc/nit follow-ups.
- Rebased cleanly onto main 3e1831c; all checks green; merged --ff-only,
  pushed, recorded slice line (slice 2 open, held for B8).

## Follow-ups (in tasks.md)

README link to the guide; timestamp prefix in quoted errors; SLUG override
for parallel e2e runs.

## Try it

`./scripts/e2e-install.sh` from the root (throwaway only); read
`docs/guides/getting-started.md` for the install → first-owner path.
