# Summary: D3 slice 3 — commerce pages

Merged 2026-11-23 as db0f85b in train2 (built at 9cbe313 on
919bab4; parent is main, no rebase needed).

- New `public_shop.go` + `public_project.go`, shop/category/
  product templates: /shop, /shop/c/{slug}, /shop/p/{slug},
  hash-pinned /p/img route (immutable cache, nosniff, SVG
  sandbox CSP); unknown/unpublished slugs share the slice-2
  notFound (noindex, no canonical, no path echo).
- SEO per page: unique title/description, canonical, OG +
  Twitter summary_large_image, one JSON-LD @graph
  (Organization + BreadcrumbList + Product with offers);
  dimension tags only on true decoded claims.
- Projection allow-lists match the ADR table exactly;
  masters/docstate liveness enforced; no RecordService/
  Grants/sessions; values never in logs; IDs never rendered.
- Escaping by construction, live-verified with adversarial
  names (no commerce escaping test — follow-up).
- Sitemap gains shop + published rows with lastmod;
  carts/checkout/tracking still absent (forward rule only).
- No migration, no proto, no gate.go/web/cmd changes.
- Review: PASS (review-slice-3.md); render-checked live on a
  throwaway Workspace; 17/17 TestPublic green.
- Merge-time verification: train2 tip gate (full make test)
  on 2d9c17d.
- Follow-ups (a–f): adversarial escaping test; shadowed
  dimension fields; 500-vs-404 degradation; empty-category
  sitemap listing; CoursesURL placeholder; slice-2 notes
  (a)/(c) stand.
