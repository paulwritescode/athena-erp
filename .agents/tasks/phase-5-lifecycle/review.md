# P5 lifecycle review

Verdict: APPROVED. No blockers.

Evidence read: roadmap P5, notes.md (recorded verification: build, vet, gofmt, go test, make generate idempotent, web typecheck/build/test with 67 cases, browser pass in both themes), the diff of records.go, main.go, and lifecycle.go in full. Suites were not rerun.

- Invariant tests (roadmap list): all present in `internal/server/lifecycle_test.go` and `internal/lifecycle/lifecycle_test.go`: immutability of every non-allow_on_submit field, cancel blocked by a live downstream link, amend keeps the original cancelled, event order equals `lifecycle.Sequence`, rollback of state/versions/activity/history when `on_submit` fails, stale revision rejected, on_trash only for drafts, span attributes carry no values.
- Behavior: transitions run in one transaction with a FOR UPDATE row lock and revision check. Update, delete and child edits check state under the same lock (`checkParentEditable` locks the parent). Amend refuses a second amendment and names X-1, X-2.
- Vocabulary: new code uses Record Type, Record and Workspace. A term search of the new files finds no old terms. `--workspace` is used in the CLI.
- CLI: `athena records submit|cancel|amend|versions|activity` is in the help text and has a test. Init/new/run/serve are unchanged. `--addr` does not apply because no server starts, which the notes state.
- API: ConnectRPC/Protobuf only (new RPCs on `RecordService`). No JSON endpoints were added.
- Telemetry: spans carry record type key, request ID, state from/to and outcome only. They stay off unless the endpoint is set.
- Docs: ADR-0011 (Proposed), D-015 (Proposed), `docs/README.md`, the overview and README are updated. AGENTS.md needs no change because nothing accepted changed. The notes and docs do not claim PostgreSQL support.
- Frontend: notes record semantic tokens, Sonner toasts, alert-dialog confirmations, Build/View unchanged, and `fieldEditable` in fieldRules.ts with Vitest cases.
- No secrets and no copies or leftover replaced code. The `insertRow` and `childRecordTypes` extractions replaced the inlined code.

Non-blocking, owner-level: open points 1 to 5 in notes.md (for example, amend fails when a required field is also unique, read-only or no-copy). They are documented trade-offs.
