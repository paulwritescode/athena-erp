# B7 leftovers analysis (night master, 2026-11-22 ~00:35)

Checked tasks.md B7 against main (9260d04). Verdict: B7 is essentially
complete; it needs a verification pass, not a build. Dispatch a small
verify-and-fix worker (or fold into a reviewer) after the B9b/B4b/B2a/B5
merges land, since fixes could touch shared files (App.vue, api.ts).

## Covered in main

- Server (8739e3d, migration 16, ADR-0026 Proposed): idle 30 min (15 min
  build/root), absolute 12 h, remember-device 7 d for non-builders, all as
  Workspace settings; AuthService.StepUp with 15-min proof for builders/root
  before destructive procedures; session ID rotation at sign-in, permission
  change and step-up; sessions end on access change; Clear-Site-Data on
  logout and password change; rate limits before the gate (sign-in per IP and
  username incl. Login/CompleteChallenge/StartReset, writes per session,
  expensive reads incl. Export*/ReportService/RunReport) with Retry-After and
  Access Log rows without values; `athena security show|set`;
  `athena user sessions` with device and expiry; Bruno 09-idle-expiry and
  10-sign-in-limit.
- Desk (ec98c5f, 76210b3, B3a): IdleWarningDialog + idleWarning.ts, cross-tab
  sessionChannel.ts + test, StepUpDialog + stepUp.ts + test, SecuritySection
  settings, Access sessions list with device + last seen + detail sheet +
  end-session confirm.

## To verify (worker checklist)

1. Idle warning appears 2 min before expiry with "Stay signed in", and
   activity in any tab resets it (BroadcastChannel activity path).
2. Sign-in page offers "Keep me signed in on this device" only for
   non-builder sessions; remembered sessions get the 7-day absolute.
3. Workspace switch clears caches in all tabs.
4. Cross-tab sign-out Desk test exists and passes (sessionChannel.test.ts).
5. Expensive-read limits are exercised against a real expensive procedure
   (ReportService/RunReport are stubs until C6; if untestable now, record as
   a C6 follow-up, not a B7 blocker).
6. Bruno 429/expiry checks pass against a throwaway Workspace.

If all six hold, tick B7. Fixes, if any, are expected to be small and Desk-side.
