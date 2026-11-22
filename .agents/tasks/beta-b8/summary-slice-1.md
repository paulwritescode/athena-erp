# Summary: B8 slice 1, root administrator server + CLI (merged 4d03467)

## What shipped

ADR-0018 confirmed end to end: root seeded after migrations at both seams
(kernel open + serve boot) with the database password (generate-and-print
fallback, idempotent, skip-with-log on name clash); the name reserved at
every creation path through one case-insensitive check; gate bypass proven
over HTTP with zero assignments; display-name edits and role/assignment
changes refused on root in UpdateUser/SaveAssignment alongside the existing
suspension guard (API, CLI and Desk share the refusal, plain language);
no DeleteUser path exists by construction; every root action audit-logged
as root; password rotation moves only the root credential. 16 lines of
guards + 4 test files (auth unit, HTTP gate/audit, CLI incl. first-boot
seeding, Desk row/sheet refusals).

## History

- Worker w-b8 committed 6c702a3 (message reworded to Conventional style at
  merge, content identical).
- Review round 1 PASS: guards + per-path tests verified (9 auth, 3 HTTP, 1
  CLI, 4 desk, none skipped), full suite green (513 web), generate stable,
  3 follow-ups.
- Rebased cleanly onto main 9fdad29; all checks green; merged --ff-only,
  pushed, recorded slice line (slice 2 open).

## Follow-ups (in tasks.md)

Untested taken-name skip branch; duplicated seeding seam coverage; Desk
isRootSelected case hardening.

## Try it

As root: invite/edit/assign/suspend paths refuse root edits with plain
errors; `athena access` likewise; audit log attributes root rows to
`administrator`.
