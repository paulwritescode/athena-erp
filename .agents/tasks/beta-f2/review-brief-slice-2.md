# Review brief: F2 slice 2 Desk search (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f2-s2-r, detached at 1087724 (slice 2 on merged slice-1 base; review ONLY the slice-2 delta — slice 1 is merged). Verify clean + `git log main..HEAD` shows slice-2 commits only.
Read: .agents/tasks/beta-f2/brief.md slice 2; the granted-scope ruling (RecordsPage search-only) + Find-verb ruling (night log).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Search: factored DataTableSearch (150ms debounced emit, immediate clear, unmount-safe — verify timer cleanup); dataTableSearch helpers; searchScope prop (scope label, placeholder override wins); Find-placeholder derivation correct per §21 (VARIANCE vs task text Search is APPROVED — do not flag).
2. RecordsPage wiring stays inside the GRANT (search->query, debounce removed there, derived placeholder, status-only expression override for F4) — fail on ANY other RecordsPage logic change.
3. Glossary: find/clearSearch words used; no hardcoded Search/Clear in the search block.
4. First-char proof: test genuinely types first char (not programmatic set); browser E2E (seed 5, z->2, apples->1, ac->2, clear->5 twice, placeholder asserted, 4 shots 1280+375 x light/dark, 0 errors) — eyeball the shots + verify the run log.
5. Gates: affected web green (70 files/660 tests claimed — verify); vue-tsc clean; no proto change (verify); gofmt/build clean.
6. Scope: NO Advanced filter / F4 builder code (deferred — fail if present); CLI search flag correctly absent.

Report: say master "F2 SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f2/review-slice-2.md with the verdict line.
