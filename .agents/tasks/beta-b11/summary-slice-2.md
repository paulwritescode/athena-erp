# Summary: B11 slice 2 — Desk Setup wizard + readiness view

Merged 2026-11-22 as 7ccf163+4c666b5 (built at be91324+5fbf685,
rebased twice onto newer mains; both rebases byte-identical).

- `web/src/components/setup/*` (new) + `SetupPage.vue` rewrite:
  six-step first-run wizard (company, logo, currency, fiscal year +
  period, invite first owner, readiness) with inline validation,
  loading skeletons, `role=alert` errors, empty→create states, one
  primary action per step, per-step denied states for partial
  grants, and skipped-step rows with Review actions.
- Readiness view merges the wizard-covered checks with the
  server's `GetReadiness` (unknown checks pass through; books
  checks explain the missing permission).
- Fix loop 1: the period step wedged after a failed save (stale
  `years` list re-saved the just-created fiscal year, no in-place
  retry could succeed). Fixed by reusing the saved year on retry
  and adopting on `AlreadyExists`; trap re-walked E2E (attempt 1
  fails on overlap, attempt 2 with clean dates succeeds in place).
- Review: round 1 NEEDS_CHANGES (B1 blocker); round 2 PASS after
  the fix (review-slice-2.md), with a 49-screenshot browser pass
  (fresh six-step walk, back/forward, light theme, 375/768/1440 ×
  dark/light matrix, denied matrix for three custom roles,
  load-error recovery) + e2e-install.sh 28/28. F1–F8
  non-blocking, recorded as follow-ups.
- Merge-time verification on 4c666b5: gofmt empty, go build/vet
  clean, make test exit 0 (quiet run; 63 web files / 613 tests),
  make generate twice with no diff, web typecheck + build clean.
- B11 COMPLETE (slice 1 CLI + guide, slice 2 Desk wizard).
