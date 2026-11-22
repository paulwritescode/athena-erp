package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Handler runs one leased job inside its transaction and returns the
// result to store. A nil error finishes the job; any other error releases
// it for a retry with backoff. Handlers must be idempotent: a crash or an
// expired lease may run them twice, and fencing only guards the stored
// outcome, not the effect. A handler that changes a subject (a scheduled
// expiry, a state move) rechecks it under lock in its transaction before
// changing it.
type Handler func(ctx context.Context, tx *sql.Tx, job Job) (map[string]any, error)

// Runner claims due jobs and runs their handlers. DB and Errors are
// required. Jobs whose kind has no registered handler sit queued until a
// deploy registers one; the runner never drops work it cannot run.
type Runner struct {
	DB     *sql.DB
	Errors Classifier
	// Handlers runs one job kind each, by kind name.
	Handlers map[string]Handler
	// Owner names the lease holder in lease_owner; empty means runner.
	Owner string
	// LeaseFor bounds a claim; empty means one minute.
	LeaseFor time.Duration
	// MaxAttempts caps attempts per job when the row does not say
	// otherwise; zero means DefaultMaxAttempts.
	MaxAttempts int
	// Backoff waits between attempts; nil means DefaultBackoff.
	Backoff func(attempts int) time.Duration
	// DefaultOwnerUserID and DefaultOwnerRole take permanent failures
	// whose job names no owner. Empty leaves the owner to the operator.
	DefaultOwnerUserID string
	DefaultOwnerRole   string
	// Now is the clock; nil means time.Now in UTC.
	Now func() time.Time
}

// RunNext claims one due job of a known kind, runs its handler and stores
// the outcome. It reports whether it ran one. A fenced-out completion
// (the lease moved on while the handler ran) is not an error: the new
// lease owns the outcome now.
func (r *Runner) RunNext(ctx context.Context) (bool, error) {
	ctx, span := otel.Tracer("athena/jobs").Start(ctx, "athena.jobs.run_next")
	defer span.End()
	outcome := "none"
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	if len(r.Handlers) == 0 {
		return false, nil
	}
	job, ok, err := r.claim(ctx)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return false, err
	}
	if !ok {
		return false, nil
	}
	span.SetAttributes(attribute.String("athena.job_id", job.ID), attribute.String("athena.job_kind", job.Kind))
	handler := r.Handlers[job.Kind]
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return false, r.release(ctx, job, err)
	}
	result, handlerErr := handler(ctx, tx, job)
	if handlerErr != nil {
		_ = tx.Rollback()
		outcome = "retry"
		if r.exhausted(job) {
			outcome = "dead"
		}
		return true, r.fail(ctx, job, handlerErr)
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		outcome = "error"
		span.RecordError(err)
		return true, r.fail(ctx, job, err)
	}
	landed, err := r.finish(ctx, job, result)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return true, err
	}
	outcome = "succeeded"
	if !landed {
		outcome = "fenced"
	}
	return true, nil
}

// claim leases one due job of a known kind: queued and due first, then a
// job whose lease expired (a crash or a too-slow handler), which counts
// an attempt. The lease token selects the row back without racing a
// fellow runner. An expired lease already out of attempts parks dead
// instead of running again.
func (r *Runner) claim(ctx context.Context) (Job, bool, error) {
	now := r.now()
	owner := r.Owner
	if owner == "" {
		owner = "runner"
	}
	token, err := newToken(owner)
	if err != nil {
		return Job{}, false, err
	}
	kinds := make([]any, 0, len(r.Handlers))
	for kind := range r.Handlers {
		kinds = append(kinds, kind)
	}
	marks := make([]string, len(kinds))
	for i := range kinds {
		marks[i] = "?"
	}
	inClause := "(" + joinMarks(marks) + ")"
	lease := r.LeaseFor
	if lease == 0 {
		lease = time.Minute
	}
	args := append([]any{token, now.Add(lease)}, kinds...)
	args = append(args, now)
	result, err := r.DB.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'leased', lease_owner = ?, lease_expires_at = ?, fencing = fencing + 1
		WHERE kind IN `+inClause+` AND status = 'queued' AND (next_run_at IS NULL OR next_run_at <= ?)
		ORDER BY next_run_at IS NULL, next_run_at LIMIT 1`, args...)
	if err != nil {
		return Job{}, false, fmt.Errorf("jobs: claim queued: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Job{}, false, fmt.Errorf("jobs: claim queued: %w", err)
	} else if affected > 0 {
		return r.claimed(ctx, token)
	}
	// No queued job is due; reclaim one expired lease. The reclaim
	// counts an attempt, so a job that always crashes still dies.
	reclaimArgs := append([]any{token, now.Add(lease)}, kinds...)
	reclaimArgs = append(reclaimArgs, now)
	result, err = r.DB.ExecContext(ctx, `UPDATE athena_jobs
		SET lease_owner = ?, lease_expires_at = ?, fencing = fencing + 1, attempts = attempts + 1
		WHERE kind IN `+inClause+` AND status = 'leased' AND lease_expires_at < ?
		ORDER BY lease_expires_at LIMIT 1`, reclaimArgs...)
	if err != nil {
		return Job{}, false, fmt.Errorf("jobs: reclaim lease: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Job{}, false, fmt.Errorf("jobs: reclaim lease: %w", err)
	} else if affected == 0 {
		return Job{}, false, nil
	}
	job, _, err := r.claimed(ctx, token)
	if err != nil {
		return Job{}, false, err
	}
	if job.Attempts >= r.maxAttempts(job) {
		if err := r.parkDead(ctx, job, fmt.Errorf("jobs: lease expired on attempt %d", job.Attempts)); err != nil {
			return Job{}, false, err
		}
		return Job{}, false, nil
	}
	return job, true, nil
}

// claimed selects the leased row back by its token.
func (r *Runner) claimed(ctx context.Context, token string) (Job, bool, error) {
	job, err := scanJob(r.DB.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM athena_jobs WHERE lease_owner = ? AND status = 'leased'`, token))
	if err != nil {
		return Job{}, false, fmt.Errorf("jobs: read claimed job: %w", err)
	}
	return job, true, nil
}

// finish stores a success when the job still holds its fencing: the
// handler's result lands, a linked occurrence completes, and the lease
// clears. It reports false when a newer lease owns the job now.
func (r *Runner) finish(ctx context.Context, job Job, result map[string]any) (bool, error) {
	encoded, err := json.Marshal(nonNilMap(result))
	if err != nil {
		return false, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("jobs: finish: %w", err)
	}
	defer tx.Rollback()
	landed, err := r.finishTx(ctx, tx, job, string(encoded))
	if err != nil {
		return false, err
	}
	if !landed {
		return false, nil
	}
	if job.OccurrenceKey != "" {
		if err := completeOccurrence(ctx, tx, r.now(), job, string(encoded)); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("jobs: finish: %w", err)
	}
	return true, nil
}

func (r *Runner) finishTx(ctx context.Context, tx *sql.Tx, job Job, encoded string) (bool, error) {
	result, err := tx.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'succeeded', result = ?, lease_owner = NULL, lease_expires_at = NULL
		WHERE id = ? AND fencing = ?`, encoded, job.ID, job.Fencing)
	if err != nil {
		return false, fmt.Errorf("jobs: finish: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("jobs: finish: %w", err)
	}
	return affected > 0, nil
}

// fail counts one attempt: the job retries with backoff, or parks dead
// with its owner once attempts run out. A linked occurrence dies with it.
func (r *Runner) fail(ctx context.Context, job Job, cause error) error {
	if r.exhausted(job) {
		return r.parkDead(ctx, job, cause)
	}
	backoff := r.Backoff
	if backoff == nil {
		backoff = DefaultBackoff
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return cause
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'queued', attempts = attempts + 1, next_run_at = ?,
			last_error = ?, lease_owner = NULL, lease_expires_at = NULL
		WHERE id = ? AND fencing = ?`,
		r.now().Add(backoff(job.Attempts)), cause.Error(), job.ID, job.Fencing); err != nil {
		return cause
	}
	if err := tx.Commit(); err != nil {
		return cause
	}
	return cause
}

// parkDead assigns the exhausted job to its owner (its own, else the
// runner default) and records the error. A linked occurrence dies with it.
func (r *Runner) parkDead(ctx context.Context, job Job, cause error) error {
	ownerUser, ownerRole := job.OwnerUserID, job.OwnerRole
	if ownerUser == "" && ownerRole == "" {
		ownerUser, ownerRole = r.DefaultOwnerUserID, r.DefaultOwnerRole
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return cause
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'dead', attempts = attempts + 1, last_error = ?,
			owner_user_id = ?, owner_role = ?, lease_owner = NULL, lease_expires_at = NULL
		WHERE id = ?`, cause.Error(), nullText(ownerUser), nullText(ownerRole), job.ID); err != nil {
		return cause
	}
	if job.OccurrenceKey != "" {
		if err := killOccurrence(ctx, tx, r.now(), job, cause); err != nil {
			return cause
		}
	}
	if err := tx.Commit(); err != nil {
		return cause
	}
	return cause
}

// release drops a lease the runner claimed but could not run (its
// transaction would not begin). The job stays queued for the next tick.
func (r *Runner) release(ctx context.Context, job Job, cause error) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, last_error = ?
		WHERE id = ? AND fencing = ?`, cause.Error(), job.ID, job.Fencing)
	if err != nil {
		return fmt.Errorf("jobs: release lease: %w", err)
	}
	return cause
}

// Recover releases expired leases back to queued after a restart, so
// work in flight when the process stopped runs again. It reports how
// many jobs it released and how many it parked dead. Jobs already out
// of attempts park dead with their owner instead of running again; the
// threshold is row-first like maxAttempts, so a restart never parks a
// job the live reclaim path would still run.
func (r *Runner) Recover(ctx context.Context) (released, dead int64, err error) {
	now := r.now()
	maxAttempts := r.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	parked, err := r.DB.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'dead', last_error = 'lease expired after restart',
			owner_user_id = COALESCE(owner_user_id, ?), owner_role = COALESCE(owner_role, ?),
			lease_owner = NULL, lease_expires_at = NULL
		WHERE status = 'leased' AND lease_expires_at < ?
			AND attempts >= CASE WHEN max_attempts > 0 THEN max_attempts ELSE ? END`, nullText(r.DefaultOwnerUserID), nullText(r.DefaultOwnerRole), now, maxAttempts)
	if err != nil {
		return 0, 0, fmt.Errorf("jobs: recover dead: %w", err)
	}
	if dead, err = parked.RowsAffected(); err != nil {
		return 0, 0, fmt.Errorf("jobs: recover dead: %w", err)
	}
	freed, err := r.DB.ExecContext(ctx, `UPDATE athena_jobs
		SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL
		WHERE status = 'leased' AND lease_expires_at < ?`, now)
	if err != nil {
		return 0, 0, fmt.Errorf("jobs: recover queued: %w", err)
	}
	if released, err = freed.RowsAffected(); err != nil {
		return 0, 0, fmt.Errorf("jobs: recover queued: %w", err)
	}
	return released, dead, nil
}

// exhausted reports whether one more attempt would run past the cap.
func (r *Runner) exhausted(job Job) bool {
	return job.Attempts+1 >= r.maxAttempts(job)
}

func (r *Runner) maxAttempts(job Job) int {
	if job.MaxAttempts > 0 {
		return job.MaxAttempts
	}
	if r.MaxAttempts > 0 {
		return r.MaxAttempts
	}
	return DefaultMaxAttempts
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func newToken(owner string) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return owner + "/" + id.String(), nil
}

func joinMarks(marks []string) string {
	out := ""
	for i, mark := range marks {
		if i > 0 {
			out += ", "
		}
		out += mark
	}
	return out
}
