# Review brief: F4 slice 1 conditions engine + builder + Rules modes + lists core (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f4-s1-r, detached at 6474b04 (bottom 16 of feat/f4-design: d6cff00 design .. 6474b04 builder-exec; slices 2–3 stack ABOVE and are out of scope).
Read: .agents/tasks/beta-f4/design.md + docs/decisions/adr-0034-no-code-conditions-and-lists.md (Proposed) + night orders F4 + the 5 Stylus design answers (in design.md).

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Engine: expr condition tree compile + parse round-trip (all ops incl. nesting/escapes/normalization) + date() — verify round-trip tests are real (run them); TS mirror matches (25/25).
2. Builder: condition builder component + Gallery story + Rules page Builder mode beside Starlark (mode toggle, message/sets editors, switch confirmations) — verify one builder-built rule executes end to end without Starlark; Starlark path untouched (regressions green).
3. Lists core: migration 23 (applies cleanly on 1..23 — verify), ListService CRUD/run/export + gopdf renderer (Inter embed, wrapped cells, repeated header; golden 3-page test deterministic — run it twice); CLI export --pdf works.
4. PDF diligence: gopdf v0.38.6 MIT closure recorded in ADR-0034 (no GPL/cgo, size within budget) — verify go.mod + the recorded list.
5. Proto: ListService + Rule builder fields additive only (no breaking changes — verify generate stable); lists:send NOT in slice 1 (it is slice 3 — fail if present).
6. Gates: full suite green (reuse-or-rerun through the main-repo suite on 6474b04 — big; affected minimum is expr/lists/server/web); browser shots eyeballed (4 shots in /tmp/f4-shots); typecheck/build clean.
7. Scope: bottom-16 ONLY (no RecordsPage/F2/F3 embeds — slice 2; no QueueMessages/Desk lists UI — slice 3; fail if present).

Report: say master "F4 SLICE 1 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f4/review-slice-1.md with the verdict line.
