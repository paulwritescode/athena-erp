# Brief: D3 public pages and SEO (server-rendered) (Muse)

Worktree: /Users/iampaulking/projects/duchess/athena-erp-d3, branch feat/d3-public (from latest main).
Read: tasks.md D3; docs/design/athena-design-system.md §11 (pages by audience) + §8 (words) + §10 (brand); docs/architecture/overview.md (server shape); internal/server (how routes are served today); web/public (static surfaces).

Numbers assigned by the master: ADR-**0030** "Public pages and SEO" (Proposed). No migration is reserved: build from approved public projections with no new tables if you can; if a table or column is genuinely needed, STOP and ask master for a number (never pick one yourself). No proto changes (the public must never call the admin RecordService).

Slices, each merging on its own (report after each with say master "D3 SLICE n: PASS or BLOCKED <one line>"; one worker, sequential; rebase on main before reporting PASS and say if the rebase changed anything):
1. ADR-0030 (Proposed) + decision-log/README rows: the SSR route design (Go `html/template`, server-rendered per request), the public-projection rule (exactly which fields each page may expose; nothing else reachable), the per-page SEO contract, the noindex list, the vitals budget.
2. Server plumbing: public route group + templates layout, per-Workspace `sitemap.xml` and `robots.txt`, noindex (header or meta) on sign-in, the whole Desk, and any non-indexable route; Workspace home page (title, description, canonical, OG/Twitter tags, JSON-LD Organization). Go tests asserting status codes, tags and headers per route.
3. Commerce pages: storefront, category, product (unique title + description, canonical, OG + Twitter tags with a 1200x630 image, JSON-LD Organization/Product/BreadcrumbList). Do NOT build carts, checkout or tracking — they do not exist; document that they must send noindex when they land.
4. Catalogue pages: course catalogue, transcript verification (JSON-LD Course; verification results noindex) + the 1200x630 images. Measure Core Web Vitals (LCP ≤ 2.5 s, INP ≤ 200 ms, CLS ≤ 0.1) with headless Chrome on the new pages and record the numbers; if the harness cannot measure, say so in the report — never fake numbers.
5. Docs (overview, README, decision log) + checkpoint (generate stable, full suite, browser pass of every new page incl. view-source tag check; confirm the Desk SPA is untouched and still serves signed-in work).

Files allowed: new `internal/server/public*.go` + templates (new dir), ADR + docs, Go tests. NOT allowed: gate.go (no new RPCs), App.vue, api.ts, any proto file, any lane's files (L1 ledger.proto, L3 record_types.proto, L5 Desk pages), web/src (no Desk changes — public pages are server-rendered, not Vue). Keep the Desk SPA exactly as it is.

Rules: no Record values, credentials or session material in public output or logs; escape everything by construction (html/template); telemetry stays off unless the endpoint is set. Use throwaway Workspaces/test_athena_* databases only, never the dev database; fake HOME for any server. Full suites and browser passes only through .agents/bin/suite (it queues; never bypass the lock). Package tests may run directly. Commit on your own branch feat/d3-public; you never merge, push or start agents. If the remaining work has more than about three independent slices or one slice runs over about 90 minutes, ask master about splitting (AGENT_RULES.md).
Checks per slice: gofmt -l cmd internal (empty), go build ./..., go vet ./..., make test through suite, make generate stable.
