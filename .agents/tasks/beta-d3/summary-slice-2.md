# Summary: D3 slice 2 — public plumbing + Workspace home

Merged 2026-11-23 as dde91f4 in train1 (built at 7f5d934 on
f02a4b7; rebased cleanly, tree byte-identical to reviewed
10ce8cb across all 7 slice files).

- New `internal/server/public.go` + layout/home/notfound
  templates: GET /home, per-Workspace sitemap.xml + robots.txt,
  `deskNoIndex` wrap on the SPA catch-all (hashed /assets/*
  excepted), /healthz + public 404/500 noindexed.
- /home projection: display name + logo refs via
  GetIdentityDoc + public.base_url setting only; no
  RecordService/Grants/sessions/cookies from public paths.
- SEO: title, composed description, canonical, OG/Twitter,
  Organization JSON-LD; escaping by construction
  (html/template + encoded-JSON JS); ADR-0030 amended
  (meta-description table, noindex list).
- No migration, no proto, no gate.go/web/cmd changes.
- Review: round-1 PASS at 10ce8cb (render-checked live);
  re-confirm PASS at 7f5d934 (identical tree, affected
  exit 0). Reviewer gates ran in a detached -r tree.
- Merge-time verification: train1 tip gate (full make test
  exit 0, web 613/613) on 919bab4.
- Follow-ups: ADR "POST-only RPC" wording imprecise for
  Connect v2 GET; logo used verbatim as og:image incl. SVG.
