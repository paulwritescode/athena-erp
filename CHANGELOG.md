# Changelog

All notable changes to Athena are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com). Athena has no tagged release yet, so everything is under Unreleased. Phase numbers (P4 to P9) refer to the build roadmap in `docs/revamp/`.

## [Unreleased]

### Added
- Starlark Rules host (P9, ADR-0016, Proposed): validation, derivation, guard and transform Rules run on document events with step, time, read and write limits, no floats, and reads and writes only through Go host functions that apply Grants, row scope and field levels. Includes `RuleService`, Rule versions, simulate with diff, a Rule Run log, `athena rules`, the Rules page, and migration 11. Not production-ready: there is no memory limit.
- Sign-in error states: no users, wrong password, locked, disabled, database down, server down, CSRF. Wrong password and unknown user stay indistinguishable.
- Workspace files service, `athena files` CLI and Files page (P8a, ADR-0015).
- Local sign-in, database sessions, Roles, Grants and the Access Log (P8, ADR-0014, Proposed).
- Restricted CEL expressions and computed fields (P7, ADR-0013).
- Numbering series, audit hash chain and Record pickers (P6, ADR-0012).
- Document lifecycle: submit, cancel and amend (P5, ADR-0011).
- Record kinds, system fields and enforced field attributes (P4, ADR-0010).
- Exact decimal, money, quantity and date types (ADR-0007).
- Versioned system migrations and `athena migrate status|up` (ADR-0006).
- Storage dialect layer with a MariaDB implementation (ADR-0005).
- Per-Workspace databases and optional OpenTelemetry export.
- Research and design documents in `docs/revamp/`.

### Changed
- Vocabulary is now Record Type, Record, Pack, Workspace and home (ADR-0008, Proposed). Old terms remain only in the legacy-name files that migrations need.
- The desk shows "Database not connected" whenever the database is not connected, including when none is configured.
- The desk self-hosts the Inter font. The browser trace forwarder answers 204 and drops spans when the collector is down.
- `make run` gives the server a Files directory (`.athena/files`).
- Centralized frontend theme and type scale.

### Fixed
- Emails without a domain dot are rejected on the server.
- A stopped database no longer shows "Server unreachable" on the sign-in page.
