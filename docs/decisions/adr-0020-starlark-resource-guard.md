# ADR-0020: Starlark resource guard

- **Status:** Proposed
- **Date:** 2026-11-20
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0016, D-004

## Context

ADR-0016 left one acceptance precondition: no memory limit, because starlark-go has no allocation accounting. It named three ways out, one of which was "a static restriction that closes the cases". Batch-2 group G3 took that option plus caps everywhere the host can see. One correction to the requirement text along the way: Starlark has no `**` operator, so the `[0] * 10**9` example does not parse; the real literal form is `[0] * 1000000000`, and it is rejected when the Rule is saved.

## Options considered

1. **Host-boundary caps plus a static repeat ban (chosen).** Everything a Rule receives from or hands to the host is length-checked and counted; sequence repetition of a literal never compiles; decimal products are digit-capped. No new dependency, no new process.
2. **Allocation hook upstream.** starlark-go has no such hook; this would be a fork or an upstream feature.
3. **Rules in a separate process with a memory limit.** Closes every residual (see below) but adds a supervised process and a serialization boundary; deferred, not rejected.

## Decision (proposed)

- `rules.Limits` gains `MaxValueLen` (10,000), `MaxValues` (50,000), `MaxRows` (100) and `MaxCascadeDepth` (3). New codes: `memory_limit`, `row_limit`, `cascade_limit`. Over-reads now report `row_limit` instead of `limit`; the cascade guard reports `cascade_limit` instead of `cascade`.
- Lengths are checked on values the Rule builds (set arguments, filter values, transform results, allocate weights and results, decimal text); host data (document fields, rows read) is counted but not length-capped, so long stored text still reads. Every host-built value counts toward `MaxValues`, reported as `Outcome.Values`.
- The save-time check rejects `*` with a list, tuple, string or bytes literal or an `allocate()` call on either side. Repeats through variables that reach the interpreter's own pre-allocation guard (2^30 elements) map to `memory_limit` at run time.
- Decimal products stop past 130 digits, so repeated squaring cannot double past memory; integers past 256 bits fail before their text is built, everywhere they convert to decimals or cross to the host.
- Saved runs and `SimulateRule` share the one `execute` path and its `DefaultLimits`; the engine clamps `find` limits again at the SQL layer, and every code lands in the Rule Run row.
- Residual risks the host cannot see (no interpreter hook exists): integer `*` growth in a loop, string/list `+` doubling, repeats through variables under 2^30 elements, and `list(range(n))` materialization. Seeds stay small (64 KiB source, counted host values) and nothing oversized can leave through `set`, filters or results, but a hostile Rule can still spend memory inside the interpreter. Closing those needs option 2 or 3.

## Consequences

- Amends ADR-0016: the memory gap is closed as far as the host can see. ADR-0016 and this ADR stay Proposed until the sponsor accepts them; `rules:write` stays gated like `build`.
- The Rules page shows the new codes from run rows without changes; no migration was needed.

## Revisit when

The sponsor accepts Rules for production, upstream gains allocation accounting, or a hostile-Rule threat model forces the host out of process.
