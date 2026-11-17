# Athena ERP documentation

This directory describes the product and the system as it exists and as decisions are made. The root [`AGENTS.md`](../AGENTS.md) is the short contributor entry point; this page is the map to the detailed docs.

## Current references

- [Architecture overview](architecture/overview.md) — product scope, component boundaries, current implementation status, and not-yet-built capabilities.
- [Record and Module model](architecture/record-and-module-model.md) — definitions, MariaDB mapping, Go/Starlark boundaries, code generation, and open lifecycle choices.
- [Decision log](decisions/decision-log.md) — accepted decisions, proposals, unresolved choices, and rejected assumptions.
- [ADR-0002: MariaDB first](decisions/adr-0002-mariadb-first.md) — initial database selection and current persistence limits.
- [ADR-0003: Record table model](decisions/adr-0003-record-table-model.md) — Record definitions map to MariaDB tables.
- [ADR template](decisions/adr-template.md) — format for recording a consequential architecture decision.
- [Project README](../README.md) — setup, run, build, and repository layout.

## Documentation rules

- Label statements as current implementation, accepted direction, proposal, or open question when that distinction could be unclear.
- The decision log is the source of truth for decision status. `AGENTS.md` summarizes it for agents and must be updated in the same change when accepted decisions or key open choices change.
- Record the reason, consequences, and reconsideration trigger for important choices. Do not copy unverified claims from brainstorming into accepted decisions.
- Keep docs close to code changes and verify file paths and commands against the repository before publishing them.
