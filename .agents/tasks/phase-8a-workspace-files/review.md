# P8a review (pass 1)
Verdict: APPROVED. Recorded evidence read; suites not rerun.

- Invariant tests (notes): path escape table shared Go/TS, symlinks, hidden/.env/.git, size/type limits, stale hash, atomic-write fault, version history, one audit row per change without content, denial logging, no content in spans/logs. All match the roadmap list.
- Authorization: gate.go lines 63-68 give all six FileService procedures a policy (files:read / files:write); TestEveryProcedureHasAPolicy covers them.
- Telemetry: spans carry hashed path id, byte and file counts only.
- CLI: list|get|put|mv|rm|history with --workspace and --json, tested. No --addr, consistent with the other kernel commands (no server started); the roadmap does not require it. init/new/run/serve unchanged.
- ADR-0015 is Proposed. The decision log has an ADR-0015 row, O-002 carries the proposed answer and stays open, and D-009 stays unchanged until acceptance, as the roadmap requires.
- Docs: README, docs/README, overview, model doc and AGENTS.md updated. No PostgreSQL claims (noted untested).
- Frontend: FilesPage uses shadcn components, tokens, Sonner toasts, a mapping tab, and filePaths.ts parity via a shared table. fieldRules.ts is unchanged because no field rule changed.
- No dependencies added, no copies or leftover code found. No blockers.
