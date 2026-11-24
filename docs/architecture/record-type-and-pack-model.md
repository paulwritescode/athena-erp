# Record Type and Pack model

**Last updated:** 2026-11-24  
**Status:** Product direction accepted; authoring synchronization and upgrade behavior remain open. Vocabulary follows ADR-0008 (Proposed): Record Type, Record Type, Pack, Workspace.

## Core concepts

Athena is the core platform. It should not ship as an invoicing or ticketing system. A future implementation can install those as Packs made from Record Types and rules.

| Concept | Meaning | Persistent representation |
| --- | --- | --- |
| Record Type | A named schema with fields and behavior hooks | Metadata row plus a MariaDB table |
| Record | One business object created from a Record Type | One row in the Record Type's MariaDB table |
| Relationship | A typed reference between Records | Foreign key or join table, according to cardinality |
| Pack | An installable, versioned bundle of Record Types, relationships, rules, actions, and UI | Pack metadata plus its included definitions and install/upgrade manifest |
| Module | A functional area inside a Pack (kernel specification section 22.4); not installable by itself | Not implemented |
| Workspace | A database plus its installed Packs, Records, users and overlays | One MariaDB database and its `athena.yaml` |
| Go behavior | Trusted compiled hooks and platform invariants | Go source in the repository and the compiled Athena executable |
| Starlark rule | Admin-configurable condition and bounded action | Versioned rule source associated with Record Type or Pack metadata |

## Intended authoring and build flow

```text
Dashboard Record Type editor ──┐
                          ├─> validated Record Type
Codebase Record Type package ──┘          ├─> MariaDB metadata and table migration
                                     ├─> generated Go type and hook scaffold
                                     └─> View renderer and rule editor schema
```

The same logical definition must be usable from the dashboard and the codebase. Before both paths are implemented, decide how to track provenance and ownership, how conflicts are detected, whether edits to generated Go are preserved, and how upgrades migrate a deployed customer's table.

### Frappe comparison and Go source boundary

Frappe supports both code-backed and database-backed DocTypes. In the developer workflow, a non-custom DocType is exported as app files, including a JSON definition, a Python controller, and a JavaScript client script. Custom DocTypes and site-level changes can instead be stored as metadata records. Client Scripts and (when enabled) Server Scripts provide a separate runtime customization path. The controller code is Python source loaded by Frappe; creating or editing a DocType is not the same action as compiling a new application binary.

Athena's corresponding split is: Record Type metadata and its MariaDB schema are data; Go packages own trusted lifecycle behavior, invariants, and controls; Starlark is the planned runtime path for bounded configurable conditions and actions. Dashboard authoring is intended to create a Go source scaffold in the developer/codebase workflow, but that file cannot alter the already-running executable. Saving a Record Type updates metadata and schema; generated Go behavior becomes active only after a new build and deployment. A hosted production process may not have a writable source tree, so the code-generation destination and dashboard-to-codebase synchronization must be designed explicitly.

### Go source generation boundary

Dashboard-created Record Types are intended to generate Go source scaffolds in a dedicated package through the developer/codebase workflow. The files should contain deterministic type names, typed field declarations, validation/lifecycle interfaces, and clearly marked extension hooks. Go must not interpret user-provided text as source code; names and types are validated and all output is generated from a constrained schema.

Generated Go source cannot become active inside an already-running Go binary. The source workflow must make its stages explicit: save the definition and DDL, generate and review source in the codebase, then rebuild and restart/deploy the executable to activate Go hooks. Saving a Record Type alone only persists metadata and database schema. Starlark is the editable, runtime-configurable path for bounded conditions and actions that do not require recompilation. The source generator, build/reload workflow, overwrite policy, and file ownership are not implemented yet.

Proposed answer (ADR-0015, not accepted): generated Go goes to `generated/<key>_gen.go` in the Workspace, owned by the publisher and marked as generated; hand-written hooks stay outside `generated/`; the publisher records the hash of what it wrote and reports a conflict instead of overwriting an edited file. The Files page and `athena files` already read and edit these files with a content-hash check and a version history, but the generator and the publisher's hash record are not built.

### MariaDB mapping

- Each Record Type gets a stable lowercase key and a deterministic table name `p_<pack>__<key>`. A Record Type authored in the Workspace uses the reserved pack name `workspace`, so `customer_address` is stored in `p_workspace__customer_address`. Generated names are at most 63 bytes, with a deterministic hash suffix beyond that (ADR-0009, Proposed).
- Core columns include a stable primary key and created/updated timestamps.
- Record Type field names become validated SQL column names. Display labels never become SQL identifiers.
- Field type mapping is owned by Go: text/email/phone to bounded strings, long text to `TEXT`, exact numeric/currency to `DECIMAL`, dates to `DATE`, flags to `BOOLEAN`, and links to a stable key type.
- A Link field names its target Record Type and is backed by a foreign key with a deliberate delete policy. `RESTRICT` is the current default for an initial many-to-one Link field.
- For many-to-many relationships, a Pack relationship will need a generated join table with foreign keys, uniqueness, timestamps, and explicit delete behavior.
- Schema updates must be versioned. The initial implementation only provisions a table and adds missing columns; it does not safely handle renames, drops, type changes, or backfills.
- Migration and Record Type metadata changes need an operator-visible plan and transaction/rollback story before production use. MariaDB DDL may commit independently of metadata writes, so error recovery must account for partially provisioned tables.

### Tree and effective-dated attributes (ADR-0029, Proposed; implemented 2026-11-24)

- A Record Type flagged `tree` (master and document kinds only; child and setting are refused) stores `tree_parent_id`, `tree_path` and `tree_depth` on its rows. The kernel refuses cycles and cross-company links. Enabling the flag backfills existing rows as top-level; disabling it is destructive-gated with counts, through the same plan-and-confirm flow as any schema change.
- A Record Type flagged `effective_dated` (any kind, but never with amendable: amending copies values without splitting their range) stores an end-exclusive `[effective_from, effective_to)` range per row. Overlapping ranges for one entity are refused; lists resolve "as of" end-exclusive.
- The desk exposes both flags as Record Type settings switches in plain words ("Arranged as a tree", "Valid between two dates"). There is no Desk as-of view yet (deferred follow-up).
- The same slice family adds UoM families with exact factors and item overrides (`athena_uom_families`, `athena_uom_conversions`), end-exclusive ranges on account determinations resolved by specificity, and materialised path/depth on the four kernel tree masters (accounts, branches, departments, cost centres), recomputed inside the save transaction. Nothing is seeded. The CLI surface is `athena ledger tree|uom|determination`; Property 25 pins the range and round-trip behaviour.

### Pack composition

A Pack is not merely a folder or a list of Record Types. Its definition should explicitly include:

- Record Types and compatible versions;
- relationships and their cardinality, owning side, foreign-key/join-table mapping, and deletion policy;
- rules, Go hook packages, and their execution lifecycle;
- user-facing views, navigation, permissions, and reports;
- install dependencies, schema migrations, upgrade compatibility, and uninstall behavior.

For example, a future `Invoicing` Pack could contain generic platform Record Types such as invoice header, invoice line, and payment. Those are examples of a Pack package, not Record Types seeded into the Athena core.

## Rule references and safety

The Starlark editor should use Record Type and field identifiers from the current schema instead of free-form strings where possible. A rule publish step validates every referenced field and relationship, pins the definition version, and declares allowed events/actions. At runtime, Go provides only the relevant authorized Record values and narrow capabilities. Rules cannot issue SQL, change schema, cross tenant boundaries, or skip Go invariants.

## Current implementation boundary

The current repository can store Record Types in MariaDB and provision a per-Record-Type table when a definition is saved. Link fields can add a foreign key when the target table exists. The desk builds Record Types and, in View, lists, creates, edits and deletes Records. There is no Pack composition API, Go source generator, Record Type migration ledger, or multi-tenant authorization. Sign-in, sessions and Grants exist (ADR-0014, Proposed). Platform tables use versioned system migrations (ADR-0006).

## Open implementation decisions

1. Authoring precedence when a dashboard edit and a codebase definition use the same Record Type key.
2. How generated Go files are distinguished from hand-maintained hooks and preserved during regeneration.
3. Whether dashboard changes may provision production schema directly or must first pass a reviewed migration plan.
4. Stable keys, tenant scoping, record deletion/archive policy, and audit columns.
5. Relationship cardinalities and supported indexes in the first Pack editor.
6. Record Type key and table-name normalization, identifier limits, and versioned schema compatibility.
7. Pack package versioning, dependency resolution, installation, upgrades, and removal.

## References

- [Frappe Controllers](https://docs.frappe.io/framework/user/en/basics/doctypes/controllers)
- [Frappe Create a DocType](https://docs.frappe.io/framework/user/en/tutorial/create-a-doctype)
- [Frappe Customizing DocTypes](https://docs.frappe.io/framework/user/en/basics/doctypes/customize)
- [Frappe Client Script](https://docs.frappe.io/framework/user/en/desk/scripting/client-script)

- [shadcn-vue Vite installation](https://www.shadcn-vue.com/docs/installation/vite)
- [shadcn-vue CLI](https://www.shadcn-vue.com/docs/cli)
- [Preset customization and semantic tokens](https://github.com/unovue/shadcn-vue/blob/dev/skills/shadcn-vue/customization.md)
