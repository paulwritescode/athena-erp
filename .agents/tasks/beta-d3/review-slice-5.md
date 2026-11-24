# Review: D3 slice 5 — docs + checkpoint

Verdict: PASS

Commit: 9a29fb7 (`feat(d3): slice 5 docs + checkpoint (overview, README, decision log)`),
reviewed in /Users/iampaulking/projects/duchess/athena-erp-d3-r (detached, verified clean
on arrival and clean after teardown). Parent is main tip 8f3bd9d, so
`git diff main...HEAD` is the whole slice: 3 md files, +4/-1. No blockers.
Three follow-ups below (none stops the merge under the merge-small rule).

## Gates (all green)

- Tree clean before gates; still at 9a29fb7, clean after.
- `git diff main...HEAD --stat`: README.md, docs/architecture/overview.md,
  docs/decisions/decision-log.md ONLY — no web/, server, templates, proto,
  gate, cmd, tasks.md. (The brief's `c0c34c8...HEAD` range spans the rebase:
  c0c34c8 is the pre-rebase slice-4 commit, merged as 2377cb7; the docs delta
  is judged against main, where it is docs-only.)
- `affected` (absolute main-repo path): `affected: no Go or web change:
  nothing to run (docs only)`, exit 0 — matches the worker's claim.
- `make generate` twice with node on PATH: both exit 0, tree clean after.
- No suite re-runs, no browser re-run (evidence reuse per the brief).

## Per-item findings

1. Docs delta — accurate, links resolve, no stale claims. All three hunks read
   against ADR-0030, the merged code and reviews 2–4 (all PASS):
   - README bullet: page list (home, storefront, category, product, course
     catalogue, transcript verification) matches the routes; fixed projections,
     per-Workspace sitemap/robots, noindex on sign-in (= `/` Desk, no separate
     route — slice-2 review) / Desk / verification results, `public.base_url`
     provisional while loopback — all true. ADR-0030 link resolves.
   - Overview diagram row + table row: projection rule (allowlist,
     `is_published`, liveness, never through RecordService) per slices 2–4;
     "every indexable page carries ... a page image, logo or embedded 1200x630
     fallback" holds via `applyBrand` (public_shop.go:93-111, called by home,
     shop, category, product, courses and the verify form; result/404 pages are
     non-indexable so exempt); noindex list, canonical rule, O-008 (resolves to
     the decision-log O-008 row), vitals (2026-11-23, LCP 28–52 ms = ADR table
     min/max, CLS 0 on all seven pages, INP unmeasured — all verbatim from
     ADR-0030:101-113), system font stack per Z10. Link resolves.
   - Decision-log row: slice 2–4 contents, vitals, O-008, "system font stack
     per Z10 interim (b) until the user clears Satori" — self-contained and
     true (see follow-up 2 on the "(b)" label). Link resolves. Stays Proposed.
2. Evidence chain — intact, logs show what the worker claims:
   - Full `make test` on c0c34c8 (suite-logs/1124-025801-w-d3.log, exit 0):
     every Go package `ok` (incl. internal/server 818.940s), web 63 files /
     616 tests passed, zero FAIL (grep count 0; the one "fail" string is the
     `notFound.test.ts` case name "keeps the failed address in the bar").
   - Browser pass on c0c34c8 (suite-logs/1124-033657-w-d3.log, exit 0): exactly
     24 CHECKPOINT-PASS, 0 FAIL, CHECKPOINT-ALL-GREEN — view-source tags for
     all 7 pages + unknown-token 404+noindex + sitemap/robots/fallback, Desk
     shell checks, headless Chrome render of all 7 public pages + `/`, 2
     screenshots. The surviving /tmp/d3-checkpoint/pass.sh (read, not run)
     confirms each line's rigor: per-page title/canonical/tag asserts,
     python `json.loads` JSON-LD parse per page, header noindex asserts,
     Chrome `--dump-dom` per page. Interim harness-PASS rule never triggered.
   - Corroboration: clean-tree train3 gate on 2377cb7 (same code, rebased)
     also exit 0 with web 616/616 (suite-logs/1124-023912-master.log).
   - Caveat (not a blocker): both worker runs record `clean: false` — the
     full suite ran with the uncommitted slice-5 docs on top, and the browser
     pass used an uncommitted `TestCheckpointServe` helper (absent from the
     tree). Docs cannot change test outcomes and the clean train3 gate
     corroborates the code; noted as follow-up 1.
3. Desk untouched — `main...HEAD` shows .md files only (zero web/ diff), and
   the browser log lines (not a re-serve) confirm `/ serves Desk + noindex`
   and `/build/overview SPA fallback + noindex`.
4. Scope — no migration, no .proto, no gate.go, no cmd, no web/; tasks.md
   untouched so Z2/Z4 untouched. Slice-4 follow-ups (filesWorkspace one-liner,
   course image w/h parity, CLS scroll re-measure) remain open — out of scope
   for a docs slice, carried forward (follow-up 3).
5. Gates here — see above.

## Follow-ups (non-blocking)

1. Evidence runs were on dirty trees (uncommitted docs + throwaway serve
   helper). Harmless here and corroborated by the clean train3 gate, but
   future checkpoints should run gates on the committed tree so `clean: true`
   is recorded.
2. "Z10 interim (b)" is defined only in agent coordination notes (night-log,
   slice-4 brief/review/summary). The decision-log row spells out its meaning
   inline, so no false claim; consider linking the licence verdict when Z10
   lands instead of the bare "(b)".
3. Slice-4 follow-ups still open: (1) `mountPublic` should take
   `filesWorkspace(config)` so `/p/img/...` works under `make run`;
   (2) course images lack `width`/`height` parity with products;
   (3) re-measure CLS on scroll if course images move above the fold.
