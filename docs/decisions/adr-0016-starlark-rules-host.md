# ADR-0016: Starlark Rules host on go.starlark.net

> **Amended by [ADR-0020](adr-0020-starlark-resource-guard.md) (Proposed).** The memory gap is closed as far as the host can see: value lengths, value counts, rows per call and cascade depth are capped with `memory_limit`, `row_limit` and `cascade_limit` codes. Both ADRs stay Proposed until the sponsor accepts them.

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** D-004, ADR-0007, ADR-0011, ADR-0012, ADR-0013, ADR-0014, ADR-0015, ADR-0020, spec sections 12.1 and 13, roadmap phase P9 and user answer 4

## Context
Some business logic needs sequencing, reads across Records and branching, which the expression language (ADR-0013) deliberately cannot do. D-004 says Go owns trusted behavior and Starlark supplies configurable rules. A Rule must never bypass Go's Grants, invariants or transaction boundaries, must terminate, must not use binary floats for money, and must be replayable.

## Options considered
1. **Lua (gopher-lua).** Mature, but not deterministic by default, has floats and a bigger standard library to remove.
2. **JavaScript (goja).** Familiar, but a large surface (floats, `Date`, `Math.random`) to strip, and a larger dependency.
3. **go.starlark.net (chosen).** A Python-like language designed for configuration. Deterministic by design, no I/O or clock in the language, hermetic execution, frozen module values, no recursion or `while` unless enabled, and an execution step counter with cancellation. Cost: one dependency, and no allocation limit (see Consequences).
4. **Only Go hooks.** Safe, but every change needs a build and a deployment, which D-004 rules out for administrator-configured logic.

## Why go.starlark.net fits this runtime
- Pure Go, no cgo, no process to supervise; the single binary stays a single binary (ADR-0001).
- The module has no tagged release. `go.mod` pins the pseudo-version `go.starlark.net v0.0.0-20260908191801-89a6a09411d5`. Its own requirements (`golang.org/x/sys`, `golang.org/x/term`, `google.golang.org/protobuf`, `go-cmp`, `readline`) are already satisfied by the repository's versions or are not built into the binary (`readline` serves only the interpreter command).
- It imports nothing the Rule can reach: the universe holds no file, network, time or random function.

## Decision (proposed)
- **Package.** `internal/rules` hosts Starlark. It knows nothing about storage or permissions. A `Host` interface (get, find, set, currency) is implemented by `internal/server/rules_engine.go`, which performs every read as the acting user and validates every write.
- **Rule types built.** Validation (events `validate`, `before_submit`), Derivation (`validate`), Guard (`before_submit`, `before_cancel`, `on_trash`) and Transform (called, not tied to an event). Posting arrives with the posting engine (P12), Reaction and Scheduled with jobs (P16). Rules attach to **document** Record Types only, because lifecycle events fire only for documents.
- **Run order.** Within one event: Derivations, then Validations, then Guards, then by priority, then by key. A derivation's `set` is visible to the rules after it and to the formulas that follow, and the writes are merged into the save through the same checks as a user's save (submitted documents, set-once fields, field permission level, field validation).
- **The language.** Starlark with `set`, `while`, top-level control flow, global reassignment, recursion and `load` off. Floats are rejected when the Rule is saved (float literals, `float()`, `/`, `/=`) and at run time (any float passed to a host function). Numbers that matter are `decimal` (exact, `internal/types`) and `money` (integer minor units plus currency, `money(minor, "USD")`); `div(a, b, scale)`, `round(d, scale, mode)` and `allocate(money, weights)` do exact math. `ctx.now` and `ctx.today` are injected, never read from the clock. Dates are ISO text.
- **Host functions.** Read: `get(record_type, id)`, `find(record_type, filters, limit)`, `exists(record_type, filters)`. Write (derivation only): `set(field, value)`. Control: `fail(code, message)`, `warn(code, message)`, `require(condition, message)`. Observability: `log.info`, `log.warn`, `log.trace`. A function a type may not use is not defined for it, so calling it fails when the Rule is saved. A `doc.<name>` that is not a field, and a literal `set("name", ...)` of a field the Rule may not set, also fail on save with a line number.
- **Permissions.** Every read is checked as the acting user: the user's read Grant on the Record Type, the row scope of that Grant, and field permission levels (a hidden field is an error to read, never a null). A Rule that cannot read what it asks for fails with the code `permission_denied`. `set` refuses fields the user may not write, formula fields, unknown fields and system fields. The CLI acts as the operator without Grants, as everywhere else.
- **Limits per run** (`rules.DefaultLimits`): 200,000 execution steps, 500 ms wall time, 200 rows read, 50 fields set, 50 messages, 64 KiB source. A run that reaches a limit stops with `step_limit`, `time_limit` or `limit` and rejects the save. The cascade guard (`rules.Stack`, depth 3, no Rule twice) exists in the dispatcher but cannot trigger in this phase, because a Rule cannot cause a write that fires another event. Amended by ADR-0020: plus 10,000 value length, 50,000 values, 100 rows per call and cascade depth 3 in `Limits`, a save-time repeat ban, and `memory_limit`, `row_limit` and `cascade_limit` codes (over-reads moved from `limit` to `row_limit`, the cascade guard from `cascade` to `cascade_limit`).
- **Transactions.** Rules run inside the transaction of the save, through the lifecycle registry (ADR-0011). A rejection or an error returns an error to the caller and the transaction rolls back, so the document and the Rule's writes disappear together.
- **Storage (system migration 11).** `athena_rules` (current version; the earlier table of that name was never used by code and is kept, with any rows, as `athena_rules_legacy`), `athena_rule_versions` (every save, with source) and `athena_rule_runs`. A save that changes nothing is not a new version. Rolling back means saving an earlier version's source again. Each save or delete writes an Audit Log row (`athena:rule`) with the source hash and attributes, never the source.
- **Rule Run log (spec 12.1).** One row per run: rule, version, event, SHA-256 of the input, outcome (`ok`, `rejected`, `error`), code, message, steps, rows read, fields set, duration, dry-run flag, actor and `request_id`. It is written on its own connection, so a rejected save still leaves its row. It holds a hash of the input, never Record values; the input includes the source hash, the version, the event, the document, the injected time and the user, so the same input gives the same hash and the same outcome (replay). Messages from `log` and `warn` are stored with the run; because a Rule can read only what the acting user can read, they hold no value above that user's level.
- **Simulate.** `RuleService.SimulateRule` runs a saved Rule or an unsaved draft against a stored Record and supplied values in a transaction that is rolled back, and returns the outcome, a diff of the fields it would set, warnings, log lines, the step count and the input hash. The run is logged as a dry run.
- **API and permissions.** `RuleService` (ConnectRPC, `proto/athena/v1/rules.proto`). `rules:read` lists Rules, versions and runs; `rules:write` saves, deletes and simulates. Treat `rules:write` like `build`: a Rule runs on every matching save. Denials go to the Access Log.
- **Spans.** One span per run (`athena.rule_run`) carries the Rule key, type, event, request ID, outcome, code, step count, rows read, fields set and duration. Never values, source or messages.
- **Source files.** The Files page stores `rules/*.star` text, but the Rule that runs is the one saved through `RuleService` or `athena rules save --file`. Keeping the two in step is the publisher work of ADR-0015, which is not built; this phase does not read `rules/` at run time.

## Consequences
- **No memory limit.** starlark-go has no allocation accounting, so a Rule such as `[0] * 10**9` can allocate a large amount in one step before the time limit stops it. This is why Starlark Rules are not production-ready and why only users with `rules:write` may save one. Before this ADR is accepted, one of these is needed: an allocation hook upstream, running Rules in a separate process with a memory limit, or a static restriction that closes the cases.
- A Rule's reads inside the transaction use the same connection as the save, so a Rule that reads many rows holds the save open; the row limit bounds that.
- A formula or rollup field is calculated after the events of the save, so a Rule at `validate` sees the formula's old value. Use the inputs of the formula instead.
- `qty()`, `fx()` and `create_draft()`/`link()` of spec 13.2 are not built. They need units of measure (P20), currencies (P11) and the posting engine.
- Rule tests as Records (spec 13.4) and approval of Rule changes by a role are not built; simulate and versions cover the dry run and rollback.
- Every save of a Record Type that uses a Rule on it does not re-check the Rule. A Rule that names a removed field fails at its next run with `compile`, and the Rule Run log shows it.

## Revisit when
- The memory limit gap is closed or the Rule host moves out of process.
- A phase adds Rules that write other Records (Reaction, Posting), which makes the cascade guard reachable.
- A Pack must ship Rules, which needs the publisher of ADR-0015.
