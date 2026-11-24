# Review: D3 slice 4 — catalogue pages + verification + images + vitals

Verdict: PASS

Commit: c0c34c8 (`feat(d3): slice 4 catalogue + transcript verification + 1200x630 fallback + vitals`),
reviewed in /Users/iampaulking/projects/duchess/athena-erp-d3-r (detached, verified clean on arrival
and clean after teardown). One commit, 12 files, +679/-27. No blockers. Three follow-ups below
(none stops the merge under the merge-small rule).

## Gates (all green)

- Tree clean before gates; still at c0c34c8 after.
- `gofmt -l cmd internal`: empty. `go build ./...`, `go vet ./...`: pass.
- `affected` (absolute main-repo path): `Go: ./cmd/athena ./internal/server | web tests: no` —
  reused the worker's green run on the same clean commit
  (suite-logs/1124-020651-w-d3.log, `ok athena-erp/internal/server 736.855s`, no FAIL/panic).
- Reran the 12 slice-4 tests narrowly myself (`TestPublicCourses`, `CoursesEmpty`,
  `CourseImage`, `VerifyForm`, `VerifySubmit`, `VerifyResult`, `VerifyResultWithoutDate`,
  `VerifyResultEscapesValues`, `OgFallback`, `SitemapLearn`, `HomeCoursesLink`,
  `HomeWithoutIdentity`): `ok athena-erp/internal/server 207.001s`.
- `make generate` twice with node on PATH: stable, no diff (first attempt without node on
  PATH failed in `web-deps`; environmental, not the change).
- No Satori file shipped: diff carries no font files; `layout.html` keeps the system stack;
  "Satori" appears only as prose in ADR-0030 (Z10 interim (b) stands).

## Per-item findings

1. Routes: `/courses`, `/verify`, `/verify/{token}` resolve through `mountPublic`, registered
   ahead of the SPA catch-all (`server.go:154` vs `/` at `:156`; Go 1.22 mux also prefers the
   explicit patterns). Unknown/unpublished/empty states share the bare `notFound` (noindex
   header, canonical cleared, "Nothing here"). Verify form stays indexable with canonical;
   results send `X-Robots-Tag: noindex, nofollow` with no canonical — matches the brief
   ("verification results noindex") and the ADR noindex list ("the verify form itself stays
   indexable"). Live: `/courses` 200, `/verify` 200, `/verify/VFY-2026-0001` 200+noindex,
   `/verify/nope` and unpublished `VFY-2026-0003` bare 404+noindex.
2. SEO: catalogue carries title/description/canonical/OG/Twitter + JSON-LD graph
   (Organization + ItemList of Course, parsed — not eyeballed — by `graphNodes`).
   og:image chain is page image → logo → `/p/og-fallback.png`; only the fallback claims
   1200x630. The PNG is real: `file` says `PNG image data, 1200 x 630`, 15486 bytes on disk
   and identical bytes served live (`Content-Type: image/png`, `Cache-Control: public,
   max-age=86400`, no robots tag). Sitemap lists `/home` + `/courses`, never `/verify`;
   robots disallows `/verify/` etc. Live bytes confirm all of this.
3. Projection: `public_learn.go:15-19` allowlists exactly name/slug/is_published +
   description/image (course) and verification_token/holder_name/course_name/status/
   is_published + completed_on (transcript). Text slots accept Text/Long text only
   (`publicTextTypes`); date kind accepts Date only; token filter uses the fixed column
   name `verification_token` as a bound parameter, never request text
   (`public_project.go:128-134`); every query keeps the `is_published` + live-row filters.
   No RecordService/Grants/session references in the new code; logs carry the route only;
   token is never rendered back (test asserts absence; live result page has no `VFY-`,
   no canonical, no og:image). Unpublished rows and secret fields stay out (tests + live).
4. Escaping: all markup through `html/template` `{{ }}`; the only `template.JS` is
   `publicJSONLD` encoder output (`encoding/json` escapes `<>&`, so `</script>` cannot
   break out) — no `template.HTML` anywhere in public code. `TestPublicVerifyResultEscapesValues`
   proves stored-XSS escaping; my live adversarial read of
   `/verify/%3Cscript%3Ealert(1)%3C/script%3E` 404s with zero `script` echoes, and the
   `?token=` redirect 303s through `url.PathEscape` to a relative path.
5. Vitals: harness is not committed, so judged the method per the brief. The ADR note
   (table + methodology paragraph) exists and matches the worker's claim: headless Chrome
   158, loopback, unthrottled, 1280x800, CDP harness reading buffered LCP/layout-shift
   entries after load; LCP 28–52 ms, CLS 0 on all 7 pages; INP honestly recorded as
   unmeasured (zero EventTiming entries) rather than zero, with the future-handlers risk
   stated. The numbers are plausible for single-HTML no-JS pages on loopback and the
   caveats (loopback flatters server time) are written down. Not faked — accepted, with
   follow-up (3) on what "after load" CLS misses.
6. ADR-0030 touch: route-table rows for `/p/img/...` + `/p/og-fallback.png`, slice-4
   projection specifics (text-slot types, exact token match, date format, no catalogue
   detail pages), og-dimension rule ("only the fallback claims dimensions" — true in
   `public_shop.go:104-110` + product override at `:326-327`), and the vitals table. It
   says what the code does. Decision stays Proposed.
7. Tests: `public_learn_test.go` (326 lines) + `public_test.go` edits cover status codes,
   per-page tags, parsed JSON-LD, escaping, 404s (missing type, unpublished, bad hash,
   wrong kind), noindex presence/absence, fallback PNG decode + 1200x630 assert, sitemap
   membership, home link on/off. Image test asserts byte equality with the stored file.
8. Scope: no migration, no .proto, no gate.go, no web/cmd changes; Z2/Z4 untouched.
   Slice-2/3 files change only where justified: `public.go` (routes, fallback embed/route,
   `coursesURLFor` wiring, `sitemapLearn`, home org-logo fix so the new og fallback never
   poses as the Workspace mark), `layout.html` (+9 form/dl CSS for the new pages),
   `public_project.go` (date kind, token filter), `public_shop.go` (og chain, dim clearing,
   course image kind), `public_test.go` (fallback tags). `home.html`'s `CoursesURL`
   conditional predates the slice (main already had it); this slice wires the value.
9. Render check: real binary (`go build ./cmd/athena`) served a throwaway migrated
   `test_athena_d3r_render` database seeded via the repo's own `seedLearn` on
   127.0.0.1:18321 (throwaway HOME for the install lock; user's server untouched).
   Raw bytes of catalogue + form + result + bad token + fallback PNG + sitemap + robots
   + home link all verified live (see item 1–2). Torn down afterwards: server stopped,
   database dropped, seed file and /tmp artifacts removed, tree clean.
10. Gates: see above.

## Follow-ups (non-blocking)

1. `server.go:154` passes `config.Workspace` to `mountPublic`, not `filesWorkspace(config)`
   like the brand handler and FileService do. Under `athena serve --files-dir` with no bound
   Workspace (the `make run` dev shape), `h.files` is nil and every `/p/img/...` is a bare
   404 while pages still render the URLs — observed live (catalogue HTML correct, course
   image 404). Pre-existing from slice 3 (product images alike), works under `athena run`;
   one-line fix, suggest the worker takes it in slice 5 or a fixup.
2. Course images carry no `width`/`height` (`courses.html` img; handler never calls
   `publicImageSize` unlike the slice-3 product page), deviating from the ADR budget line
   "explicit `width`/`height` on every image". og dims are correctly omitted (unverified),
   and measured CLS is 0 — but add `publicImageSize` to the course view for parity.
3. Vitals method note: CLS read from buffered entries after load misses post-load lazy-image
   shifts; fine given the margins, but re-measure on scroll if course images move above the fold.

## Review-process note

My first live-serve attempt used a broken `${DSN%...}` substitution (no-op), so the server
briefly connected to the dev `athena_erp` database: read-only public GETs plus serve startup
(root ensure + one worker purge row). Caught from the 404s, killed the server, re-ran with the
correct DSN against `test_athena_d3r_render` for every result above.
