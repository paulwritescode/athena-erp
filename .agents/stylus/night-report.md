# Stylus night report — night shift, 2026-11-24 (main bafe51d)

## Summary

Shift 3 + night shift have merged 46 code commits plus docs batches: train1 (D3s2 + C2s3 + C1 6.11 + C4s2), train2 (D3s3 + FU batch 1), C4 slice 3 + fix, C5 slice 1 (migration 20 + ADR-0031), train3 (13.3b + D3s4), C4 slice 4, D3 slice 5 (D3 COMPLETE), C2 split slices 4–6 (7 commits via sm-c2), train4 (C1 6.10 + FU batch 2 + FU-H shared harness). C1 6.12 verified commitless. 12 night decisions recorded (Z1–Z7/Z10/Z11/Z13/B7/B10). Six lanes, 2-slot suite (Go serialized slot 0, web/browser slot 1). The user server runs headless on the merged code. All lanes moving; C7 slice 1 + F6 in the merge pipeline.

## Merged this shift

- **Train1** (`f02a4b7` → `919bab4`, 8 commits, tip gate green — Go ok, web 613/613): D3s2, C2s3 + fixes incl. migration 19, C1 6.11 (one review-found false claim fixed), C4s2.
- **C1 6.12** (no commit): checkpoint verified on `919bab4` (browser 22/22, 6/6 shots); r-c1 evidence review PASS.
- **Train2** (`919bab4` → `2d9c17d`, 13 commits, tip gate green — Go ok, web 616/616): D3s3, FU batch 1 (12 Desk commits; B2a → batch 2).
- **C4 slice 3 + fix** (`46bf4fd` → `bc9b1e3`): UoM + determination ranges + 12dp scale fix from review.
- **C5 slice 1** (`b9d08a2` → `f00fc58`): migration 20 party_ledger + ADR-0031.
- **Train3** (`58948de` → `2377cb7`): 13.3b ledger-save recompute + D3 slice 4 (vitals in ADR-0030).
- **C4 slice 4** (`640bd5b` → `094610d`): Property 25 (period ranges, UoM round-trip, conversion bounds).
- **D3 slice 5** (`8f3bd9d` → `9a29fb7`, docs → `4acb8d4`): public-pages docs; D3 COMPLETE.
- **C2 split 4–6** (`4acb8d4` → `87db25e`, 7 commits, docs → `b0510f1`): workflows + Property 14 + consumer wiring, outbox delivery, Event/Job surface + CLI + Desk. Review PASS, 6 follow-ups.
- **Train4** (`b0510f1` → `bafe51d`, 12 commits, tip gate green — 17 Go pkgs ok, web 632/632, zero FAIL): C1 6.10 voucher view (1 commit; browser 25/26 + 15/15 shots), FU batch 2 (9 commits; editor timeouts proven load flake), FU-H shared browser harness (2 commits; all lanes use `.agents/bin/browser` now).

## In progress (by lane, main bafe51d)

- **L1 Books and posting:** C1 COMPLETE; w-c1 on C5 slice 2 (settlement.go implemented, rebased on bafe51d, gating).
- **L2 Jobs, events, integration:** w-c2 on C2 secrets slice 1 — ADR-0033 + docs committed `987261b`; migration 22 file held for 21's merge (contiguity). C10 after secrets.
- **L3 Masters and Packs:** C7 slice 1 REVIEW PASS (migration 21 + ADR-0032, uncached evidence); merge gate re-running on `15ff044` after the train4 rebase. w-c7 on C7 slice 2 (stock engine + 15 tests green, gating, stacked).
- **L4 Desk, first run, access:** w-f1 F1 tables — browser 15/15 green, final suite queued; rebase + PASS next.
- **L5 Quality:** F6 PASS (32/32 shots, full suite green) with r-f6 reviewing; w-fu starts the decisions-docs commit (Z rows + log + README; ADRs worker-owned) on the train4 release.
- **L6 Night asks:** w-f7 on F7 slice 1 (bare-athena dynamic menu + TTY gate, code done, gating). F8 blocked on tap-owner question.

## Blocked and why

- Migration 22 waits for 21 (C7s1 gate running; w-c2 ordered to add + gate on the merge signal). Hours, not stuck.
- F7's menu cannot list backups (no backup command on main) — flagged for the slice verdict, not a blocker.
- F8 waits for the user's tap-owner answer.

## Questions for the user

Z10 licence — Satori is Personal-Use trial only (no licence/binaries in repo): buy a web licence or relent (interim: system stack on public pages, Desk loads Satori via local() only). F8 Homebrew tap owner. Notifications full-list page design. (Second suite slot: moot — slot classes hold.)

## Errors and how they were handled

- C1 6.11 + C4s3 reviews each found one real blocker: one fix each, re-reviewed, merged.
- Go contention timeouts (w-d3/w-c4 20m, sm-c2 probable OOM): slot classes now serialize ALL Go on slot 0; browser/web on slot 1; 30/15-min watchdog.
- SIGTERM on sm-c2's running gate logged a misleading red + orphaned test procs: reaped all; suite now kills the process group and logs `stopped`; stops only via `suite stop`.
- Mac idle-slept 03:41–05:07: caffeinate terminal guards the rest of the night (never close until clock-out).
- Two stale tick lines + future-dated log lines: re-derive every tick from log tail + locks; timestamps from `date`, verified against comms.jsonl.
- C7s1 gate went green on the pre-train4 base mid-merge: re-picked patch-identical onto the new main and re-gated (rule: gate covers the merge tip).

## Next up

C7s1 merge → w-c2 migration 22 → secrets slices → C10; F6 merge → F7 continues → F8 needs tap owner; F1 merge; decisions-docs commit; C5s2 review; C7s2 review.
