# Decision log

This is the canonical status of current product and architecture decisions. Update it when a decision changes; update [`AGENTS.md`](../../AGENTS.md) in the same change so agent guidance does not go stale.

## Accepted direction

| ID | Decision | Status | Notes |
| --- | --- | --- | --- |
| ADR-0001 | One Go server serves the embedded frontend and API | Accepted | The frontend is built by Vite and embedded in the Go executable; one run command starts the UI and API together. See [ADR-0001](adr-0001-single-server.md). |
| ADR-0002 | Use MariaDB first for platform metadata | Accepted | Configure with `ATHENA_DATABASE_URL`; the first implementation persists Form definitions and initializes metadata tables. See [ADR-0002](adr-0002-mariadb-first.md). |
| D-002 | Vue 3 + TypeScript + Vite is the single frontend stack | Accepted | No additional frontend frameworks are planned for the base product. |
| D-003 | ConnectRPC is the Go RPC library, with Protobuf as the contract | Accepted | Generate Go server code and TypeScript descriptors/clients from `.proto` definitions. |
| D-004 | Go owns trusted core business logic; Starlark provides configurable rules | Accepted direction | The exact Starlark API, execution limits, and rule lifecycle need design before implementation. |
| D-005 | Use “Form” for a business-record definition and “Module” for an installable capability bundle | Accepted vocabulary | A module groups Forms with related workflows, rules, reports, permissions, and navigation. The UI editor can be called a Form view. |
| D-006 | Keep cloud-provider services optional to the core ERP | Accepted direction | Providers and first supported deployment profiles have not been selected. |

## Open decisions

| ID | Question | Current options / context |
| --- | --- | --- |
| O-001 | Should PostgreSQL be added as a second supported relational database? | MariaDB is the first metadata database; multi-dialect support adds query, migration, and operations costs. |
| O-002 | What first business workflow should prove the model? | Candidate: Customer → Sales Invoice → Payment → Ledger Entry. No workflow selected. |
| O-003 | Where do Form definitions live and how are they edited? | Go-defined core, UI-authored metadata, Starlark, or a carefully defined combination. Schema upgrades and validation are unresolved. |
| O-004 | What exact capabilities may Starlark rules use? | Define available events, inputs, actions, limits, failure handling, permissions, and versioning. |
| O-005 | How are modules packaged, installed, upgraded, and removed? | Reusable across projects is the product goal; dependency/version constraints and upgrade migrations are open. |
| O-006 | What multi-tenancy model is required? | Not selected. Do not assume one database, schema, or process per tenant. |
| O-007 | What concrete Frappe gaps should Athena fix first? | Gather user/workflow evidence before making comparative product claims. |
| O-008 | Which authentication, file storage, queue, and cloud adapters are needed first? | All provider examples remain exploratory. |
| O-009 | What should the installation wizard configure? | Profiles and supported combinations are not yet defined. |
| O-010 | Is Athena the final product name? | It is the working name in this repository; no naming clearance or final decision has been made. |
| O-011 | Should metadata CRUD use ConnectRPC or a typed JSON/HTTP interface? | Initial Form metadata read/write is JSON/HTTP on the same origin; avoid growing this surface until the API contract is deliberately selected. |

## Rejected or unverified assumptions

- PostgreSQL and MariaDB are not yet both supported just because both were discussed.
- The Go executable does not replace a hosted relational database for a multi-user deployment.
- ConnectRPC protocol compatibility does not automatically create a conventional REST API.
- Storage, identity, queues, and databases are not interchangeable abstractions merely because adapters can be written.
- Claims about provider pricing, operational cost, performance, or superiority to Frappe/SAP remain unverified unless documented with current evidence.
