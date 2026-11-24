// Package jobs_test holds the database-backed job runner tests. It is an
// external test package so it can migrate with server.SystemMigrations
// without an import cycle.
package jobs_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"athena-erp/internal/jobs"
	"athena-erp/internal/store"

	"pgregory.net/rapid"
)

func setup(t *testing.T) *sql.DB {
	t.Helper()
	return sharedDatabase(t)
}

func runner(db *sql.DB) *jobs.Runner {
	return &jobs.Runner{DB: db, Handlers: map[string]jobs.Handler{}}
}

func jobStatus(t *testing.T, db *sql.DB, id string) (string, int, string) {
	t.Helper()
	var status, lastError string
	var attempts int
	if err := db.QueryRow(`SELECT status, attempts, IFNULL(last_error, '') FROM athena_jobs WHERE id = ?`, id).
		Scan(&status, &attempts, &lastError); err != nil {
		t.Fatalf("job row: %v", err)
	}
	return status, attempts, lastError
}

func TestEnqueueAndRunSucceeds(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "add", Payload: map[string]any{"left": 2}})
	if err != nil {
		t.Fatal(err)
	}
	var ran atomic.Int32
	r := runner(db)
	r.Handlers["add"] = func(_ context.Context, _ *sql.Tx, job jobs.Job) (map[string]any, error) {
		ran.Add(1)
		if job.Payload["left"] != float64(2) {
			return nil, fmt.Errorf("payload = %v", job.Payload)
		}
		return map[string]any{"sum": 3}, nil
	}
	done, err := r.RunNext(ctx)
	if err != nil || !done {
		t.Fatalf("run = %v, %v, want true", done, err)
	}
	if ran.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", ran.Load())
	}
	if status, attempts, _ := jobStatus(t, db, id); status != "succeeded" || attempts != 0 {
		t.Fatalf("status %q attempts %d, want succeeded 0", status, attempts)
	}
	var result string
	if err := db.QueryRow(`SELECT CAST(result AS CHAR) FROM athena_jobs WHERE id = ?`, id).Scan(&result); err != nil || result != `{"sum":3}` {
		t.Fatalf("result = %q, %v, want the handler map", result, err)
	}
	done, err = r.RunNext(ctx)
	if err != nil || done {
		t.Fatalf("second run = %v, %v, want nothing due", done, err)
	}
}

func TestUnknownKindSitsQueued(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "future-work"})
	if err != nil {
		t.Fatal(err)
	}
	done, err := runner(db).RunNext(ctx)
	if err != nil || done {
		t.Fatalf("run = %v, %v, want nothing due", done, err)
	}
	if status, _, _ := jobStatus(t, db, id); status != "queued" {
		t.Fatalf("status = %q, want queued", status)
	}
}

func TestFailureRetriesThenParksDeadWithOwner(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "flaky", MaxAttempts: 2, OwnerRole: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	r := runner(db)
	r.Backoff = func(int) time.Duration { return 0 }
	r.Handlers["flaky"] = func(_ context.Context, _ *sql.Tx, _ jobs.Job) (map[string]any, error) {
		calls.Add(1)
		return nil, fmt.Errorf("downstream is down")
	}
	if _, err := r.RunNext(ctx); err == nil {
		t.Fatal("first run succeeded, want the handler error")
	}
	if status, attempts, last := jobStatus(t, db, id); status != "queued" || attempts != 1 || last == "" {
		t.Fatalf("after failure: %q %d %q, want queued 1 with an error", status, attempts, last)
	}
	if _, err := r.RunNext(ctx); err == nil {
		t.Fatal("second run succeeded, want the handler error")
	}
	if status, attempts, _ := jobStatus(t, db, id); status != "dead" || attempts != 2 {
		t.Fatalf("status %q attempts %d, want dead 2", status, attempts)
	}
	var ownerRole string
	if err := db.QueryRow(`SELECT owner_role FROM athena_jobs WHERE id = ?`, id).Scan(&ownerRole); err != nil || ownerRole != "operator" {
		t.Fatalf("owner = %q, %v, want operator", ownerRole, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2", calls.Load())
	}
	done, err := r.RunNext(ctx)
	if err != nil || done {
		t.Fatalf("run after dead = %v, %v, want nothing due", done, err)
	}
}

func TestRunnerDefaultOwnerTakesUnownedFailures(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "doomed", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := runner(db)
	r.DefaultOwnerRole = "sysadmin"
	r.Handlers["doomed"] = func(_ context.Context, _ *sql.Tx, _ jobs.Job) (map[string]any, error) {
		return nil, fmt.Errorf("always fails")
	}
	if _, err := r.RunNext(ctx); err == nil {
		t.Fatal("run succeeded, want the handler error")
	}
	var ownerRole string
	if err := db.QueryRow(`SELECT owner_role FROM athena_jobs WHERE id = ?`, id).Scan(&ownerRole); err != nil || ownerRole != "sysadmin" {
		t.Fatalf("owner = %q, %v, want sysadmin", ownerRole, err)
	}
}

func TestFutureJobWaitsForItsRunAt(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "later", RunAt: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	r := runner(db)
	r.Handlers["later"] = func(_ context.Context, _ *sql.Tx, _ jobs.Job) (map[string]any, error) { return nil, nil }
	done, err := r.RunNext(ctx)
	if err != nil || done {
		t.Fatalf("run = %v, %v, want nothing due", done, err)
	}
	if status, _, _ := jobStatus(t, db, id); status != "queued" {
		t.Fatalf("status = %q, want queued", status)
	}
}

func TestRecoverReleasesExpiredLeases(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	r := runner(db)
	stuck, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "work"})
	if err != nil {
		t.Fatal(err)
	}
	held, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "work"})
	if err != nil {
		t.Fatal(err)
	}
	past, future := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'leased', lease_owner = 'dead-worker', lease_expires_at = ?, fencing = 1 WHERE id = ?`, past, stuck); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'leased', lease_owner = 'live-worker', lease_expires_at = ? WHERE id = ?`, future, held); err != nil {
		t.Fatal(err)
	}
	released, dead, err := r.Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if released != 1 || dead != 0 {
		t.Fatalf("recovered %d released %d dead, want 1 and 0", released, dead)
	}
	if status, attempts, _ := jobStatus(t, db, stuck); status != "queued" || attempts != 1 {
		t.Fatalf("stuck status = %q attempts %d, want queued with one attempt counted", status, attempts)
	}
	if status, _, _ := jobStatus(t, db, held); status != "leased" {
		t.Fatalf("held status = %q, want leased", status)
	}
}

func TestRecoverParksExhaustedLeasesDead(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "work", MaxAttempts: 3, OwnerRole: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'leased', lease_owner = 'dead-worker', lease_expires_at = ?, attempts = 3 WHERE id = ?`, past, id); err != nil {
		t.Fatal(err)
	}
	released, dead, err := runner(db).Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if released != 0 || dead != 1 {
		t.Fatalf("recovered %d released %d dead, want 0 and 1", released, dead)
	}
	if status, _, _ := jobStatus(t, db, id); status != "dead" {
		t.Fatalf("status = %q, want dead", status)
	}
}

func TestRecoverHonorsRowMaxAttempts(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "work", MaxAttempts: 10})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'leased', lease_owner = 'dead-worker', lease_expires_at = ?, attempts = 7 WHERE id = ?`, past, id); err != nil {
		t.Fatal(err)
	}
	released, dead, err := runner(db).Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if released != 1 || dead != 0 {
		t.Fatalf("recovered %d released %d dead, want 1 and 0", released, dead)
	}
	if status, _, _ := jobStatus(t, db, id); status != "queued" {
		t.Fatalf("status = %q, want queued", status)
	}
}

func TestExpiredLeaseReclaimCountsAnAttempt(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	id, err := jobs.Enqueue(ctx, db, jobs.EnqueueOptions{Kind: "work", MaxAttempts: 5})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'leased', lease_owner = 'dead-worker', lease_expires_at = ?, attempts = 2 WHERE id = ?`, past, id); err != nil {
		t.Fatal(err)
	}
	var ran atomic.Int32
	r := runner(db)
	r.Handlers["work"] = func(_ context.Context, _ *sql.Tx, job jobs.Job) (map[string]any, error) {
		ran.Add(1)
		if job.Attempts != 3 {
			return nil, fmt.Errorf("attempts = %d, want 3 after the reclaim", job.Attempts)
		}
		return nil, nil
	}
	done, err := r.RunNext(ctx)
	if err != nil || !done {
		t.Fatalf("run = %v, %v, want true", done, err)
	}
	if ran.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", ran.Load())
	}
	if status, attempts, _ := jobStatus(t, db, id); status != "succeeded" || attempts != 3 {
		t.Fatalf("status %q attempts %d, want succeeded 3", status, attempts)
	}
}

func TestEnqueueIfAbsentDoesNotPileUp(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	first, err := jobs.EnqueueIfAbsent(ctx, db, jobs.EnqueueOptions{Kind: "purge_expired"})
	if err != nil || !first {
		t.Fatalf("first = %v, %v, want true", first, err)
	}
	second, err := jobs.EnqueueIfAbsent(ctx, db, jobs.EnqueueOptions{Kind: "purge_expired"})
	if err != nil || second {
		t.Fatalf("second = %v, %v, want false", second, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM athena_jobs WHERE kind = 'purge_expired'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("purge jobs = %d, %v, want 1", count, err)
	}
}

func TestOccurrenceEnsureDispatchAndComplete(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	stored, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
		Workspace: "acme", Schedule: "nightly", Scope: "", Period: "2026-11-22", Timezone: "Africa/Lagos",
	})
	if err != nil || !stored {
		t.Fatalf("ensure = %v, %v, want true", stored, err)
	}
	stored, err = jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
		Workspace: "acme", Schedule: "nightly", Scope: "", Period: "2026-11-22",
	})
	if err != nil || stored {
		t.Fatalf("second ensure = %v, %v, want false", stored, err)
	}
	jobID, dispatched, err := jobs.DispatchDue(ctx, db, jobs.DispatchOptions{
		Workspace: "acme", Schedule: "nightly", Kind: "close-day",
		Payload: map[string]any{"book": "main"},
	})
	if err != nil || !dispatched {
		t.Fatalf("dispatch = %v, %v, want a job", dispatched, err)
	}
	occurrence, err := jobs.GetOccurrence(ctx, db, "acme", "nightly", "", "2026-11-22")
	if err != nil {
		t.Fatal(err)
	}
	if occurrence.Status != "leased" || occurrence.JobID != jobID || occurrence.Timezone != "Africa/Lagos" {
		t.Fatalf("occurrence = %+v, want leased with the job and its timezone", occurrence)
	}
	r := runner(db)
	r.Handlers["close-day"] = func(_ context.Context, _ *sql.Tx, job jobs.Job) (map[string]any, error) {
		carried, ok := job.Payload["occurrence"].(map[string]any)
		if !ok || carried["period"] != "2026-11-22" {
			return nil, fmt.Errorf("payload = %v, want the occurrence identity", job.Payload)
		}
		return map[string]any{"closed": true}, nil
	}
	done, err := r.RunNext(ctx)
	if err != nil || !done {
		t.Fatalf("run = %v, %v, want true", done, err)
	}
	occurrence, err = jobs.GetOccurrence(ctx, db, "acme", "nightly", "", "2026-11-22")
	if err != nil {
		t.Fatal(err)
	}
	if occurrence.Status != "done" || occurrence.Watermark == nil || occurrence.Result["closed"] != true {
		t.Fatalf("occurrence = %+v, want done with a watermark and result", occurrence)
	}
	if _, dispatched, err := jobs.DispatchDue(ctx, db, jobs.DispatchOptions{
		Workspace: "acme", Schedule: "nightly", Kind: "close-day",
	}); err != nil || dispatched {
		t.Fatalf("second dispatch = %v, %v, want nothing due", dispatched, err)
	}
}

func TestDispatchDueLeavesCallerPayloadAlone(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
		Workspace: "acme", Schedule: "nightly", Period: "2026-11-24",
	}); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"book": "main"}
	if _, _, err := jobs.DispatchDue(ctx, db, jobs.DispatchOptions{
		Workspace: "acme", Schedule: "nightly", Kind: "close-day", Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["book"] != "main" {
		t.Fatalf("caller payload = %v, want it untouched", payload)
	}
}

func TestDispatchDueHandlesLongNames(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	workspace := strings.Repeat("w", 140)
	schedule := strings.Repeat("s", 140)
	scope := strings.Repeat("c", 255)
	period := strings.Repeat("p", 64)
	if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
		Workspace: workspace, Schedule: schedule, Scope: scope, Period: period,
	}); err != nil {
		t.Fatal(err)
	}
	jobID, dispatched, err := jobs.DispatchDue(ctx, db, jobs.DispatchOptions{
		Workspace: workspace, Schedule: schedule, Scope: scope, Kind: "close-day",
	})
	if err != nil || !dispatched {
		t.Fatalf("dispatch = %v, %v, want a job", dispatched, err)
	}
	want, err := jobs.OccurrenceKey(workspace, schedule, scope, period)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 602 {
		t.Fatalf("key length = %d, want the 602-char maximum", len(want))
	}
	var stored string
	if err := db.QueryRow(`SELECT occurrence_key FROM athena_jobs WHERE id = ?`, jobID).Scan(&stored); err != nil || stored != want {
		t.Fatalf("stored key length = %d, want the full 602-char key", len(stored))
	}
	if _, _, _, _, err := jobs.SplitOccurrenceKey(stored); err != nil {
		t.Fatalf("stored key does not split: %v", err)
	}
}

func TestOccurrenceDiesWithItsJob(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
		Workspace: "acme", Schedule: "nightly", Period: "2026-11-23",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := jobs.DispatchDue(ctx, db, jobs.DispatchOptions{
		Workspace: "acme", Schedule: "nightly", Kind: "close-day", MaxAttempts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	r := runner(db)
	r.Handlers["close-day"] = func(_ context.Context, _ *sql.Tx, _ jobs.Job) (map[string]any, error) {
		return nil, fmt.Errorf("books do not balance")
	}
	if _, err := r.RunNext(ctx); err == nil {
		t.Fatal("run succeeded, want the handler error")
	}
	occurrence, err := jobs.GetOccurrence(ctx, db, "acme", "nightly", "", "2026-11-23")
	if err != nil {
		t.Fatal(err)
	}
	if occurrence.Status != "dead" || occurrence.Watermark == nil {
		t.Fatalf("occurrence = %+v, want dead with a watermark", occurrence)
	}
}

func TestCatchUpSkipsPastPeriods(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	for _, period := range []string{"2026-11-20", "2026-11-21", "2026-11-22"} {
		if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
			Workspace: "acme", Schedule: "nightly", Period: period,
		}); err != nil {
			t.Fatal(err)
		}
	}
	skipped, err := jobs.CatchUp(ctx, db, "acme", "nightly", "", "2026-11-22", jobs.CatchUpSkip)
	if err != nil || skipped != 2 {
		t.Fatalf("skipped = %d, %v, want 2", skipped, err)
	}
	current, err := jobs.GetOccurrence(ctx, db, "acme", "nightly", "", "2026-11-22")
	if err != nil || current.Status != "pending" {
		t.Fatalf("current = %+v, %v, want pending", current, err)
	}
	skipped, err = jobs.CatchUp(ctx, db, "acme", "nightly", "", "2026-11-23", jobs.CatchUpRun)
	if err != nil || skipped != 0 {
		t.Fatalf("run policy skipped = %d, %v, want 0", skipped, err)
	}
	if _, err := jobs.CatchUp(ctx, db, "acme", "nightly", "", "2026-11-23", "delete"); err == nil {
		t.Fatal("unknown policy succeeded, want an error")
	}
}

func TestEnsureOccurrenceValidates(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	for _, opts := range []jobs.EnsureOptions{
		{Schedule: "s", Period: "2026-11-22"},
		{Workspace: "w", Period: "2026-11-22"},
		{Workspace: "w", Schedule: "s"},
		{Workspace: "w", Schedule: "s", Period: "2026-11-22", Timezone: "Mars/Olympus"},
		{Workspace: "w", Schedule: "s", Period: "2026-11-22", CatchUp: "delete"},
	} {
		if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, opts); err == nil {
			t.Fatalf("ensure of %+v succeeded, want an error", opts)
		}
	}
}

func TestOccurrenceKeyRoundTrip(t *testing.T) {
	key, err := jobs.OccurrenceKey("acme", "nightly", "branch-1", "2026-11-22")
	if err != nil {
		t.Fatal(err)
	}
	workspace, schedule, scope, period, err := jobs.SplitOccurrenceKey(key)
	if err != nil || workspace != "acme" || schedule != "nightly" || scope != "branch-1" || period != "2026-11-22" {
		t.Fatalf("round trip = %q %q %q %q, %v", workspace, schedule, scope, period, err)
	}
	if _, err := jobs.OccurrenceKey("", "nightly", "", "2026-11-22"); err == nil {
		t.Fatal("empty workspace succeeded, want an error")
	}
	if _, _, _, _, err := jobs.SplitOccurrenceKey("not-a-key"); err == nil {
		t.Fatal("bad key split succeeded, want an error")
	}
}

func TestRecoverOccurrencesHealsAfterRestart(t *testing.T) {
	ctx := context.Background()
	db := setup(t)
	for _, period := range []string{"2026-11-20", "2026-11-21", "2026-11-22"} {
		if _, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
			Workspace: "acme", Schedule: "nightly", Period: period,
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := jobs.DispatchDue(ctx, db, jobs.DispatchOptions{
			Workspace: "acme", Schedule: "nightly", Kind: "close-day",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// The crash lands mid-flight: one job finished without propagating,
	// one died, one is still queued.
	var ids []string
	rows, err := db.Query(`SELECT id FROM athena_jobs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 3 {
		t.Fatalf("jobs = %d, want 3", len(ids))
	}
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'succeeded', result = '{"closed":true}' WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE athena_jobs SET status = 'dead', last_error = 'boom' WHERE id = ?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	done, dead, refreshed, err := jobs.RecoverOccurrences(ctx, db, "runner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 || dead != 1 || refreshed != 1 {
		t.Fatalf("recovered done %d dead %d refreshed %d, want 1 1 1", done, dead, refreshed)
	}
	first, err := jobs.GetOccurrence(ctx, db, "acme", "nightly", "", "2026-11-20")
	if err != nil || first.Status != "done" || first.Result["closed"] != true {
		t.Fatalf("first = %+v, %v, want done with the result", first, err)
	}
	second, err := jobs.GetOccurrence(ctx, db, "acme", "nightly", "", "2026-11-21")
	if err != nil || second.Status != "dead" {
		t.Fatalf("second = %+v, %v, want dead", second, err)
	}
}

// TestOccurrenceUniquenessProperty checks Property 15 over drawn
// schedules, scopes and periods: concurrent ensures of one occurrence
// store exactly one row, and distinct periods store distinct rows.
//
// Feature: athena-batch-2, Property 15: Occurrence uniqueness.
func TestOccurrenceUniquenessProperty(t *testing.T) {
	db := setup(t)
	timezones := []string{"UTC", "Africa/Lagos", "America/New_York"}
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		if _, err := db.Exec(`DELETE FROM athena_schedule_occurrences`); err != nil {
			t.Fatal(err)
		}
		schedule := "sched-" + rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "schedule")
		scope := rapid.StringMatching(`[a-z]{0,6}`).Draw(t, "scope")
		day := rapid.IntRange(1, 28).Draw(t, "day")
		period := fmt.Sprintf("2026-11-%02d", day)
		extra := rapid.IntRange(0, 3).Draw(t, "extra")
		racers := rapid.IntRange(2, 6).Draw(t, "racers")
		timezone := timezones[rapid.IntRange(0, len(timezones)-1).Draw(t, "timezone")]

		var stored atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				created, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
					Workspace: "prop", Schedule: schedule, Scope: scope, Period: period, Timezone: timezone,
				})
				if err != nil {
					return
				}
				if created {
					stored.Add(1)
				}
			}()
		}
		wg.Wait()
		if stored.Load() != 1 {
			t.Fatalf("concurrent ensures stored %d, want 1", stored.Load())
		}
		for i := 1; i <= extra; i++ {
			other := fmt.Sprintf("2026-10-%02d", i)
			created, err := jobs.EnsureOccurrence(ctx, db, store.MariaDB{}, jobs.EnsureOptions{
				Workspace: "prop", Schedule: schedule, Scope: scope, Period: other,
			})
			if err != nil || !created {
				t.Fatalf("distinct period %q stored = %v, %v, want true", other, created, err)
			}
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM athena_schedule_occurrences`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1+extra {
			t.Fatalf("rows = %d, want %d", count, 1+extra)
		}
	})
}
