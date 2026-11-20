# P7 review (expressions, cel-go)

Verdict: APPROVED. No blockers.

Evidence (recorded results read, suites not re-run):
- Roadmap invariants covered: determinism (TestDeterministic, 50 runs plus recompile), dependency order and cycle rejection at save, rollup equals recomputation after add/edit/delete, SQL translation equals in-memory evaluation (36 filters over rows with nulls). Null and money tests exist. Sandboxing is by construction (macros off, no floats/lists/maps, std lib limited, `today()` injected) with rejection tests (TestRejectedAtCompile, TestScopeTypeConfusion).
- Vocabulary: no stale old terms in the new files (grep). The one "project owner" phrase in ADR-0013 is the prose role, not the old term.
- CLI: `athena expressions check|eval|preview`, `records list --filter`, `--workspace` per AGENTS.md (the brief's `--project` is superseded by ADR-0008), `--json`, TestExpressionsCommand. init/new/run/serve unchanged.
- Docs: ADR-0013 (Proposed), D-017, decision-log, docs/README.md, overview, README updated; AGENTS.md rightly unchanged since nothing is accepted. PostgreSQL is stated untested.
- Telemetry: spans carry operation, type key, request ID and outcome; TestExpressionSpansNeverCarryRecordValues; off without the endpoint.
- API is ConnectRPC/Protobuf only. Frontend adds no components or hard-coded colours, uses Sonner, and fieldRules.ts changes have Vitest cases. No new copies or secrets; untracked files are the intended new ones.

Non-blocking note: the parser size (2000) and recursion (32) limits are configured but no test pins them. Expressions have no loops, so evaluation terminates regardless.
