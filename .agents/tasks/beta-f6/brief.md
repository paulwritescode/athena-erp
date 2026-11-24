# Brief: F6 live ASCII Athena logo (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-f6, branch feat/f6-logo (from latest main).
Read: tasks.md F6; docs/design/athena-design-system.md (brand + hairline/ASCII-art rules); `web/src/assets/brand/` (the actual logo artwork — light + dark originals); `web/src/art/registry.ts`; the sign-in, first-run, 404 and offline surfaces; `cmd/athena` no-args and `version` output paths.

Scope (one worker; report once with say master "F6: PASS or BLOCKED <one line>"; rebase on main before reporting PASS and say if the rebase changed anything):
1. Frames: generate ASCII frames FROM the artwork (luminance to a character ramp; separate light and dark variants). Never letters spelling the name. Commit the generator script (new, under `web/scripts/` or `tools/`) plus the generated frames so the art is reproducible; document the ramp, sizes and how to regenerate.
2. Motion: animate (draw-in or shimmer, 120–240 ms steps, transform and opacity ONLY, static under `prefers-reduced-motion`). Register in `web/src/art/registry.ts` for sign-in, first-run, 404 and offline, at most one per screen.
3. CLI: static banner for bare `athena` and `athena version`. F8 (later, same lane) owns build flags/version lines — put the banner ABOVE the version lines and touch nothing else in version output.
4. Checks: 375/768/1280/1440 px in both themes for every touched surface (screenshots to /tmp, paths in report); full browser pass through suite; typecheck, touched Vitest, vite build.

Files allowed: `web/src/art/**`, `web/src/assets/brand/**` (read + add frames/generator), the four surfaces' Vue files (minimal wiring only), `web/src/art/registry.ts`, `cmd/athena` main/version output (banner only), tests. NOT allowed: gate.go, protos, migrations, App.vue (ask master), RecordsPage.vue (C1 6.10 owns it now), RecordTypesPage/FieldSettingsDialog/fieldRules (L3), api.ts.

Rules: design-system posture (tokens, spacing, Satori/Inter scope, reduced-motion); telemetry off unless the endpoint is set. Throwaway Workspaces/test_athena_* DBs only, never the dev database; fake HOME for any server. Browser ports/DB (parallel passes run tonight): use server port 3120 + CDP 3791 and test database `test_athena_f6_browser` — yours alone, never another lane's. Full suites and browser passes only through .agents/bin/suite (it queues across 2 slots; never bypass the lock). Package tests (one Vitest file) may run directly. Commit on your own branch feat/f6-logo (restore internal/webui/dist/.gitkeep before committing); you never merge, push or start agents.
Checks: gofmt -l cmd internal (empty), go build ./..., go vet ./..., CI=true pnpm typecheck + touched Vitest + vite build, one full `make test` through suite at the end.
Report: say master "F6: PASS or BLOCKED <one line>" + commits + screenshot paths + regeneration doc path. Then wait for instructions.
