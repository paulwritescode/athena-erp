# Summary: B5 slice 2, Workspace identity Desk (merged c2674c2; B5 done)

## What shipped

The Workspace name and logo show in the header (sidebar 24 px, mobile top
bar 24 px), sign-in (48 px), tab title (`<name> - Athena`) and favicon, per
theme, never stretched, with the Athena mark as fallback (black on light,
white on dark). "Powered by Athena" appears in the user menu footer (desktop
and mobile) only once a logo is set. Settings → Workspace identity edits the
name and uploads light/dark logos with previews, staged Remove with undo,
dirty-tracked Save, inline type/size validation, and disabled-with-reason
state without `settings:workspace`. B2's brand assets are reused; no root
copies remain. Manifest name fixed to "Athena" with the new tab title.

## History

- Worker w-b5 committed 2e3f5b7 (13 web files).
- Review round 1 NEEDS_CHANGES: a test-only type error broke typecheck, and
  pre-save previews used blob: URLs the page CSP blocks (broken images).
  Fix 5465cfe (rebased c2674c2): typed mock + data: URL previews, CSP
  untouched. Re-review PASS with typecheck green, zero CSP errors, 100+
  scripted DOM assertions across 375/768/1280/1440 × light/dark.
- Rebased cleanly onto main 4dca9c0 (zero overlap); all checks green
  (465 web tests); merged --ff-only, pushed, ticked B5 complete.

## Follow-ups (in tasks.md)

Swatch empty-state contrast; logo error icon pattern; empty-name snapback;
static theme-color (pre-existing); black-mark fallback favicon in dark
chrome.

## Try it

Settings → Workspace identity: set a name + light/dark logos, Save, reload
— header, sign-in, tab and favicon follow; remove them and the Athena mark
returns everywhere.
