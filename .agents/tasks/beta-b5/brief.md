# Brief: B5 Workspace identity and logo (Muse t1q)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-b5, branch feat/b5-identity. Server and Desk. Merge-small: slice 1 is the server and CLI; slice 2 is the Desk.
Read: requirements R5; design-system sections 10 and 18; design.md section 1 (brand) and section 3; tasks.md B5; internal/files; proto/athena/v1/workspace.proto (WorkspaceService exists, with appearance).
The two logo PNGs, `athena-black-logo` and `athena-white-logo` (1254x1254 RGBA, no extension), are copied into the worktree root. Rename them to `web/src/assets/brand/athena-mark-black.png` and `athena-mark-white.png` if they are not there already (B2 may have copied them under other names: reuse, do not duplicate), and remove the root copies from the worktree.

Slice 1, server and CLI:
- WorkspaceService.GetIdentity (public: name and logo URL only) and SetIdentity (permission settings:workspace, new). Fields: display name (1 to 80 characters), and an optional logo for light and an optional logo for dark (PNG, or SVG sanitized to remove scripts, event handlers, external references and foreignObject; at most 512 KB; image dimensions checked).
- Store through internal/files under brand/ with versions and audit.
- Serve GET /brand/logo-<hash>.<ext> same-origin with long cache headers, nosniff, and Content-Security-Policy sandbox for SVG.
- Also generate favicon PNGs (32 and 180 px) from the light logo, or the Athena mark when none is set.
- CLI: `athena workspace identity show|set --name --logo-light file --logo-dark file`, and `athena new --logo`.
- Tests: SVG sanitizing (script, onload, href to http, foreignObject), size and type limits, public Get returns nothing else.
Slice 2, Desk:
- Header, sign-in, tab title and favicon use the Workspace name and logo. The Athena mark is the fallback (black on light, white on dark). "Powered by Athena" goes in the user menu footer once a logo is set.
- Settings → Workspace identity: name, light and dark logo upload with preview, remove. Apply design-system section 10 sizes and clear space.
Checks per slice: gofmt, build, vet, make test, make generate stable; web typecheck, test, build; browser pass on a throwaway Workspace. Do not commit, merge, push or start agents.
Report: hc send t6 "B5 SLICE n: PASS or BLOCKED".
