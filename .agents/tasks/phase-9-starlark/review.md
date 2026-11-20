# P9 Starlark host review (iteration 1)
Verdict: APPROVED. No blockers.

Evidence (recorded results in notes.md, plus spot checks of source; suites not re-run):
- Limits: `internal/rules/rules.go` sets MaxExecutionSteps (200,000), a 500 ms timer that cancels the thread, and ctx cancellation. Floats, `load`, `while` and `/` are refused at compile.
- Host boundary: Rules reach data only through `ruleHost` (`rules_engine.go`). Get, Find and Set check Grants, row scope and hidden fields as the acting user. Set accepts only writable fields (not `docstate`, formula fields or unknown fields) and goes through `fieldArgument`. State checks apply when writes are merged. A failing rule rolls back the document.
- Invariant tests match the roadmap list: replay hash, step and time limits, no-float, rollback on error, permission-checked reads, no cross-field writes. All are listed in notes.md with passing results.
- Spans carry only rule key, type, event, request id, outcome, counts and duration. No Record values. The test checks this.
- Docs: ADR-0016 (Proposed), decision log, overview, README, docs/README.md and AGENTS.md all say Rules are not production-ready (no memory limit). The roadmap's "documented as not production-ready" requirement is met.
- CLI `athena rules` has subcommands, `--workspace` and `--json`, and a test. `--workspace` replaces `--project` per AGENTS.md and ADR-0008. No `--addr` is documented because no server starts. init/new/run/serve are unchanged.
- API is ConnectRPC/Protobuf only. Web uses shadcn Select/Switch/Textarea/Tabs, Sonner toasts, and nav that follows `rules:read`.
- No PostgreSQL claims or secrets were found. New files are new, not copies. Migration 11 renames the old unused table to `athena_rules_legacy` instead of leaving duplicate code.

Non-blocking notes: Set checks hidden fields but not a separate field write level. The save path already needs a Record write Grant. Formula fields are stale at `validate`. Both are documented caveats.
