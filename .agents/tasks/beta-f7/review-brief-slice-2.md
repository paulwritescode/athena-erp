# Review brief: F7 slice 2 athena doctor (reviewer)

Tree: /Users/iampaulking/projects/duchess/athena-erp-f7-s2-r, detached at 0b4e563 (on 3cc6022). Verify clean + diff is slice-2 scope only: cmd/athena/doctor.go + registry entry + usage stanza + README bullet + doctor_test.go.
Read: .agents/tasks/beta-f7/brief.md slice 2.

Review (design.md section 5; verdict line `Verdict: PASS` / `NEEDS_CHANGES` to master):
1. Checks: all 5 present (mariadb, migrations, disk, port, frontend/node-not-needed), each meaningful (fails when the thing is actually broken — verify at least 1 failure path live, e.g. bad port or missing DB).
2. Contract: exit 0 healthy / 1 on failure; machine line stable format; --json valid JSON with the same verdicts.
3. No-secret leak: failure output carries no DSN/password/token (read the redaction path + run one failing check and eyeball output).
4. Wiring: registry entry only (menu picks doctor up dynamically — verify listed); usage stanza + README bullet accurate.
5. Gates: affected green (reuse-or-rerun cmd/athena through suite on 0b4e563); gofmt/build/vet clean; generate stable.
6. Scope: doctor + wiring ONLY (no command behavior/flag changes elsewhere — fail if any).

Report: say master "F7 SLICE 2 REVIEW: PASS/NEEDS_CHANGES <one line>"; write .agents/tasks/beta-f7/review-slice-2.md with the verdict line.
