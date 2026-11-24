# Summary: D3 slice 4 — catalogue + verification + images + vitals

Merged 2026-11-24 as 2377cb7 in train3 (built at a064771,
rebased cleanly twice — c0c34c8 on 58948de; patch
identical).

- New courses/verify/verifyresult templates + 1200x630
  og-fallback.png (15 KB, byte-asserted) + public_learn.go
  (+326-line tests): course catalogue (JSON-LD Course),
  transcript verification (form + result, results noindex),
  token/date projection slots (exact match, typed slots
  only), sitemapLearn, home org-logo fix.
- Vitals measured with headless Chrome 158 + CDP harness
  and recorded honestly in ADR-0030: LCP 28–52 ms, CLS 0
  on all 7 pages, INP unmeasured (no EventTiming entries).
- Z10 licence verdict (formal): no Satori licence file or
  binaries anywhere; in-repo evidence unanimous Personal
  Use Only trial via local() — web embedding NOT cleared,
  nothing bundled (interim (b): system stack stands; user
  question queued).
- ADR-0030 touch (+22): route rows, projection specifics,
  og-dimension rule, vitals table. No migration, no proto,
  no gate.go/web/cmd changes; Z2/Z4 untouched.
- Review: PASS (review-slice-4.md) — render-checked live
  (catalogue, verify form/result incl. adversarial,
  bad-token 404s, fallback bytes); 12 slice tests rerun
  green. Process note: first render briefly touched the
  dev DB (broken DSN substitution; read-only GETs + serve
  startup writes); caught, killed, redone correctly on a
  throwaway DB.
- Merge-time verification: train3 tip gate (full make test
  exit 0, web 616/616) on 2377cb7.
- Follow-ups: (1) filesWorkspace one-liner (suggested for
  slice 5); (2) course image width/height parity; (3) CLS
  re-measure on scroll if images move above the fold.
