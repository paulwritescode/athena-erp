# Review brief: D3 slice 4 — catalogue pages + verification + images + vitals

You are an independent reviewer (read-only on product code). Work in
/Users/iampaulking/projects/duchess/athena-erp-d3-r (detached at c0c34c8;
verify clean on arrival). Brief:
../athena-erp/.agents/tasks/beta-d3/brief.md (slice 4 only). Priors:
review-slice-2.md, review-slice-2-reconfirm.md, review-slice-3.md (all
PASS). Contract: ADR-0030. Follow
.kiro/specs/athena-beta/design.md section 5 reviewer workflow and AGENTS.md.

Review the slice-4 diff (git diff main...HEAD — 12 files: courses/
verify/verifyresult templates, og-fallback.png new (15 KB),
public_learn.go + public_learn_test.go new, public.go/project/shop
edits, public_test.go edits, ADR-0030 +22/-?):

1. Routes: course catalogue + transcript verification (form + result)
   resolve correctly; unknown/bad tokens use notFound (noindex, no
   canonical, no echo). Verify form/result pages send noindex (brief:
   verification results noindex).
2. SEO: JSON-LD Course on catalogue; per-page title/description/
   canonical/OG as in slices 2–3; og:image chain incl. the 1200x630
   fallback PNG (verify the bytes are a real 1200x630 PNG and the
   dimension claims are true).
3. Projection rule: catalogue/verification expose exactly the
   ADR-allowed fields; token/date projection slots publish nothing
   beyond them; no RecordService/Grants/sessions; nothing sensitive
   in HTML or logs.
4. Escaping by construction (html/template; JSON-LD encoded only);
   verify with tests + one adversarial read.
5. Vitals: worker claims LCP 28–52 ms, CLS 0 on all 7 pages, INP
   unmeasured (no EventTiming entries) recorded honestly in ADR-0030.
   Verify the numbers/methodology note exists in the ADR diff; judge
   whether the harness + sample justify the claim (never accept faked
   numbers — rerun the harness if it is cheap and committed, else
   judge the method).
6. ADR-0030 touch (+22): vitals + any rule changes say what the code
   does. Confirm NO Satori bundling (Z10 interim (b): system stack
   stands; shipping any Satori file is a blocker).
7. Tests (public_learn_test.go + public_test.go edits): rerun; judge
   coverage (status codes, tags, JSON-LD, escaping, 404s, noindex,
   image bytes).
8. Confirm: no migration, no .proto, no gate.go, no web//cmd/ changes,
   Z2/Z4 untouched. Slice-2/3 files change only where justified.
9. Render check (not a full browser pass): serve a throwaway Workspace
   from this tree's binary on a free loopback port with fixture
   course data; read raw bytes of catalogue + verify form + a verify
   result + a bad token.
10. Gates: gofmt -l cmd internal (empty), go build ./..., go vet
    ./..., .agents/bin/affected by the ABSOLUTE main-repo path
    (same lock; reuse of the worker's green run on c0c34c8 is fine —
    read that log), make generate twice with no diff. Tree clean
    before any gate.

Merge-small rule: only blockers stop the merge (projection
overreach, missing noindex, unescaped output, false SEO/vitals
claims, shipped Satori file, red tests); the rest are follow-ups.
Write findings to
../athena-erp/.agents/tasks/beta-d3/review-slice-4.md with a verdict
line `Verdict: PASS` or `Verdict: NEEDS_CHANGES`, blockers first
with file:line. Then report: say master "D3 SLICE 4 REVIEW: PASS
or NEEDS_CHANGES <one line>". Then wait for instructions. Do not
edit product code, commit, merge, push or start agents. Send UPDATE
every 10 minutes.
