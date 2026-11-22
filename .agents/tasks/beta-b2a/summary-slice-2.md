# Summary: B2a slice 2, Access + Rules + Files on DataTable (merged 96edd68)

## What shipped

Access users, roles and sessions, the Rules list (Type/Status filters) and
Rule Runs table (Outcome/Event filters), and the Files list (Folder filter)
plus the mapping table — all on the shared DataTable with §21 frame,
toolbar, selection, footer and states. Row menus reuse each sheet's actions
with explicit targets (no sheet+dialog same-tick stacking). Versions and
history tables, the permissions matrix and the simulate table intentionally
stay plain Tables. All six slice-1 follow-ups fixed along the way (search
padding + 480 px cap, dead CSS removed, glossary status words, single list
request on filter clear, pressTimer cleanup).

## History

- Worker w-b2a committed ed600ca (9 files), then fix 746af86 in loop 1.
- Review round 1 NEEDS_CHANGES: three stale frozen row-menu targets — a
  broken assignment save plus two wrong-object deletions (data loss), all
  reproduced with CLI proof. Fix sets/clears targets on the sheet/editor
  paths and captures before closing. Re-review PASS with all three
  reproductions re-run green plus regression.
- Rebased cleanly onto main f6f2fc0; all checks green; merged --ff-only,
  pushed, recorded slice line (slice 3 open).

## Follow-ups (in tasks.md)

No bulk RPCs (select correctly hidden); runs 50-cap with ignored next
token and no total; centered columns in stacked rows; no-file mapping rows
focusable; empty Actions column; unexplained padding rewrite.

## Try it

Access → users/roles/sessions; Rules → list + runs; Files → list + mapping:
search, filter chips, paging, selection, row menus, all at phone and desktop
widths.
