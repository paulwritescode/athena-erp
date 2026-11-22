// Package jobs is the durable job runner of the kernel (batch-2 G8,
// ADR-0028, Proposed): callers enqueue jobs and schedule occurrences, and
// worker loops claim them under a lease with expiry and a fencing counter,
// run them with backoff, and assign permanent failures to an owner. A
// restart reclaims expired leases, so committed work is never lost. The
// tables live in system migration 17.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"athena-erp/internal/store"

	"github.com/google/uuid"
)

// Job statuses of an athena_jobs row.
const (
	StatusQueued    = "queued"
	StatusLeased    = "leased"
	StatusSucceeded = "succeeded"
	StatusDead      = "dead"
)

// Occurrence statuses of an athena_schedule_occurrences row.
const (
	OccurrencePending = "pending"
	OccurrenceLeased  = "leased"
	OccurrenceDone    = "done"
	OccurrenceSkipped = "skipped"
	OccurrenceDead    = "dead"
)

// Catch-up policies for past-due schedule occurrences.
const (
	// CatchUpRun leaves missed occurrences pending; the worker runs them
	// late in period order.
	CatchUpRun = "run"
	// CatchUpSkip marks missed occurrences skipped without running them.
	CatchUpSkip = "skip"
)

// Classifier is the error-classification half of store.Dialect, the only
// half the runner needs to tell a duplicate occurrence from a failure.
type Classifier interface {
	Classify(error) store.ErrorKind
}

// Querier runs statements on a database or an open transaction.
type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Job is one row of athena_jobs: its kind and payload, lease, attempts,
// and, once finished, its result or its owner and error.
type Job struct {
	ID            string
	Kind          string
	Payload       map[string]any
	Status        string
	Attempts      int
	MaxAttempts   int
	NextRunAt     *time.Time
	LeaseOwner    string
	LeaseExpiry   *time.Time
	Fencing       int64
	OccurrenceKey string
	OwnerUserID   string
	OwnerRole     string
	LastError     string
	Result        map[string]any
}

// EnqueueOptions describes a job to enqueue. RunAt in the past or zero
// means due now; MaxAttempts zero means the runner default.
type EnqueueOptions struct {
	Kind          string
	Payload       map[string]any
	RunAt         time.Time
	MaxAttempts   int
	OccurrenceKey string
	OwnerUserID   string
	OwnerRole     string
}

// Enqueue stores a queued job and returns its id.
func Enqueue(ctx context.Context, db *sql.DB, opts EnqueueOptions) (string, error) {
	if opts.Kind == "" {
		return "", fmt.Errorf("jobs: enqueue needs a kind")
	}
	payload, err := json.Marshal(nonNilMap(opts.Payload))
	if err != nil {
		return "", err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	var nextRun any
	if !opts.RunAt.IsZero() {
		nextRun = opts.RunAt.UTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO athena_jobs
		(id, kind, payload, status, attempts, max_attempts, next_run_at, fencing,
		 occurrence_key, owner_user_id, owner_role)
		VALUES (?, ?, ?, 'queued', 0, ?, ?, 0, ?, ?, ?)`,
		id.String(), opts.Kind, string(payload), maxAttempts, nextRun,
		nullText(opts.OccurrenceKey), nullText(opts.OwnerUserID), nullText(opts.OwnerRole))
	if err != nil {
		return "", fmt.Errorf("jobs: enqueue: %w", err)
	}
	return id.String(), nil
}

// EnqueueIfAbsent stores a queued job of the kind unless one is already
// queued or leased, for periodic work like the purge that must not pile
// up. It reports whether it enqueued. Two loops racing may both enqueue;
// the work must be idempotent, as every handler must be.
func EnqueueIfAbsent(ctx context.Context, db *sql.DB, opts EnqueueOptions) (bool, error) {
	if opts.Kind == "" {
		return false, fmt.Errorf("jobs: enqueue needs a kind")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_jobs WHERE kind = ? AND status IN ('queued', 'leased')`,
		opts.Kind).Scan(&count); err != nil {
		return false, fmt.Errorf("jobs: check pending: %w", err)
	}
	if count > 0 {
		return false, nil
	}
	_, err := Enqueue(ctx, db, opts)
	if err != nil {
		return false, err
	}
	return true, nil
}

// DefaultMaxAttempts caps deliveries per job when the enqueue call does
// not say otherwise.
const DefaultMaxAttempts = 5

// DefaultBackoff waits 2^attempts seconds, capped at five minutes.
func DefaultBackoff(attempts int) time.Duration {
	wait := time.Second << min(attempts, 8)
	if wait > 5*time.Minute {
		return 5 * time.Minute
	}
	return wait
}

func nonNilMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}

// cloneMap copies values so the caller keeps its own map; a nil map
// becomes an empty one.
func cloneMap(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func decodeMap(raw string) map[string]any {
	out := map[string]any{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// scanJob reads a job row. The column order must match jobColumns.
func scanJob(scanner interface {
	Scan(...any) error
}) (Job, error) {
	var job Job
	var payload string
	var nextRun, leaseExpiry sql.NullTime
	var leaseOwner, occurrenceKey, ownerUser, ownerRole sql.NullString
	var lastError sql.NullString
	var result sql.NullString
	if err := scanner.Scan(&job.ID, &job.Kind, &payload, &job.Status,
		&job.Attempts, &job.MaxAttempts, &nextRun, &leaseOwner, &leaseExpiry,
		&job.Fencing, &occurrenceKey, &ownerUser, &ownerRole, &lastError, &result); err != nil {
		return Job{}, err
	}
	job.Payload = decodeMap(payload)
	if nextRun.Valid {
		at := nextRun.Time.UTC()
		job.NextRunAt = &at
	}
	job.LeaseOwner = leaseOwner.String
	if leaseExpiry.Valid {
		at := leaseExpiry.Time.UTC()
		job.LeaseExpiry = &at
	}
	job.OccurrenceKey = occurrenceKey.String
	job.OwnerUserID = ownerUser.String
	job.OwnerRole = ownerRole.String
	job.LastError = lastError.String
	if result.Valid {
		job.Result = decodeMap(result.String)
	}
	return job, nil
}

// jobColumns lists every athena_jobs column scanJob reads.
const jobColumns = `id, kind, CAST(payload AS CHAR), status, attempts, max_attempts,
	next_run_at, lease_owner, lease_expires_at, fencing, occurrence_key,
	owner_user_id, owner_role, last_error, CAST(result AS CHAR)`

// OccurrenceKey joins Workspace, schedule, scope and period into the
// occurrence key a job carries. Workspace, schedule and period must be
// non-empty; scope may be empty for the global scope. No part may hold
// the unit separator.
func OccurrenceKey(workspace, schedule, scope, period string) (string, error) {
	for _, part := range []string{workspace, schedule, period} {
		if part == "" || strings.Contains(part, "\x1f") {
			return "", fmt.Errorf("jobs: an occurrence key needs a workspace, a schedule and a period without separators")
		}
	}
	if strings.Contains(scope, "\x1f") {
		return "", fmt.Errorf("jobs: an occurrence scope must not hold a separator")
	}
	return strings.Join([]string{workspace, schedule, scope, period}, "\x1f"), nil
}

func newJobID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// SplitOccurrenceKey undoes OccurrenceKey.
func SplitOccurrenceKey(key string) (workspace, schedule, scope, period string, err error) {
	parts := strings.Split(key, "\x1f")
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("jobs: %q is not an occurrence key", key)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}
