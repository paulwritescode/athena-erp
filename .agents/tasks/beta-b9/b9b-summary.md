# Summary: B9b name-first New Record Type flow (merged 2468598)

## What shipped

"New Record Type" no longer opens an unnamed editor. It opens a dialog over
the list with Name (required, focused), "What is it for?" (optional) and
Kind (Master default, one-line explanations each). The key derives live
("Saved as customer", editable via Change); Create stays disabled until the
name is valid and unique, with inline errors. Create saves a real Record
Type with one required Name text field and opens its editor on the Fields
tab. The editor title is the h1, renamed in place, via the pencil, or from
the header menu (Rename / divider / destructive Delete). The old
unnamed-draft path is gone (`/build/record-types/new` opens the dialog over
the list). Also fixed four B9a follow-ups in these files: separated Delete,
h1 title, widened field-row action column (fixed the stray glyph), aligned
Required checkbox.

## History

- Worker w-b9b committed 0596b2e (8 web files, no Go).
- Review round 1 PASS: brief checklist fully met, `make test` green (419
  tests), browser pass 29/30 + 1 corrected (script bug, not product) at 1440
  and 375 in dark and light with 18 inspected screenshots, 4 non-blocking
  nits. Reviewer dropped its throwaway DB and stopped its server; the one
  harness restart (hung script) was reviewer-side only.
- Rebased cleanly onto main f9039b5 (zero overlap, incl. the just-merged
  B4b); master reran all checks green; merged --ff-only, pushed, ticked.

## Follow-ups (in tasks.md)

Stale draft comments in `useDeskRoute.ts`; one-way Change key; duplicate
Enter needs blur for the inline error. (The dead `.records-layout` rule
noted in review stays under the B9a follow-up line.)

## Try it

Build mode → Record Types → New Record Type: name it "Vendor", watch the
key derive, Create → editor opens on Fields with the Name field present.
