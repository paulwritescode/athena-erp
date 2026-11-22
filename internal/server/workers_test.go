package server

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"athena-erp/internal/auth"
	"athena-erp/internal/jobs"
)

// TestWorkersDrainJobsAndPurge starts the background workers on an
// isolated database and checks a queued job runs and the scheduled purge
// deletes a dead session, keeps a live one and logs its minimal row.
func TestWorkersDrainJobsAndPurge(t *testing.T) {
	_, records := newIsolatedServices(t)
	db := records.db
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Now().UTC()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO athena_users (id, username, display_name, state) VALUES
		('b1111111-1111-1111-1111-111111111111', 'worker-ad', 'Worker', 'active')`)
	exec(`INSERT INTO athena_sessions (id, token_hash, user_id, created_at, last_seen_at, expires_at) VALUES
		('b2222222-2222-2222-2222-222222222222', 'aa', 'b1111111-1111-1111-1111-111111111111', ?, ?, ?),
		('b3333333-3333-3333-3333-333333333333', 'bb', 'b1111111-1111-1111-1111-111111111111', ?, ?, ?)`,
		now.Add(-2*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour),
		now, now, now.Add(24*time.Hour))
	handlers := defaultJobHandlers(db)
	ran := make(chan string, 1)
	handlers["test-work"] = func(_ context.Context, _ *sql.Tx, job jobs.Job) (map[string]any, error) {
		ran <- job.ID
		return map[string]any{"ok": true}, nil
	}
	startWorkers(ctx, db, WorkerOptions{JobHandlers: handlers, PollInterval: 20 * time.Millisecond, PurgeInterval: 50 * time.Millisecond})
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "test-work"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ran:
		if got != id {
			t.Fatalf("ran job %q, want %q", got, id)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the worker did not run the queued job")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var status string
		if err := db.QueryRow(`SELECT status FROM athena_jobs WHERE id = ?`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job status = %q, want succeeded", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for {
		var purged int
		if err := db.QueryRow(`SELECT COUNT(*) FROM athena_jobs WHERE kind = 'purge_expired' AND status = 'succeeded'`).Scan(&purged); err != nil {
			t.Fatal(err)
		}
		if purged > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker did not run the purge job")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_sessions`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("sessions left = %d, %v, want 1", remaining, err)
	}
	rows, err := (&auth.Service{DB: db}).ListAccessLog(ctx, 10, 0, auth.EventExpiredPurged)
	if err != nil || len(rows) != 1 {
		t.Fatalf("purge log rows = %d, %v, want 1", len(rows), err)
	}
}
