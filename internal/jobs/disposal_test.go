package jobs_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"athena-erp/internal/jobs"
)

// oldTime predates every retention period; newTime is fresh.
var (
	oldTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
)

func seedSystemRows(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	stmts := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO athena_access_log (at, event, outcome) VALUES (?, 'login', 'ok')`, []any{oldTime}},
		{`INSERT INTO athena_access_log (at, event, outcome) VALUES (?, 'login', 'ok')`, []any{newTime}},
		{`INSERT INTO athena_outbox (id, event_type, payload, status, created_at) VALUES ('01911111-2222-7333-8444-555555555551', 't.old', '{}', 'dispatched', ?)`, []any{oldTime}},
		{`INSERT INTO athena_outbox (id, event_type, payload, status, created_at) VALUES ('01911111-2222-7333-8444-555555555552', 't.old', '{}', 'pending', ?)`, []any{oldTime}},
		{`INSERT INTO athena_outbox (id, event_type, payload, status, created_at) VALUES ('01911111-2222-7333-8444-555555555553', 't.new', '{}', 'dispatched', ?)`, []any{newTime}},
		{`INSERT INTO athena_events (id, event_type, producer, subject_type, subject_id, correlation_id, payload, created_at) VALUES ('01911111-2222-7333-8444-555555555554', 't.old', 'p', 's', 's1', '01911111-2222-7333-8444-555555555555', '{}', ?)`, []any{oldTime}},
		{`INSERT INTO athena_events (id, event_type, producer, subject_type, subject_id, correlation_id, payload, created_at) VALUES ('01911111-2222-7333-8444-555555555556', 't.new', 'p', 's', 's1', '01911111-2222-7333-8444-555555555555', '{}', ?)`, []any{newTime}},
		{`INSERT INTO athena_jobs (id, kind, payload, status, created_at) VALUES ('01911111-2222-7333-8444-555555555557', 'k', '{}', 'succeeded', ?)`, []any{oldTime}},
		{`INSERT INTO athena_jobs (id, kind, payload, status, created_at) VALUES ('01911111-2222-7333-8444-555555555558', 'k', '{}', 'queued', ?)`, []any{oldTime}},
		{`INSERT INTO athena_jobs (id, kind, payload, status, created_at) VALUES ('01911111-2222-7333-8444-555555555559', 'k', '{}', 'succeeded', ?)`, []any{newTime}},
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func candidateIDs(candidates []jobs.Candidate) map[string]bool {
	got := map[string]bool{}
	for _, c := range candidates {
		got[c.RecordType+"/"+c.RecordID] = true
	}
	return got
}

func TestSystemCandidates(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	seedSystemRows(t, db)
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	candidates, err := jobs.SystemCandidates(ctx, db, now)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	got := candidateIDs(candidates)
	// The old access log row carries its decimal id; fetch it.
	var logID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM athena_access_log WHERE at = ?`, oldTime).Scan(&logID); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"access_log/" + strconv.FormatInt(logID, 10):  true,
		"outbox/01911111-2222-7333-8444-555555555551": true,
		"events/01911111-2222-7333-8444-555555555554": true,
		"jobs/01911111-2222-7333-8444-555555555557":   true,
	}
	if len(got) != len(want) {
		t.Fatalf("candidates are %v, want %v", got, want)
	}
	for key := range want {
		if !got[key] {
			t.Errorf("candidate %s missing, got %v", key, got)
		}
	}
	for _, c := range candidates {
		if c.Action != jobs.ActionDelete {
			t.Errorf("candidate %s/%s acts %q, want delete", c.RecordType, c.RecordID, c.Action)
		}
	}
}

func seedGuestTable(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE p_workspace__guestbook (id CHAR(36) NOT NULL PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("create guest table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO p_workspace__guestbook (id, created_at) VALUES ('01911111-2222-7333-8444-555555555561', ?), ('01911111-2222-7333-8444-555555555562', ?)`, oldTime, newTime); err != nil {
		t.Fatalf("seed guests: %v", err)
	}
}

func TestRecordCandidates(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	seedGuestTable(t, db)
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	guest := jobs.RecordProfile{Key: "guestbook", Class: jobs.ClassGuest}
	candidates, err := jobs.RecordCandidates(ctx, db, now, []jobs.RecordProfile{guest})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(candidates) != 1 || candidates[0].RecordID != "01911111-2222-7333-8444-555555555561" {
		t.Fatalf("candidates are %+v, want only the old guest row", candidates)
	}
	if candidates[0].Action != jobs.ActionAnonymise {
		t.Fatalf("guest action is %q, want anonymise", candidates[0].Action)
	}
	held := guest
	held.Hold = true
	ledger := jobs.RecordProfile{Key: "guestbook", Class: jobs.ClassLedger}
	candidates, err = jobs.RecordCandidates(ctx, db, now, []jobs.RecordProfile{held, ledger})
	if err != nil {
		t.Fatalf("preview held/ledger: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("held and ledger profiles listed %+v, want none", candidates)
	}
	if _, err := jobs.RecordCandidates(ctx, db, now, []jobs.RecordProfile{{Key: "guestbook", Class: "forever"}}); err == nil {
		t.Fatal("unknown class preview succeeded, want an error")
	}
	if _, err := jobs.RecordCandidates(ctx, db, now, []jobs.RecordProfile{{Key: "guestbook", Class: jobs.ClassGuest, Trigger: "someday"}}); err == nil {
		t.Fatal("unknown trigger preview succeeded, want an error")
	}
	if _, err := jobs.RecordCandidates(ctx, db, now, []jobs.RecordProfile{{Key: "nope; DROP", Class: jobs.ClassGuest}}); err == nil {
		t.Fatal("bad key preview succeeded, want an error")
	}
}

func TestHolds(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	if err := jobs.PlaceHold(ctx, db, "guestbook", "r1", "dispute", "clerk"); err != nil {
		t.Fatalf("place: %v", err)
	}
	if err := jobs.PlaceHold(ctx, db, "", "r1", "", ""); err == nil {
		t.Fatal("empty hold succeeded, want an error")
	}
	released, err := jobs.ReleaseHold(ctx, db, "guestbook", "r1")
	if err != nil || !released {
		t.Fatalf("release = %v, %v, want true, nil", released, err)
	}
	released, err = jobs.ReleaseHold(ctx, db, "guestbook", "r1")
	if err != nil || released {
		t.Fatalf("second release = %v, %v, want false, nil", released, err)
	}
}

func applyAll(t *testing.T, db *sql.DB, candidates []jobs.Candidate, links jobs.LinksTo) []jobs.Outcome {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := jobs.Apply(ctx, tx, candidates, links, "tester")
	if err != nil {
		tx.Rollback()
		t.Fatalf("apply: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return outcomes
}

func outcomeByID(outcomes []jobs.Outcome, id string) jobs.Outcome {
	for _, o := range outcomes {
		if o.Candidate.RecordID == id {
			return o
		}
	}
	return jobs.Outcome{}
}

func TestApply(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	seedSystemRows(t, db)
	seedGuestTable(t, db)
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	system, err := jobs.SystemCandidates(ctx, db, now)
	if err != nil {
		t.Fatalf("preview system: %v", err)
	}
	guest, err := jobs.RecordCandidates(ctx, db, now, []jobs.RecordProfile{{Key: "guestbook", Class: jobs.ClassGuest}})
	if err != nil {
		t.Fatalf("preview guests: %v", err)
	}
	// Hold the old outbox row; link the guest row; force a 7-year
	// candidate, a guest delete and an unknown class through.
	var heldID string
	for _, c := range system {
		if c.RecordType == jobs.SystemOutbox {
			heldID = c.RecordID
		}
	}
	if err := jobs.PlaceHold(ctx, db, jobs.SystemOutbox, heldID, "audit", "clerk"); err != nil {
		t.Fatalf("hold: %v", err)
	}
	candidates := append(system, guest...)
	candidates = append(candidates,
		jobs.Candidate{RecordType: "guestbook", RecordID: "01911111-2222-7333-8444-555555555563", Class: jobs.ClassGuest, Action: jobs.ActionDelete, Reason: "forced"},
		jobs.Candidate{RecordType: "guestbook", RecordID: "01911111-2222-7333-8444-555555555564", Class: jobs.ClassGuest, Action: jobs.ActionAnonymise, Reason: "forced"},
		jobs.Candidate{RecordType: "invoice", RecordID: "01911111-2222-7333-8444-555555555571", Class: jobs.ClassInvoice, Action: jobs.ActionDelete, Reason: "forced"},
		jobs.Candidate{RecordType: "guestbook", RecordID: "01911111-2222-7333-8444-555555555599", Class: "forever", Action: jobs.ActionDelete, Reason: "forced"},
		jobs.Candidate{RecordType: "guestbook", RecordID: "01911111-2222-7333-8444-555555555598", Class: jobs.ClassGuest, Action: "shred", Reason: "forced"},
	)
	links := func(ctx context.Context, recordType, recordID string) (bool, error) {
		return recordType == "guestbook" && recordID == "01911111-2222-7333-8444-555555555561", nil
	}
	outcomes := applyAll(t, db, candidates, links)
	checks := map[string]string{
		heldID:                                 "skipped",
		"01911111-2222-7333-8444-555555555561": "skipped",
		"01911111-2222-7333-8444-555555555563": "exception",
		"01911111-2222-7333-8444-555555555564": "exception",
		"01911111-2222-7333-8444-555555555571": "skipped",
		"01911111-2222-7333-8444-555555555599": "exception",
		"01911111-2222-7333-8444-555555555598": "exception",
	}
	for id, want := range checks {
		if got := outcomeByID(outcomes, id); got.Result != want {
			t.Errorf("outcome for %s is %+v, want %s", id, got, want)
		}
	}
	// The unheld system rows deleted; the held outbox row and the
	// linked guest row survive.
	for _, c := range system {
		if c.RecordID == heldID {
			continue
		}
		table := map[string]string{jobs.SystemAccessLog: "athena_access_log", jobs.SystemOutbox: "athena_outbox", jobs.SystemEvents: "athena_events", jobs.SystemJobs: "athena_jobs"}[c.RecordType]
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE id = ?`, c.RecordID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("system row %s/%s survives apply, want it deleted", c.RecordType, c.RecordID)
		}
		if got := outcomeByID(outcomes, c.RecordID); got.Result != jobs.OutcomeDeleted {
			t.Errorf("outcome for %s is %+v, want deleted", c.RecordID, got)
		}
	}
	var heldCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_outbox WHERE id = ?`, heldID).Scan(&heldCount); err != nil || heldCount != 1 {
		t.Fatalf("held outbox row count = %d, %v, want 1, nil", heldCount, err)
	}
	var guestCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM p_workspace__guestbook WHERE id = '01911111-2222-7333-8444-555555555561'`).Scan(&guestCount); err != nil || guestCount != 1 {
		t.Fatalf("linked guest row count = %d, %v, want 1, nil", guestCount, err)
	}
	var register int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_disposal_register`).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if register != len(outcomes) {
		t.Fatalf("register holds %d rows, want one per outcome (%d)", register, len(outcomes))
	}
}

func TestReapplyRegister(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	seedSystemRows(t, db)
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	system, err := jobs.SystemCandidates(ctx, db, now)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	applyAll(t, db, system, func(ctx context.Context, recordType, recordID string) (bool, error) { return false, nil })
	// A restore brings one deleted event row back; skips reapply to nothing.
	if _, err := db.ExecContext(ctx, `INSERT INTO athena_events (id, event_type, producer, subject_type, subject_id, correlation_id, payload, created_at) VALUES ('01911111-2222-7333-8444-555555555554', 't.old', 'p', 's', 's1', '01911111-2222-7333-8444-555555555555', '{}', ?)`, oldTime); err != nil {
		t.Fatalf("restore a row: %v", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	reapplied, err := jobs.ReapplyRegister(ctx, tx)
	if err != nil {
		tx.Rollback()
		t.Fatalf("reapply: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if reapplied != 1 {
		t.Fatalf("reapplied %d rows, want 1", reapplied)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_events WHERE id = '01911111-2222-7333-8444-555555555554'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("restored event row count = %d, %v, want 0, nil", count, err)
	}
}
