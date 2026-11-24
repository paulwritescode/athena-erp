# F4 design: Rules and lists without code (2026-11-24; Stylus answers folded)

Status: Stylus answers folded, in build. Implements tasks.md F4. Decided here:
a condition builder over `internal/expr`, and Lists (saved queries) with PDF
export and outbox-queued messages. ADR-0034 records the decisions (Proposed).
No code is written; implementation slices are in §7.

## 1. Research: what the best builders do

Read Airtable, Notion and HubSpot list/filter builders (Nov 2026 docs and
community captures) plus Athena's `internal/expr` (CEL, `Compile` + `Eval` +
SQL `Translate` with in-memory fallback), the Rules page (Starlark source per
rule), and the C2 outbox (pending/leased/dispatched/dead rows, ids-and-counts
payloads, never Record values).

Convergent patterns (all three products):

- One condition is **field → operator → value**. The operator list depends
  on the field type (text: contains/starts-with; numbers: comparisons;
  dates: before/after/between; selects: is-any-of).
- **Empty is a first-class operator** (`is empty` / `is not empty`), never
  typed as a blank value. Airtable treats empty as BLANK(), not `''`.
- **AND is the default connector; OR needs a group.** Notion: rules join
  with And unless flipped to Or; `+ Add filter group` nests. Airtable:
  alternatives go inside an Or group; flat AND/OR mixing is refused as
  ambiguous. HubSpot is stricter: OR only between groups, AND only inside
  (disjunctive normal form) — simple, but users complain they cannot nest.
- **Relative dates** (`in the last/next N days`, `more than 30 days ago`)
  are operators with a number input, not calendar picks.
- **Autocomplete everywhere**: fields by label, values by existing choices
  (select options, list membership in HubSpot).
- **Lists are saved filters with actions**: HubSpot active lists re-evaluate
  live and feed workflows; Airtable views combine filters with sorts.

What fits Athena's Desk idiom: Notion's shape (flat rules + nestable
groups) with HubSpot's plain-words operators, field labels (never keys),
glossary words in View, and Athena's own honesty rule — the builder shows
the expression it compiles ("Show as expression"), never a black box.

## 2. Condition builder

One shared Vue component used by the Rules page, F3 field conditions and
the F2 "Advanced filter". quiz-free: no `doc.` syntax, no CEL shown unless
asked.

### 2.1 Shape

`When [record] [field ▾] [operator ▾] [value or field ▾]`, one row per
condition. `[record]` is fixed by context (the rule's Record Type, the
field's Record Type, the filtered list) and shown as a caption, not a
control, except in Rules where a rule watches one type only. The right side
is a literal input or a same-typed field pick (field-to-field compare).

Groups: a group holds conditions and groups joined by its own AND/OR
toggle (Notion/Airtable nesting, not HubSpot's fixed DNF — `internal/expr`
compiles both, and nesting serves F5's RSVP queries). Empty groups are
refused at save with an inline error naming the group. Slice 1 is
literals-only on the right side; same-record field compare plus one Link
hop lands as the next slice before F5 needs it.

### 2.2 Operators by Athena field type

| Field types | Operators |
| --- | --- |
| Text, Email, Phone, Long text | is, is not, contains, does not contain, starts with, is empty, is not empty |
| Number, Currency, Money, Quantity | is, is not, more than, at least, less than, at most, is empty, is not empty |
| Date | is, is not, before, after, on or before, on or after, in the last N days, in the next N days, is empty, is not empty |
| Checkbox | is checked, is not checked |
| Enum, Select | is, is not, is any of, is none of, is empty, is not empty |
| Link | is, is not (label picker), is empty, is not empty |
| Status (documents only) | is Draft / Submitted / Cancelled (glossary labels) |

No `between`: two conditions in an AND group say it. No per-field regex:
patterns stay in F3 Advanced. `is any of` takes a multi-pick of existing
choices (select options) or typed values (text).

### 2.3 Semantics

- Empty follows the engine both worlds share: comparisons with null are
  false, equality treats null as a value (`internal/expr/sql.go`).
  `is empty` compiles to `field == null`.
- Relative dates compile to `athena.today` with `add_days` (both exist):
  "in the last 30 days" → `d >= add_days(athena.today, -30)`.
- Type errors (text in a number slot) are inline builder errors; the
  expression is never compiled from an invalid tree.
- Everything the builder emits must SQL-`Translate`. The one exception is
  `contains` on Long text: `Translate` accepts `startsWith` only, so
  `contains` falls back to in-memory evaluation and the builder warns
  ("checked after loading", capped by the list page size).

### 2.4 Round-trip and "Show as expression"

The tree is the source of truth, stored as JSON next to the compiled expr
string. Loading parses the expr back into a tree when it matches the
canonical grammar the builder emits; anything else (hand edits, future
operators) opens read-only with "Show as expression" on and an option to
keep editing as a raw expression (advanced) or rebuild. Round-trip tests
pin tree → expr → tree for every operator.

### 2.5 Rules page modes (no Starlark for the common cases)

Each validation/guard/derivation rule gains an authoring mode toggle:

- **Builder**: condition tree + outcome. Validation: message text shown on
  failure. Guard: block with a reason. Derivation: set field(s) to a
  literal or a small formula (expr arithmetic over the record's fields).
- **Starlark** (unchanged): full source for everything else.

Posting/reaction/scheduled Rules stay Starlark-only. Switching Builder →
Starlark renders the condition as a commented scaffold the author extends
(one-way, confirmed); Starlark → Builder starts over blank (confirmed
discard) — Starlark never parses back into a tree. The expr → tree
parse-back serves the builder's editable expression box, not Starlark.

Storage (implementation pins the proto): the Rule carries
`condition_tree` (JSON) and `condition_expr` (compiled) beside `source`;
the server evaluates the compiled expr through the existing lifecycle
registry — no new execution path.

## 3. Lists (saved queries)

A List is a named condition tree over one Record Type with a column choice:
"RSVP yes but missing diet or parking details" is three conditions in AND
plus the Status filter the table already has. Lists live beside Records in
View (lists the user may read) and are managed in Build. Storage is a
kernel table `athena_lists` (migration 23, reserved), not a Record Type:
id, name, record-type key, condition-tree JSON, compiled expr, column
choice, owner, revisions (exact DDL at implementation).

- **Live list**: opening a List runs the compiled expr through the normal
  list path (SQL `Translate` first, in-memory fallback inside the page).
  No snapshots, no staleness.
- **PDF export now**: one click renders the current list (visible columns,
  applied order) to a PDF download. Server-rendered with gopdf v0.38.6
  (MIT; see ADR-0034 for the diligence) plus a CLI
  `athena lists export --pdf`. Every page carries a header with the
  Workspace name and logo, list title, applied filter and render time
  (hand-rolled layout: gopdf has no auto page-break tables); body text
  embeds Inter TTF. Maroto was rejected on binary size (+14.9 MB over a
  ~5 MB budget) and the print-stylesheet fallback is rejected: "export
  now" means bytes. Stylus cleared gopdf v0.38.6 and `go.mod`
  carries it.
- **"Email them" / "Message them"**: queue, not send. One click writes one
  outbox envelope per recipient row (`list.message.requested`, payload of
  IDs + the list ID only — never names, addresses or message text, per the
  envelope rule and the Z4 guest-data decision) through a new `QueueMessages`
  RPC (list ID + channel + audience options). No delivery provider exists
  in beta, so rows stay pending and the button honestly says "Queued";
  the Jobs page shows the rows, and `RequeueOutbox` redrives them when a
  provider lands. The future provider resolves addresses at dispatch time.
- **Audience rule**: the exclusion is a saved condition on the send, not
  domain seeding (F5 owns the sample "invited but not responded" rule).
  Queueing ANDs the list's audience condition into the compiled query and
  the confirm sheet shows counts ("42 guests, 6 excluded as not
  responded"); the envelope records the full query, so redrives keep it.

Permissions: anyone with read on the Record Type opens its lists;
editing lists needs `build`. Queueing needs read on the type plus the new
`lists:send` system permission (declared in `procedurePolicies` and the
Roles UI), so messaging stays grantable without full build. PDF export
follows read.

## 4. F2 / F3 integration

- **F2**: the search box filters from the first character (server text
  match, ~150 ms debounce); the expression box moves behind an "Advanced
  filter" control that embeds this builder. Saved lists appear as one-click
  chips beside it when the Record Type has any.
- **F3**: "Rules for this field" (required when, read only when, show
  when, must be true) and formula inputs embed the builder in compact
  (single-group) form. F3 Advanced keeps key/pattern/levels as selects.

## 5. Desk idiom compliance

Everyday words from `web/src/copy/glossary.ts` in View ("more than", not
`>`; "is empty", not `null`); field labels, never keys; one primary action
per surface (Save list, Queue messages); the same action in the same place
on every builder host; rest/hover/focus-visible/pressed/disabled-with-reason/
loading/error/empty states per component; 375/768/1280/1440 in both themes.
No new accent carriers; Lucide icons with words.

## 6. What is NOT in F4

Reaction/scheduled Rules, posting Rules in the builder, cross-Record-Type
conditions (link traversal stays a picker on the linked label), formula
fields (derivation sets stored fields only), a delivery provider, static
(frozen) lists, list sharing beyond type read Grants.

## 7. Implementation slices (for the build brief)

1. Literals-only core: `athena_lists` (migration 23), condition tree model
   + expr compile/round-trip (Go + shared TS mirror for live validation)
   with round-trip tests per operator.
2. Builder component + "Show as expression", storybooked in the Gallery.
3. Rules page modes (Builder beside Starlark) on validation/guard/
   derivation; Starlark untouched otherwise.
4. Lists: live list + `lists:send` + F2/F3 embeds; field compare + one Link
   hop before F5.
5. PDF export via gopdf (Stylus-cleared) + `athena lists
   export --pdf` + queue RPC + audience rule.

## 8. Answered by Stylus (2026-11-24 order; folded in above)

1. PDF: server-rendered gopdf v0.38.6 (MIT, +1.2 MB) + `athena lists export --pdf`;
   Stylus cleared gopdf; `go.mod` carries it.
   Maroto v2.4.4 evaluated and rejected on size (+14.9 MB).
2. `lists:send` is a new system permission (+ `procedurePolicies` + Roles UI).
3. Slice 1 literals-only; field compare + one Link hop next, before F5.
4. Kernel table `athena_lists`, migration 23 (not a Record Type).
5. Audience exclusion as a saved condition on the send; F5 owns the sample.
