# ADR-0034: No-code conditions and lists

- **Status:** Proposed
- **Date:** 2026-11-24
- **Decision owner:** not assigned (awaiting project owner review)
- **Related decisions:** ADR-0013, ADR-0016, ADR-0028, tasks.md F2, F3, F4

## Context

F4 asks for Rules and lists without code: everyday users must build
conditions ("RSVP yes but missing diet or parking") and act on them (PDF,
Email/Message) without Starlark or CEL. Athena already has the engine
(`internal/expr`: CEL compile, exact math, SQL translate with in-memory
fallback), the execution paths (lifecycle registry, list queries) and the
delivery queue (C2 outbox, no provider in beta). What is missing is the
authoring surface and the saved-query model. Research (Airtable, Notion,
HubSpot, Nov 2026) converges on field → operator → value rows with
type-dependent operators, AND by default with nestable OR groups, empty as
a first-class operator, and relative dates. Full analysis and the builder
spec are in `.agents/tasks/beta-f4/design.md` (draft).

## Options considered

1. **Builder over `internal/expr` with round-trip (chosen).** One shared
   condition-tree component compiles to a canonical expr subset and parses
   it back; "Show as expression" keeps it honest. Reuses the engine, the
   SQL translation and the lifecycle registry; no new execution path.
   Costs: a JSON tree model plus a TS mirror for live validation.
2. **Builder generating Starlark.** Reuses the Rules host but drags every
   simple condition through codegen, step budgets and a second language;
   round-trip is impractical and F2/F3 list filters cannot run Starlark.
3. **HubSpot-style fixed DNF (OR of AND-groups).** Simpler UI, but users
   cannot nest and F5's RSVP queries need nesting; `internal/expr`
   compiles nesting already, so the restriction buys nothing.

## Decision (proposed)

- Ship the §2 builder (field → operator → value rows, per-group AND/OR,
  per-type operator sets, relative dates via `athena.today`, empty per
  engine null semantics) for Rules (validation/guard/derivation beside
  Starlark), F3 field conditions and the F2 Advanced filter. Starlark
  stays for posting, reaction, scheduled and anything advanced.
- Ship Lists as named condition trees over one Record Type, stored in a
  kernel table `athena_lists` (migration 23, reserved): live lists,
  one-click PDF export, and "Email them"/"Message them" queueing one
  `list.message.requested` envelope per recipient through a new
  `QueueMessages` RPC (IDs only, never values; rows stay pending with an
  honest "Queued" label until a provider lands). The audience exclusion
  is a saved condition on the send (confirm-sheet counts, envelope
  records the query); F5 owns the sample rule, no domain seeding here.
- Permissions: lists follow type read Grants; editing lists needs
  `build`; queueing needs read plus the new `lists:send` system
  permission (declared in `procedurePolicies` and the Roles UI).
- PDF export is server-rendered with
  `github.com/johnfercher/maroto/v2 v2.4.4` (MIT, pure Go, no cgo,
  released 2026-10-22; builder rows/cols/tables/images, auto page breaks,
  repeating headers, custom TTF embedding for Inter; transitive deps
  MIT/Apache-2.0/BSD) plus a CLI `athena lists export --pdf`. Every page
  carries Workspace name/logo, list title, applied filter and render
  time. PDF wiring waits for Stylus library clearance before any
  `go.mod` change. Slice 1 is literals-only; field compare plus one Link
  hop follows before F5.

## Consequences

- Validation/guard/derivation Rules gain a Builder authoring mode; the
  Rule carries `condition_tree` + compiled `condition_expr` beside
  `source`, evaluated through the existing registry.
- F2 keeps type-to-filter with the builder behind "Advanced filter"; F3
  embeds the compact builder for field conditions.
- New surfaces: Lists UI, `QueueMessages` RPC, outbox message rows,
  PDF bytes. Envelope privacy rule and Z4 guest-data handling extend to
  message queueing unchanged.
- Out of scope: reaction/scheduled/posting Rules in the builder,
  cross-type conditions, formula fields, a delivery provider, static
  lists, sharing beyond type read Grants.

## Revisit when

Stylus clears or rejects the maroto library (PDF approach), implementation
finds a canonical-subset gap that breaks round-trip, or a delivery
provider arrives (queue → send semantics, address resolution, consent
receipts).
