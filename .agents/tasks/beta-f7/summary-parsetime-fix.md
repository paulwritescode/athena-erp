# Summary: CLI parseTime fix (UTC connector)

Merged 2026-11-24 as 6ef16c3 (pick of 3a26f4d onto post-C7-train main), 1 commit, 4 files.

Scope: real product bug found by the F7s3 E2E — ledger determination/period/setting reads failed on real Workspaces (sql Scan []uint8 into *time.Time; DATE columns; parseTime missing on the CLI connector path; unit tests masked it via testdb). Fix: export server.EndpointConnector (utcConfig: ParseTime, Loc=UTC, time_zone +00:00) and use it in ledgerService; doctor.go confirmed ping-only; workspace.Connector untouched (create.go needs plain sessions). Regression test opens the ledger service on a DSN without parseTime (fails before, passes after).

Evidence: worker affected green (cmd/athena 274s, server 439s). Merge gate: exit 0, 28m cold-cache (19 Go ok, 684/684 web, zero FAIL, log 1124-205909). Pushed during the TS train gate per Stylus direct order + standing ruling (zero shared files, trial-pick clean, build+vet green — TS gate stands).

Review: r-parse PASS (review-parsetime-fix.md) — UTC fix verified, fail-before + pass-after + gates green.
