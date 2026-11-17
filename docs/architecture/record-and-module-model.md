# Record and Module model

**Last updated:** 2026-11-17  
**Status:** Product direction accepted; authoring synchronization and upgrade behavior remain open.

## Core concepts

Athena is the core platform. It should not ship as an invoicing or ticketing system. A future implementation can install those as Modules made from Records and rules.

| Concept | Meaning | Persistent representation |
| --- | --- | --- |
| Record definition | A named schema with fields and behavior hooks | Metadata row plus a MariaDB table |
| Record entry | One business object created from a Record definition | One row in the Record's MariaDB table |
| Relationship | A typed reference between Record entries | Foreign key or join table, according to cardinality |
| Module | A reusable capability grouping Records, relationships, rules, actions, and UI | Module metadata plus its included definitions and install/upgrade manifest |
| Go behavior | Trusted compiled hooks and platform invariants | Go source in the repository and the compiled Athena executable |
| Starlark rule | Admin-configurable condition and bounded action | Versioned rule source associated with Record or Module metadata |

## Intended authoring and build flow

```text
Dashboard Record editor ──┐
                          ├─> validated Record definition
Codebase Record package ──┘          ├─> MariaDB metadata and table migration
                                     ├─> generated Go type and hook scaffold
                                     └─> View renderer and rule editor schema
```

The same logical definition must be usable from the dashboard and the codebase. Before both paths are implemented, decide how to track provenance and ownership, how conflicts are detected, whether edits to generated Go are preserved, and how upgrades migrate a deployed customer's table.

### Frappe comparison and Go source boundary

Frappe supports both code-backed and database-backed DocTypes. In the developer workflow, a non-custom DocType is exported as app files, including a JSON definition, a Python controller, and a JavaScript client script. Custom DocTypes and site-level changes can instead be stored as metadata records. Client Scripts and (when enabled) Server Scripts provide a separate runtime customization path. The controller code is Python source loaded by Frappe; creating or editing a DocType is not the same action as compiling a new application binary.

Athena's corresponding split is: Record metadata and its MariaDB schema are data; Go packages own trusted lifecycle behavior, invariants, and controls; Starlark is the planned runtime path for bounded configurable conditions and actions. Dashboard authoring is intended to create a Go source scaffold in the developer/codebase workflow, but that file cannot alter the already-running executable. Saving a Record updates metadata and schema; generated Go behavior becomes active only after a new build and deployment. A hosted production process may not have a writable source tree, so the code-generation destination and dashboard-to-codebase synchronization must be designed explicitly.

### Go source generation boundary

Dashboard-created Records are intended to generate Go source scaffolds in a dedicated package through the developer/codebase workflow. The files should contain deterministic type names, typed field declarations, validation/lifecycle interfaces, and clearly marked extension hooks. Go must not interpret user-provided text as source code; names and types are validated and all output is generated from a constrained schema.

Generated Go source cannot become active inside an already-running Go binary. The source workflow must make its stages explicit: save the definition and DDL, generate and review source in the codebase, then rebuild and restart/deploy the executable to activate Go hooks. Saving a Record alone only persists metadata and database schema. Starlark is the editable, runtime-configurable path for bounded conditions and actions that do not require recompilation. The source generator, build/reload workflow, overwrite policy, and file ownership are not implemented yet.

### MariaDB mapping

- Each Record gets a stable, sanitized name and deterministic table name, for example `CustomerAddress` → `athena_record_customeraddress` (the exact naming rule can be improved before the migration format is frozen).
- Core columns include a stable primary key and created/updated timestamps.
- Record field names become validated SQL column names. Display labels never become SQL identifiers.
- Field type mapping is owned by Go: text/email/phone to bounded strings, long text to `TEXT`, exact numeric/currency to `DECIMAL`, dates to `DATE`, flags to `BOOLEAN`, and links to a stable key type.
- A Link field names its target Record and is backed by a foreign key with a deliberate delete policy. `RESTRICT` is the current default for an initial many-to-one Link field.
- For many-to-many relationships, a Module relationship will need a generated join table with foreign keys, uniqueness, timestamps, and explicit delete behavior.
- Schema updates must be versioned. The initial implementation only provisions a table and adds missing columns; it does not safely handle renames, drops, type changes, or backfills.
- Migration and Record metadata changes need an operator-visible plan and transaction/rollback story before production use. MariaDB DDL may commit independently of metadata writes, so error recovery must account for partially provisioned tables.

### Module composition

A Module is not merely a folder or a list of Records. Its definition should explicitly include:

- Record definitions and compatible versions;
- relationships and their cardinality, owning side, foreign-key/join-table mapping, and deletion policy;
- rules, Go hook packages, and their execution lifecycle;
- user-facing views, navigation, permissions, and reports;
- install dependencies, schema migrations, upgrade compatibility, and uninstall behavior.

For example, a future `Invoicing` Module could contain generic platform Records such as invoice header, invoice line, and payment. Those are examples of a Module package, not Records seeded into the Athena core.

## Rule references and safety

The Starlark editor should use Record and field identifiers from the current schema instead of free-form strings where possible. A rule publish step validates every referenced field and relationship, pins the definition version, and declares allowed events/actions. At runtime, Go provides only the relevant authorized Record values and narrow capabilities. Rules cannot issue SQL, change schema, cross tenant boundaries, or skip Go invariants.

## Current implementation boundary

The current repository can store Record definitions in MariaDB and provision a per-Record table when a definition is saved. Link fields can add a foreign key when the target table exists. The UI is a starter builder and View renderer; it does not yet create Record-entry rows. There is no Module composition API, Go source generator, Starlark runtime, schema migration ledger, authentication, or multi-tenant authorization.

## Open implementation decisions

1. Authoring precedence when a dashboard edit and a codebase definition use the same Record key.
2. How generated Go files are distinguished from hand-maintained hooks and preserved during regeneration.
3. Whether dashboard changes may provision production schema directly or must first pass a reviewed migration plan.
4. Stable keys, tenant scoping, record deletion/archive policy, and audit columns.
5. Relationship cardinalities and supported indexes in the first Module editor.
6. Record-name/table-name normalization, identifier limits, and versioned schema compatibility.
7. Module package versioning, dependency resolution, installation, upgrades, and removal.

## References

- [Frappe Controllers](https://docs.frappe.io/framework/user/en/basics/doctypes/controllers)
- [Frappe Create a DocType](https://docs.frappe.io/framework/user/en/tutorial/create-a-doctype)
- [Frappe Customizing DocTypes](https://docs.frappe.io/framework/user/en/basics/doctypes/customize)
- [Frappe Client Script](https://docs.frappe.io/framework/user/en/desk/scripting/client-script)

- [shadcn-vue Vite installation](https://www.shadcn-vue.com/docs/installation/vite)
- [shadcn-vue CLI](https://www.shadcn-vue.com/docs/cli)
- [Preset customization and semantic tokens](https://github.com/unovue/shadcn-vue/blob/dev/skills/shadcn-vue/customization.md)
