# Summary: D3 slice 1 — ADR-0030 public pages and SEO (Proposed)

Merged 2026-11-22 as e801e78 (docs-only, no rebase needed).

- `docs/decisions/adr-0030-public-pages-and-seo.md` (new, Proposed):
  SSR route design (Go `html/template` per request, route group
  ahead of the SPA catch-all), public-projection rule (named Go
  function per page, fixed Record Type + field allowlist, direct
  store queries, never RecordService/Grants/sessions),
  per-page SEO contract (unique title/description, canonical,
  OG, Twitter `summary_large_image`, 1200x630 image, JSON-LD),
  noindex list (`X-Robots-Tag`; `/`, `/build/*`, `/view/*`,
  `/verify/{token}`, 404, RPC/telemetry; forward rule for
  carts/checkout/tracking), vitals budget (LCP ≤ 2.5 s,
  INP ≤ 200 ms, CLS ≤ 0.1 + no-client-JS construction budget).
- All six page kinds in the route table; Desk SPA stays
  (explicitly rejects SPA/SSR migration); no new tables; no Z
  decision (Z10 avoided with system fonts); consistent with
  design-system §11.
- Review: PASS, no blockers (review-slice-1.md). Non-blocking
  notes: (a) per-page meta-description sources/templates still
  to define in slices 2–4; (b) ADR-0029 gap expected (C4
  writes it); (c) `/home` vs `/`-stays-Desk flagged for owner
  review.
- Merge-time verification: docs-only; diff is exactly the 3
  docs files, no product code. No suite run.
- Open: slices 2–5 (plumbing + Workspace home, commerce pages,
  catalogue pages + vitals, docs + checkpoint).
