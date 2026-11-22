# Summary: B10 slice 1, art registry + core placements (merged 82cf024)

## What shipped

`web/src/art/registry.ts` per §9: placement key → figure/ASCII, size,
label; `useArt` returns nothing for unlisted keys; figures lazy-loaded.
Placements: sign-in padlock (240/160 px + 5-line ASCII footer), 404 query
figure, offline plug figure — one per screen, quiet tones, aria-hidden,
reduced-motion rested — plus the ASCII mark in the CLI banner (byte-
identical Go/TS drawings) and sign-in footer. Stock hairline figures only
(padlock/plug/query); every placement rule enforced by tests (one figure
per .vue, registry-only imports, unknown-key rejection, motion).

## History

- Worker w-b10 committed f1d620d (19 files). The hairline-create skill is
  not installed and cannot be fetched (no-network policy), so the custom
  Athena set was correctly deferred — stock coverage is complete.
- Review round 1 PASS: §9 checklist verified, typecheck/test (509)/
  build green, Go banner test green, browser matrix 375/1440 × dark/light
  with measured sizes and reduced-motion frame equality, 4 follow-ups.
- Rebased cleanly onto main 68a32d3; all checks green; merged --ff-only,
  pushed, recorded slice line (slice 2 open).

## Follow-ups (in tasks.md)

Stale 404 address on Back-to-home; 120 KB all-figure chunk (tree-shaking);
FigureName closed union; custom figure set (needs the skill or a user-
provided set — also a morning question).

## Try it

Sign-in page (padlock + ASCII footer), a bad URL (404 figure, Back to
home), offline emulation (plug figure), `athena --help` (ASCII banner).
