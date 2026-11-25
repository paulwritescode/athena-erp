# Stylus night report — night shift, 2026-11-24→25 (main 0897792)

## Summary

Shift 3 + night shift have merged about 100 code commits plus docs batches: the full C2 (events/jobs + secrets) and C4 (ledger masters) tasks are COMPLETE and ticked, as are D3, F1, F2, F6; F4 slice 1 (builder conditions engine + lists service/PDF/CLI) and four test-speed slices (full gate 836s, posting 853s → 19s) are in. Six lanes plus L7 test-speed, 2-slot suite (Go serialized slot 0 with DB-env guard, web/browser slot 1; suite default -p 2). The user server runs headless on the merged code. All lanes moving; 6 workers + 4 reviewers live.

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
- **Train4** (`b0510f1` → `bafe51d`, 12 commits, tip gate green — 17 Go pkgs ok, web 632/632, zero FAIL): C1 6.10 voucher view, FU batch 2, FU-H shared browser harness.
- **C7 slice 1** (`a6dec5c` → `6c99d3a`, 1 commit): migration 21 (10 stock tables) + ADR-0032. Review PASS (cache-provenance crux cleared). Merge gate: real 1042s run (17 Go ok, 632 web) after two hollow gates were voided (no DB env in the fresh staging tree).
- **Decisions docs** (`6c99d3a` → `72caec6`, 1 commit): section Z (Z1–Z7/Z10/Z11/Z13 DECIDED), 12 log rows, ADR-0023 amendment, LICENSE/NOTICE/README/AGENTS.md. Review PASS (transcription exact).
- **F1 tables** (`16cea19` → `6ecf06d`, 3 commits): footer count, right actions, explicit padding/left cells/select symmetry; browser 15/15 + 8/8 shots; DISJOINT from 6.10. Review PASS. Merge gate 424s (server re-ran fresh 411s DB-backed, 636 web).
- **F6 logo** (`2954bce` → `bcd2913`, 4 commits): generator + frames, draw-in on 4 surfaces, CLI banner; 32/32 shots, 23/23 art tests. Review PASS after one NEEDS_CHANGES (step-ms, fixed). Merge gate 507s (server fresh 489s, 646 web).
- **C5 slice 2** (`6c871c9` → `929d3dd`, 1 commit): settlement open items + applications (locks, ceiling, unapplied cash). Review PASS. Merge gate 1124s (17 Go ok, 646 web).
- **F2 slice 1** (`3e1bc9d` → `57e04c4`, 1 commit): server free-text match on ListRecords (hidden-safe, paged, escaped). Review PASS (disjoint-rule carry). Merge gate 1151s (17 Go ok, 646 web).
- **C2 secrets slice 1** (`baf6a8b` → `65e59cc`, 2 commits): migration 22 (secret refs, key versions, disposal, holds) + ADR-0033 (Z4 exact). Review PASS. Merge gate 1150s (17 Go ok, 646 web). Unblocks F4's DB gates. Also: B7 ticked (verified + descope), C4 slice 5 briefed (L6 after F7s2), F3 briefed (L4 after F2s2).
- **C7 train s2+fix+s3** (`96acb15` → `3272765`, 3 commits): stock core + 4-finding fix + cost deltas. Reviews PASS (re-review for the fix). Gate 703s (18 Go ok, 646 web) after a contention red (rogue run killed, DB fix halved times).
- **F3 slice 1** (`8b0efdf` → `a82d934`, 1 commit): field sheet + sections + Basics move + selects + glossary (3 variances disclosed). Browser caught 1 real state bug (fixed). Review PASS. Merge gate 1223s (17 Go ok, 670 web; auth/posting/server finished together — the Argon2-starvation evidence).
- **C2 secrets slice 2** (`276130a` → `4616729`, 1 commit): internal/secrets core (refs, file provider, versions, bound envelopes, redaction). Review PASS (4 advisories). Gate 32s (cached Go + fresh web 646) after a registry-flake red. Disjoint-additive merge per Stylus ruling.
- **C7 slice 4** (`da19c31` → `2630dd3`, 1 commit): Property 21 append-only cost correction (rapid, byte-identical history). Review PASS. Merge gate 843s (18 Go ok, 646 web).
- **C2 secrets slice 3 + F2 slice 2** (main `3cc6022`): Property 16 keys (100 iterations, mutation-checked) + Desk type-to-filter on Records (Find verb, 150ms debounce, E2E). Both reviews PASS. F2 COMPLETE and ticked.
- **C5 slice 3** (main `5795830`): Property 11 allocation sums (both levels). Review PASS.
- **TS slice 1** (main `6c871c9`): cheap Argon2 in auth TestMain + production pins; auth 1178s → 188s alone. Solo merge per Stylus order.
- **C2 secrets slice 4 + F7 slice 1** (main `89cf82d`): disposal jobs (exact Z4; guest gap pending markers) + bare-athena registry menu. Reviews PASS. -p 4 stopgap starts.
- **F7 slice 2** (main `eb04345`): athena doctor (5 checks, exit/JSON contract, no leak). Review PASS. Suite default -p 2.
- **C5 slice 4** (main `43ded7c`): realised FX as plans + current-period reversals (no sentinels). Review PASS.
- **C2 secrets slice 5 + C4 slice 5 + C5 slice 5** (main `9b1c070`): secrets CLI; tree/uom CLI + determination as-of + Desk flags; Property 12 party-equals-control. Reviews PASS. C2 + C4 COMPLETE and ticked. C5s5's rapid .fail ruled debris (removed in C5s6).
- **C7 train s5+fix+fix2+s6** (main `74d242d`, 4 commits): reservations/serial/FEFO/counts + re-hold revive + same-bin pin + Property 23. Reviews PASS (fix2 counter-proposal accepted by Stylus ruling (a): same-bin returns are the design; ADR-0032 line recorded).
- **parseTime fix + TS train s2+s3** (main `f1fb4a0`): CLI ledger via EndpointConnector (ParseTime+UTC; found by F7s3 E2E) + stamped DB names/sweep + posting shared-DB (396s → 14s). Reviews PASS.
- **F4 slice 1** (main `9cb58bd`, 17 commits): builder conditions engine (tree codec Go+TS, date(), eval/sets/simulate), Rules Builder modes (migration 24), lists service + gopdf PDF + CLI export + Desk UI + QueueMessages. gopdf v0.38.6 cleared after maroto failed the size budget (+15MB vs +1.2MB). Review PASS. Gate 1448s.
- **CLI markers fix** (`ff18cb2`): F4s1 merge had left conflict markers in help text (compiled + passed tests; spotted by w-c1). Keep-both, build+vet green.
- **TS slice 4** (main `12ee455`, 2 commits): shared-DB for ledger/jobs/events/stock + server stragglers. Review PASS. Gate 836s: posting 18.8s (was 853), server 223s, ledger 7.5s, jobs 11.6s, events 16.4s, stock 32.6s; auth 532s still the pole.
- **C7 slice 7 + fix** (main `42b56ec`, 2 commits): stock-to-GL check + LogID doc fix. Review PASS (1 cycle). Gate 704s (21 Go ok, 717 web). Pushed during C10s2 gate per standing ruling.
- **C10 slices 1–2** (main `7cff511`, 2 commits): ADR-0035 + provider inbox (HMAC, dedupe, Operation_ID). Review PASS. Gate 784s (21 Go ok, 717 web).
- **F3 slice 2 + fix** (main `05e222b`, 2 commits): compact builder per condition; review found peek-deletes-expression (fixed + proven regression test). Gate 723s (21 Go ok, 724 web). Server restarted (t7n). F3 COMPLETE.
- **C7 slice 8** (main `679e519`, 1 commit): Property 22 + 2 in-slice bugfixes (LIFO guards, value-sign legs). Review PASS. Gate 725s (22 Go ok, 724 web).
- **C5 slice 6 backend** (main `ca6ad9d`, 1 commit): settlement RPCs + CLI + ledger:close; stale .fail deleted. Review PASS. Gate 694s (22 Go ok, 724 web; clean=false was a queue-time-head artifact). Server restarted (t7r). First push hit transient GitHub 500, retry green.
- **C5 slice 8** (main `fd21f66`, 1 commit): open-item creation + ADR-0031 amend. Review PASS. Gate 759s (22 Go ok, 724 web). Server restarted (t7w).
- **F4 train slices 2+3** (main `2d9b2cc`, 11 commits): lists:send + QueueMessages + lists Desk UI + Advanced filter + saved-list chips. Review PASS after 1 rebase-only cycle (inherited markers). Gate 693s (22 Go ok, 735 web). Server restarted (t7z). Unblocks C5s7, C7s9b, F5.
- **TS slice 5** (main `0897792`, 4 commits): auth redundant-Up drop + cmd shared-DSN + GO_TEST_P ?= 4 (suite -p=4 in tooling). -p 4 wins 39% (513s vs 836s). Review PASS. Gate 561s (22 Go ok, 735 web). L7 resting; slot to F5.

## In progress (by lane, main 0897792)

- **L1 Books and posting:** C5s6 + C5s8 merged; C5s9 PASS with r-c5s9; w-c1 building slice 7 Desk (settle-from-doc + prefill, Stylus-ruled). C5 ticks at slice 7; C1 slice 7 (Z2) after C5.
- **L2 Jobs, events, integration:** C10s3 PASS (migration 25 + Property 26) with r-c10s3; w-c2 building slice 4 (attachments/search/comments/import).
- **L3 Masters and Packs:** C7s9 backend with r-c7s9 (split ruled); w-c7 on 9b RecordsPage columns + slice-10 docs. Slice 11 wiring after.
- **L4 Desk:** F3 COMPLETE. w-f2 (fresh tree) on F5 slice 1 (Circle Space sample; brief + Stylus additions binding).
- **L5 Quality:** F4 slices 1–3 merged (F4 unticked); w-fu building slice 4 (field compare + Link hop, ≤2-hop ruling). FU batch 3 after.
- **L6 Pilot and shipping:** F7 slice 3 rebased past C5s8 (settle integrated, E2E 136/136), re-gating. F7 ticks on merge; F8 waits for tap owner.
- **L7 Test speed:** RESTING per Stylus ruling (slices 1–5 merged, full gate ~9min at -p 4); w-ts stood down, branch kept. Slice 6 (files, secrets, auth) when a lane frees.

## Blocked and why

- F8 waits for the user's tap-owner answer.
- C5s7 + C7s9b share RecordsPage.vue (both live; tight-hunk warnings sent; master resolves at merge).
- F4 ticks at slice 4 (field-to-field + Link hop, building); F5 slice 3 uses it if merged, else pending.
- TS slice 6 + C1 slice 7 wait for free lanes. Disk recovered to ~9GB (user cleared caches).

## Questions for the user

Z10 licence — Satori is Personal-Use trial only (no licence/binaries in repo): buy a web licence or relent (interim: system stack on public pages, Desk loads Satori via local() only). F8 Homebrew tap owner. Notifications full-list page design. Copyright holder for LICENSE/NOTICE (interim: 'Copyright 2026 Athena contributors'; user to confirm).

## Errors and how they were handled

- C1 6.11 + C4s3 + F6 reviews each found one real issue (false docs claim; 13dp corruption; step-ms/doc mismatch): one fix each, re-reviewed.
- Go contention timeouts (w-d3/w-c4 20m, sm-c2 probable OOM): slot classes now serialize ALL Go on slot 0; browser/web on slot 1; 30/15-min watchdog.
- SIGTERM on sm-c2's running gate logged a misleading red + orphaned test procs: reaped all; suite now kills the process group and logs `stopped`; stops only via `suite stop`.
- Mac idle-slept 03:41–05:07: caffeinate terminal guards the rest of the night (never close until clock-out).
- Two C7s1 merge gates ran hollow (no .env.local in the fresh staging tree): caught pre-merge by package timings, voided, full-night audit run, real 1042s re-gate green; suite now refuses Go runs without DB env; MASTER.md staging rule fixed.
- `make restart` from a managed session owned the user server (t5f died): server moved to hc terminal `athena-server` (no-parent/outlive, node PATH); MASTER.md merge rule now names the hc procedure; ~1 min downtime.
- Stale ticks + future-dated log lines: re-derive every tick from log tail + locks; timestamps from `date`, verified against comms.jsonl.
- Rogue direct `go test` during a gate (w-c7) + stale worktree `suite -p 8` (w-fu): suite-only rule + absolute main-repo tool paths broadcast; narrow -run <1min only.
- npm registry flakes: shared pnpm store via CACHE_DIR on all gates.
- Cold-cache 20m posting/server timeouts root-caused (Argon2 CPU starvation → -p 8→4→2 stopgaps → per-test DDL fsync storm found via probe): TS slices 1–4 are the structural fix (posting 853s → 19s in full runs).
- mid-run `go clean -cache` red + disk-floor spawn refusals: ticker + suite self-clean at slot boundaries only; merged trees removed after every merge.
- Unlogged worktree removals (merge9/c5-s5-r, ts-s4-r): main verified intact; forward rule is log every removal as CLOSED with path. LATER SOLVED: reviewers self-remove their -r trees on verdict (6 instances) — benign, now expected.
- Bogus `suite lanes` invocation by master (no such subcommand): logged exit-1 triple, 0s, no slot harm; corrected in-log; Stylus added an unknown-command guard to suite.
- Suite `clean=false` on the C5s6 merge gate was a queue-time-head artifact of the Stylus-ordered mid-queue rebase (reflog proved the gate ran on the rebased tip); suggestion for Stylus: capture head at run start.
- Transient GitHub 500 on a push (remote Internal Server Error); retry green.
- Orphaned go-build temps (~2GB from killed runs) + full disk blocked spawns twice; master cleans stale temps, user cleared tool caches.
- Conflict markers shipped inside a Go string by the F4s1 rebase (compiled + passed all tests): fixed in `ff18cb2`; suite now refuses Go runs on marked trees; marker-grep added to the merge checklist.

## Next up

First verdict (C10s3/C5s9/C7s9) → tip gate → merge; F7s3 PASS → review; F4s4 + C5s7 + C7s9b + C10s4 + F5s1 builds; F8 needs tap owner; TSs6 + C1s7 wait for lanes.
