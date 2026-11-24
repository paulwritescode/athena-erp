# Summary: C7 slices 5+fix+fix2+6 train (reservations, tracking, counts, Property 23)

Merged 2026-11-24 as fb22709 (4-car train: 43e1676 s5 + aaec26a fix1 + dcc8f44 fix2 + fb22709 s6, picks onto post-C5s5 main), 4 commits, internal/stock only.

Scope: 11.5 — reserve.go (locked takes, release/consume/expire, serial holds), tracking.go (serial lifecycle + rental on_rent with exact cancel reversals, batches + FEFO oldest-first NULLS-last, value-preserving transfers incl. quarantine + serial moves), count.go (cutoff-adjusted counts, gains at average); rentals tracked-not-consumed e2e. Fix1: re-hold revive + cancel-location exactness. Fix2: same-bin-return enforcement pin (TestReturnRequiresIssueBin) + moved-away guard — reviewer's third-location scenario ruled unpostable by design (planReturn stock.go:459; Stylus ruling (a); ADR-0032 Consequences line recorded). 11.6: Property 23 availability (available == on-hand minus active holds, 100/100).

Evidence: worker affected green (stock 299s, 45+ tests); fix1/fix2/s6 reviews green. Train gate on fb22709: exit 0, 27m (19 Go ok all fresh under suite -p 2 — posting 815s, server 588s, auth 569s, stock 204s; 684/684 web; zero FAIL, log 1124-200805). Staging verified byte-identical to branch stock files. Fast-forward, main unmoved.

Reviews: r-c7s5 slice-5 NEEDS_CHANGES→fix1→fix2 PASS (pushback adjudicated, pin real); r-c7s6 slice-6 PASS (genuine rapid, 100/100 rerun). w-c7's docs paraphrase car dropped (Stylus verbatim ADR line kept instead).
