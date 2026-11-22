package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"athena-erp/internal/store"
)

// Occurrence is one row of athena_schedule_occurrences: one period of one
// schedule in one scope, with its timezone, catch-up policy, lease,
// watermark and result. Period keys must sort chronologically (ISO-8601
// dates or datetimes); catch-up compares them as strings.
type Occurrence struct {
	Workspace   string
	Schedule    string
	Scope       string
	Period      string
	Timezone    string
	CatchUp     string
	Status      string
	LeaseOwner  string
	LeaseExpiry *time.Time
	Watermark   *time.Time
	Result      map[string]any
	JobID       string
}

// EnsureOptions describes a schedule occurrence to ensure.
type EnsureOptions struct {
	Workspace string
	Schedule  string
	Scope     string
	Period    string
	// Timezone names the schedule's location; empty means UTC. The
	// scheduler generates periods in this zone and the row keeps it as
	// evidence.
	Timezone string
	// CatchUp is CatchUpRun or CatchUpSkip; empty means CatchUpSkip.
	CatchUp string
}

// EnsureOccurrence stores the occurrence unless its Workspace, schedule,
// scope and period already have one. It reports whether it stored: two
// schedulers racing store exactly one row.
func EnsureOccurrence(ctx context.Context, db *sql.DB, classifier Classifier, opts EnsureOptions) (bool, error) {
	if opts.Workspace == "" || opts.Schedule == "" || opts.Period == "" {
		return false, fmt.Errorf("jobs: an occurrence needs a workspace, a schedule and a period")
	}
	timezone := opts.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return false, fmt.Errorf("jobs: unknown timezone %q", timezone)
	}
	catchUp := opts.CatchUp
	if catchUp == "" {
		catchUp = CatchUpSkip
	}
	if catchUp != CatchUpRun && catchUp != CatchUpSkip {
		return false, fmt.Errorf("jobs: unknown catch-up policy %q", opts.CatchUp)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO athena_schedule_occurrences
		(workspace, schedule_name, scope, period_key, timezone, catchup_policy, status)
		VALUES (?, ?, ?, ?, ?, ?, 'pending')`,
		opts.Workspace, opts.Schedule, opts.Scope, opts.Period, timezone, catchUp)
	if err == nil {
		return true, nil
	}
	if classifier.Classify(err) == store.ErrDuplicateKey {
		return false, nil
	}
	return false, fmt.Errorf("jobs: ensure occurrence: %w", err)
}

// GetOccurrence reads one occurrence by its key parts.
func GetOccurrence(ctx context.Context, q Querier, workspace, schedule, scope, period string) (Occurrence, error) {
	var occurrence Occurrence
	var leaseOwner sql.NullString
	var leaseExpiry, watermark sql.NullTime
	var result sql.NullString
	var jobID sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT workspace, schedule_name, scope, period_key, timezone,
		catchup_policy, status, lease_owner, lease_expires_at, watermark,
		CAST(result AS CHAR), job_id FROM athena_schedule_occurrences
		WHERE workspace = ? AND schedule_name = ? AND scope = ? AND period_key = ?`,
		workspace, schedule, scope, period).Scan(
		&occurrence.Workspace, &occurrence.Schedule, &occurrence.Scope, &occurrence.Period,
		&occurrence.Timezone, &occurrence.CatchUp, &occurrence.Status,
		&leaseOwner, &leaseExpiry, &watermark, &result, &jobID); err != nil {
		return Occurrence{}, fmt.Errorf("jobs: read occurrence: %w", err)
	}
	occurrence.LeaseOwner = leaseOwner.String
	if leaseExpiry.Valid {
		at := leaseExpiry.Time.UTC()
		occurrence.LeaseExpiry = &at
	}
	if watermark.Valid {
		at := watermark.Time.UTC()
		occurrence.Watermark = &at
	}
	if result.Valid {
		occurrence.Result = decodeMap(result.String)
	}
	occurrence.JobID = jobID.String
	return occurrence, nil
}

// DispatchOptions describes the job a dispatched occurrence runs.
type DispatchOptions struct {
	Workspace string
	Schedule  string
	Scope     string
	// Kind is the job kind to enqueue.
	Kind string
	// Payload is the job payload; the dispatcher adds the occurrence
	// identity under the "occurrence" key.
	Payload map[string]any
	// MaxAttempts caps the job's attempts; zero means the default.
	MaxAttempts int
	OwnerUserID string
	OwnerRole   string
	// LeaseOwner names the occurrence lease holder; empty means runner.
	LeaseOwner string
	// LeaseFor bounds the occurrence lease; empty means one minute.
	LeaseFor time.Duration
}

// DispatchDue leases the earliest pending occurrence of the schedule
// without a job and enqueues its job in one transaction, so an
// occurrence runs exactly one job. It reports the job id, or false when
// nothing is due. The job completion propagates back to the occurrence.
func DispatchDue(ctx context.Context, db *sql.DB, opts DispatchOptions) (string, bool, error) {
	if opts.Workspace == "" || opts.Schedule == "" || opts.Kind == "" {
		return "", false, fmt.Errorf("jobs: dispatch needs a workspace, a schedule and a job kind")
	}
	owner := opts.LeaseOwner
	if owner == "" {
		owner = "runner"
	}
	lease := opts.LeaseFor
	if lease == 0 {
		lease = time.Minute
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("jobs: dispatch: %w", err)
	}
	defer tx.Rollback()
	var period, timezone string
	if err := tx.QueryRowContext(ctx, `SELECT period_key, timezone FROM athena_schedule_occurrences
		WHERE workspace = ? AND schedule_name = ? AND scope = ? AND status = 'pending' AND job_id IS NULL
		ORDER BY period_key ASC LIMIT 1 FOR UPDATE`,
		opts.Workspace, opts.Schedule, opts.Scope).Scan(&period, &timezone); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("jobs: dispatch: %w", err)
	}
	key, err := OccurrenceKey(opts.Workspace, opts.Schedule, opts.Scope, period)
	if err != nil {
		return "", false, err
	}
	payload := nonNilMap(opts.Payload)
	payload["occurrence"] = map[string]any{
		"workspace": opts.Workspace, "schedule": opts.Schedule,
		"scope": opts.Scope, "period": period, "timezone": timezone,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	jobID, err := newJobID()
	if err != nil {
		return "", false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_jobs
		(id, kind, payload, status, attempts, max_attempts, fencing, occurrence_key, owner_user_id, owner_role)
		VALUES (?, ?, ?, 'queued', 0, ?, 0, ?, ?, ?)`,
		jobID, opts.Kind, string(encoded), maxAttempts, key,
		nullText(opts.OwnerUserID), nullText(opts.OwnerRole)); err != nil {
		return "", false, fmt.Errorf("jobs: dispatch: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_schedule_occurrences
		SET status = 'leased', lease_owner = ?, lease_expires_at = ?, job_id = ?
		WHERE workspace = ? AND schedule_name = ? AND scope = ? AND period_key = ?`,
		owner, now.Add(lease), jobID, opts.Workspace, opts.Schedule, opts.Scope, period); err != nil {
		return "", false, fmt.Errorf("jobs: dispatch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("jobs: dispatch: %w", err)
	}
	return jobID, true, nil
}

// completeOccurrence marks the job's occurrence done with its result and
// watermark. The caller commits with the job outcome.
func completeOccurrence(ctx context.Context, tx *sql.Tx, now time.Time, job Job, encodedResult string) error {
	workspace, schedule, scope, period, err := SplitOccurrenceKey(job.OccurrenceKey)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE athena_schedule_occurrences
		SET status = 'done', result = ?, watermark = ?
		WHERE workspace = ? AND schedule_name = ? AND scope = ? AND period_key = ? AND job_id = ?`,
		encodedResult, now, workspace, schedule, scope, period, job.ID)
	return err
}

// killOccurrence marks the job's occurrence dead with the error and
// watermark. The caller commits with the job outcome.
func killOccurrence(ctx context.Context, tx *sql.Tx, now time.Time, job Job, cause error) error {
	workspace, schedule, scope, period, err := SplitOccurrenceKey(job.OccurrenceKey)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(map[string]any{"error": cause.Error()})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE athena_schedule_occurrences
		SET status = 'dead', result = ?, watermark = ?
		WHERE workspace = ? AND schedule_name = ? AND scope = ? AND period_key = ? AND job_id = ?`,
		string(encoded), now, workspace, schedule, scope, period, job.ID)
	return err
}

// RecoverOccurrences heals occurrences after a restart: ones whose job
// finished propagate its outcome, and ones whose job is still active get
// a fresh lease. It reports done, dead and refreshed counts.
func RecoverOccurrences(ctx context.Context, db *sql.DB, owner string, leaseFor time.Duration) (done, dead, refreshed int64, err error) {
	if owner == "" {
		owner = "runner"
	}
	if leaseFor == 0 {
		leaseFor = time.Minute
	}
	now := time.Now().UTC()
	rows, err := db.QueryContext(ctx, `SELECT o.workspace, o.schedule_name, o.scope, o.period_key,
		o.job_id, j.status, CAST(j.result AS CHAR), j.last_error
		FROM athena_schedule_occurrences o JOIN athena_jobs j ON j.id = o.job_id
		WHERE o.status = 'leased'`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
	}
	type healing struct {
		workspace, schedule, scope, period, jobID, status string
		result                                            sql.NullString
		lastError                                         sql.NullString
	}
	var pending []healing
	for rows.Next() {
		var healing healing
		if err := rows.Scan(&healing.workspace, &healing.schedule, &healing.scope, &healing.period,
			&healing.jobID, &healing.status, &healing.result, &healing.lastError); err != nil {
			rows.Close()
			return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
		}
		pending = append(pending, healing)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
	}
	defer tx.Rollback()
	for _, healing := range pending {
		switch healing.status {
		case StatusSucceeded:
			if _, err := tx.ExecContext(ctx, `UPDATE athena_schedule_occurrences
				SET status = 'done', result = ?, watermark = ? WHERE job_id = ?`,
				healing.result.String, now, healing.jobID); err != nil {
				return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
			}
			done++
		case StatusDead:
			encoded, err := json.Marshal(map[string]any{"error": healing.lastError.String})
			if err != nil {
				return 0, 0, 0, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE athena_schedule_occurrences
				SET status = 'dead', result = ?, watermark = ? WHERE job_id = ?`,
				string(encoded), now, healing.jobID); err != nil {
				return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
			}
			dead++
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE athena_schedule_occurrences
				SET lease_owner = ?, lease_expires_at = ? WHERE job_id = ?`,
				owner, now.Add(leaseFor), healing.jobID); err != nil {
				return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
			}
			refreshed++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, fmt.Errorf("jobs: recover occurrences: %w", err)
	}
	return done, dead, refreshed, nil
}

// CatchUp skips pending occurrences of the schedule older than the
// current period, and reports how many it skipped. The run policy
// leaves them pending for the worker in period order. Period keys
// compare as strings, so they must be ISO-8601.
func CatchUp(ctx context.Context, db *sql.DB, workspace, schedule, scope, currentPeriod, policy string) (int64, error) {
	if policy != CatchUpRun && policy != CatchUpSkip {
		return 0, fmt.Errorf("jobs: unknown catch-up policy %q", policy)
	}
	if policy == CatchUpRun {
		return 0, nil
	}
	result, err := db.ExecContext(ctx, `UPDATE athena_schedule_occurrences
		SET status = 'skipped' WHERE workspace = ? AND schedule_name = ? AND scope = ?
		AND status = 'pending' AND job_id IS NULL AND period_key < ?`,
		workspace, schedule, scope, currentPeriod)
	if err != nil {
		return 0, fmt.Errorf("jobs: catch up: %w", err)
	}
	skipped, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("jobs: catch up: %w", err)
	}
	return skipped, nil
}
