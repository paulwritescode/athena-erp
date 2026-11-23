package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
	"athena-erp/internal/events"
	"athena-erp/internal/jobs"

	"connectrpc.com/connect/v2"
)

// eventService serves EventService: reads over the durable event bus
// (ADR-0028, Proposed). jobService serves JobService: reads over the durable
// job runner and retries of dead work. Both need the build system permission,
// the same gate as the Jobs page; the CLI acts as the operator.
type eventService struct{ db *sql.DB }

type jobService struct{ db *sql.DB }

// NewEventService returns the EventService handler over db. The CLI uses it
// directly; Serve mounts it behind the permission gate.
func NewEventService(db *sql.DB) athenav1connect.EventServiceHandler {
	return eventService{db: db}
}

// NewJobService returns the JobService handler over db. The CLI uses it
// directly; Serve mounts it behind the permission gate.
func NewJobService(db *sql.DB) athenav1connect.JobServiceHandler {
	return jobService{db: db}
}

func (s eventService) ready() error {
	if s.db == nil {
		return connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	return nil
}

func (s jobService) ready() error {
	if s.db == nil {
		return connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	return nil
}

// queuePageSize bounds the Jobs and Events lists: 30 rows unless asked, never
// more than 100.
func queuePageSize(want int32) int {
	size := int(want)
	if size <= 0 {
		size = 30
	}
	return min(size, 100)
}

func formatAt(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}

func formatNullAt(at sql.NullTime) string {
	if !at.Valid {
		return ""
	}
	return formatAt(at.Time)
}

// ListEvents returns committed events, newest first, with keyset paging over
// the event id. Ids are UUIDv7, so id order is creation order.
func (s eventService) ListEvents(ctx context.Context, req *athenav1.ListEventsRequest) (*athenav1.ListEventsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	size := queuePageSize(req.GetPageSize())
	where, args := "1 = 1", []any{}
	if req.GetEventType() != "" {
		where += " AND event_type = ?"
		args = append(args, req.GetEventType())
	}
	if req.GetSubjectType() != "" {
		where += " AND subject_type = ?"
		args = append(args, req.GetSubjectType())
	}
	if req.GetSubjectId() != "" {
		where += " AND subject_id = ?"
		args = append(args, req.GetSubjectId())
	}
	if req.GetPageToken() != "" {
		after, err := decodeCursor(req.GetPageToken(), "events")
		if err != nil {
			return nil, err
		}
		where += " AND id < ?"
		args = append(args, after)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, sequence, event_type, schema_version, workspace,
		producer, subject_type, subject_id, subject_revision, actor, operation_id,
		correlation_id, causation_id, CAST(payload AS CHAR), created_at
		FROM athena_events WHERE `+where+` ORDER BY id DESC LIMIT ?`, append(args, size+1)...)
	if err != nil {
		return nil, internalError("read events", err)
	}
	defer rows.Close()
	res := &athenav1.ListEventsResponse{}
	for rows.Next() {
		var event athenav1.Event
		var causation sql.NullString
		var at time.Time
		if err := rows.Scan(&event.Id, &event.Sequence, &event.EventType, &event.SchemaVersion,
			&event.Workspace, &event.Producer, &event.SubjectType, &event.SubjectId,
			&event.SubjectRevision, &event.Actor, &event.OperationId, &event.CorrelationId,
			&causation, &event.Payload, &at); err != nil {
			return nil, internalError("read events", err)
		}
		event.CausationId = causation.String
		event.CreatedAt = formatAt(at)
		res.Events = append(res.Events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read events", err)
	}
	if len(res.GetEvents()) > size {
		res.NextPageToken = encodeCursor("events", res.GetEvents()[size-1].GetId())
		res.Events = res.GetEvents()[:size]
	}
	return res, nil
}

// GetEvent returns one event envelope with its minimal payload.
func (s eventService) GetEvent(ctx context.Context, req *athenav1.GetEventRequest) (*athenav1.GetEventResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	var event athenav1.Event
	var causation sql.NullString
	var at time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT id, sequence, event_type, schema_version, workspace,
		producer, subject_type, subject_id, subject_revision, actor, operation_id,
		correlation_id, causation_id, CAST(payload AS CHAR), created_at
		FROM athena_events WHERE id = ?`, req.GetId()).Scan(
		&event.Id, &event.Sequence, &event.EventType, &event.SchemaVersion,
		&event.Workspace, &event.Producer, &event.SubjectType, &event.SubjectId,
		&event.SubjectRevision, &event.Actor, &event.OperationId, &event.CorrelationId,
		&causation, &event.Payload, &at); err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, "no event with that id")
		}
		return nil, internalError("read the event", err)
	}
	event.CausationId = causation.String
	event.CreatedAt = formatAt(at)
	return &athenav1.GetEventResponse{Event: &event}, nil
}

// ListOutbox returns outbox delivery rows, newest first, with keyset paging
// over the row id.
func (s eventService) ListOutbox(ctx context.Context, req *athenav1.ListOutboxRequest) (*athenav1.ListOutboxResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	switch status := req.GetStatus(); status {
	case "", events.StatusPending, events.StatusLeased, events.StatusDispatched, events.StatusDead:
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, "status is one of pending, leased, dispatched or dead")
	}
	size := queuePageSize(req.GetPageSize())
	where, args := "1 = 1", []any{}
	if req.GetStatus() != "" {
		where += " AND status = ?"
		args = append(args, req.GetStatus())
	}
	if req.GetPageToken() != "" {
		after, err := decodeCursor(req.GetPageToken(), "outbox")
		if err != nil {
			return nil, err
		}
		where += " AND id < ?"
		args = append(args, after)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, event_type, CAST(payload AS CHAR),
		status, attempts, available_at, leased_by, leased_until, last_error,
		dispatched_at, created_at FROM athena_outbox WHERE `+where+` ORDER BY id DESC LIMIT ?`,
		append(args, size+1)...)
	if err != nil {
		return nil, internalError("read the outbox", err)
	}
	defer rows.Close()
	res := &athenav1.ListOutboxResponse{}
	for rows.Next() {
		var row athenav1.OutboxRow
		var eventID, leasedBy, lastError sql.NullString
		var available, leasedUntil, dispatched sql.NullTime
		var at time.Time
		if err := rows.Scan(&row.Id, &eventID, &row.EventType, &row.Payload, &row.Status,
			&row.Attempts, &available, &leasedBy, &leasedUntil, &lastError, &dispatched, &at); err != nil {
			return nil, internalError("read the outbox", err)
		}
		row.EventId, row.LeasedBy, row.LastError = eventID.String, leasedBy.String, lastError.String
		row.AvailableAt, row.LeasedUntil, row.DispatchedAt =
			formatNullAt(available), formatNullAt(leasedUntil), formatNullAt(dispatched)
		row.CreatedAt = formatAt(at)
		res.Rows = append(res.Rows, &row)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read the outbox", err)
	}
	if len(res.GetRows()) > size {
		res.NextPageToken = encodeCursor("outbox", res.GetRows()[size-1].GetId())
		res.Rows = res.GetRows()[:size]
	}
	return res, nil
}

// RequeueOutbox tries a dead delivery again by emitting a fresh envelope
// through the event bus: same type, subject and payload, a new id, the same
// correlation id and the dead event as the cause. The dead row stays as
// history. Rows without an envelope predate the bus and cannot be requeued.
func (s eventService) RequeueOutbox(ctx context.Context, req *athenav1.RequeueOutboxRequest) (_ *athenav1.RequeueOutboxResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "requeue_outbox", "")
	defer func() { end(err) }()
	if err = s.ready(); err != nil {
		return nil, err
	}
	if _, err = requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	var row struct {
		id, status string
		eventID    sql.NullString
	}
	if err = s.db.QueryRowContext(ctx, `SELECT id, status, event_id FROM athena_outbox WHERE id = ?`,
		req.GetId()).Scan(&row.id, &row.status, &row.eventID); err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, "no outbox row with that id")
		}
		return nil, internalError("read the outbox row", err)
	}
	if row.status != events.StatusDead {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "only a dead delivery can be requeued")
	}
	if !row.eventID.Valid {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "this delivery predates envelopes and cannot be requeued")
	}
	var envelope events.Envelope
	var rawPayload string
	var causation sql.NullString
	if err = s.db.QueryRowContext(ctx, `SELECT event_type, schema_version, workspace, producer,
		subject_type, subject_id, subject_revision, actor, operation_id, correlation_id,
		causation_id, CAST(payload AS CHAR) FROM athena_events WHERE id = ?`,
		row.eventID.String).Scan(&envelope.EventType, &envelope.SchemaVersion, &envelope.Workspace,
		&envelope.Producer, &envelope.SubjectType, &envelope.SubjectID, &envelope.SubjectRevision,
		&envelope.Actor, &envelope.OperationID, &envelope.CorrelationID, &causation, &rawPayload); err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeFailedPrecondition, "this delivery has no envelope and cannot be requeued")
		}
		return nil, internalError("read the dead event", err)
	}
	envelope.CausationID = row.eventID.String
	if err = json.Unmarshal([]byte(rawPayload), &envelope.Payload); err != nil {
		return nil, internalError("read the dead event", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("requeue the delivery", err)
	}
	defer tx.Rollback()
	fresh, err := events.Emit(ctx, tx, envelope)
	if err != nil {
		return nil, internalError("requeue the delivery", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, internalError("requeue the delivery", err)
	}
	return &athenav1.RequeueOutboxResponse{EventId: fresh}, nil
}

// ListSubscriptions returns the declared consumer subscriptions in consumer
// order. The table is small, so the list is unpaged.
func (s eventService) ListSubscriptions(ctx context.Context, req *athenav1.ListSubscriptionsRequest) (*athenav1.ListSubscriptionsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	query := `SELECT consumer, event_type, mode, action, effect_key, exhaust_behavior, pack, enabled
		FROM athena_event_subscriptions`
	var args []any
	if req.GetEventType() != "" {
		query += ` WHERE event_type = ?`
		args = append(args, req.GetEventType())
	}
	query += ` ORDER BY consumer, event_type`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, internalError("read the subscriptions", err)
	}
	defer rows.Close()
	res := &athenav1.ListSubscriptionsResponse{}
	for rows.Next() {
		var sub athenav1.Subscription
		var pack sql.NullString
		var enabled int
		if err := rows.Scan(&sub.Consumer, &sub.EventType, &sub.Mode, &sub.Action,
			&sub.EffectKey, &sub.ExhaustBehavior, &pack, &enabled); err != nil {
			return nil, internalError("read the subscriptions", err)
		}
		sub.Pack = pack.String
		sub.Enabled = enabled != 0
		res.Subscriptions = append(res.Subscriptions, &sub)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read the subscriptions", err)
	}
	return res, nil
}

// ListJobs returns jobs, newest first, with keyset paging over the job id.
// The list carries no payload or result; GetJob returns them.
func (s jobService) ListJobs(ctx context.Context, req *athenav1.ListJobsRequest) (*athenav1.ListJobsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	switch status := req.GetStatus(); status {
	case "", jobs.StatusQueued, jobs.StatusLeased, jobs.StatusSucceeded, jobs.StatusDead:
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, "status is one of queued, leased, succeeded or dead")
	}
	size := queuePageSize(req.GetPageSize())
	where, args := "1 = 1", []any{}
	if req.GetStatus() != "" {
		where += " AND status = ?"
		args = append(args, req.GetStatus())
	}
	if req.GetKind() != "" {
		where += " AND kind = ?"
		args = append(args, req.GetKind())
	}
	if req.GetPageToken() != "" {
		after, err := decodeCursor(req.GetPageToken(), "jobs")
		if err != nil {
			return nil, err
		}
		where += " AND id < ?"
		args = append(args, after)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, status, attempts, max_attempts,
		next_run_at, lease_owner, lease_expires_at, fencing, occurrence_key,
		owner_user_id, owner_role, last_error, created_at, updated_at
		FROM athena_jobs WHERE `+where+` ORDER BY id DESC LIMIT ?`, append(args, size+1)...)
	if err != nil {
		return nil, internalError("read jobs", err)
	}
	defer rows.Close()
	res := &athenav1.ListJobsResponse{}
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, internalError("read jobs", err)
		}
		res.Jobs = append(res.Jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read jobs", err)
	}
	if len(res.GetJobs()) > size {
		res.NextPageToken = encodeCursor("jobs", res.GetJobs()[size-1].GetId())
		res.Jobs = res.GetJobs()[:size]
	}
	return res, nil
}

// scanJobRow reads the ListJobs columns, in order, without payload or result.
func scanJobRow(rows *sql.Rows) (*athenav1.Job, error) {
	var job athenav1.Job
	var nextRun, leaseExpiry sql.NullTime
	var leaseOwner, occurrenceKey, ownerUser, ownerRole, lastError sql.NullString
	var created, updated time.Time
	if err := rows.Scan(&job.Id, &job.Kind, &job.Status, &job.Attempts, &job.MaxAttempts,
		&nextRun, &leaseOwner, &leaseExpiry, &job.Fencing, &occurrenceKey,
		&ownerUser, &ownerRole, &lastError, &created, &updated); err != nil {
		return nil, err
	}
	job.NextRunAt, job.LeaseExpiresAt = formatNullAt(nextRun), formatNullAt(leaseExpiry)
	job.LeaseOwner, job.OccurrenceKey, job.OwnerUserId, job.OwnerRole, job.LastError =
		leaseOwner.String, occurrenceKey.String, ownerUser.String, ownerRole.String, lastError.String
	job.CreatedAt, job.UpdatedAt = formatAt(created), formatAt(updated)
	return &job, nil
}

// GetJob returns one job with its payload and result.
func (s jobService) GetJob(ctx context.Context, req *athenav1.GetJobRequest) (*athenav1.GetJobResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	var job athenav1.Job
	var nextRun, leaseExpiry sql.NullTime
	var leaseOwner, occurrenceKey, ownerUser, ownerRole, lastError sql.NullString
	var result sql.NullString
	var created, updated time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT id, kind, status, attempts, max_attempts,
		next_run_at, lease_owner, lease_expires_at, fencing, occurrence_key,
		owner_user_id, owner_role, last_error, CAST(payload AS CHAR), CAST(result AS CHAR),
		created_at, updated_at FROM athena_jobs WHERE id = ?`, req.GetId()).Scan(
		&job.Id, &job.Kind, &job.Status, &job.Attempts, &job.MaxAttempts,
		&nextRun, &leaseOwner, &leaseExpiry, &job.Fencing, &occurrenceKey,
		&ownerUser, &ownerRole, &lastError, &job.Payload, &result, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, "no job with that id")
		}
		return nil, internalError("read the job", err)
	}
	job.NextRunAt, job.LeaseExpiresAt = formatNullAt(nextRun), formatNullAt(leaseExpiry)
	job.LeaseOwner, job.OccurrenceKey, job.OwnerUserId, job.OwnerRole, job.LastError =
		leaseOwner.String, occurrenceKey.String, ownerUser.String, ownerRole.String, lastError.String
	job.Result = result.String
	job.CreatedAt, job.UpdatedAt = formatAt(created), formatAt(updated)
	return &athenav1.GetJobResponse{Job: &job}, nil
}

// RetryJob runs dead work again by enqueueing a new job with the same kind,
// payload and owner through the runner, so leases, fencing and backoff apply
// as for any job. The dead row stays as history, and a linked schedule
// occurrence stays dead: the new job runs as ad-hoc work.
func (s jobService) RetryJob(ctx context.Context, req *athenav1.RetryJobRequest) (_ *athenav1.RetryJobResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "retry_job", "")
	defer func() { end(err) }()
	if err = s.ready(); err != nil {
		return nil, err
	}
	if _, err = requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	var dead struct {
		id, kind, status, payload string
		maxAttempts               int
		ownerUser, ownerRole      sql.NullString
	}
	if err = s.db.QueryRowContext(ctx, `SELECT id, kind, status, CAST(payload AS CHAR),
		max_attempts, owner_user_id, owner_role FROM athena_jobs WHERE id = ?`,
		req.GetId()).Scan(&dead.id, &dead.kind, &dead.status, &dead.payload,
		&dead.maxAttempts, &dead.ownerUser, &dead.ownerRole); err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, "no job with that id")
		}
		return nil, internalError("read the dead job", err)
	}
	if dead.status != jobs.StatusDead {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "only a dead job can be retried")
	}
	payload := map[string]any{}
	if err = json.Unmarshal([]byte(dead.payload), &payload); err != nil {
		return nil, internalError("read the dead job", err)
	}
	fresh, err := jobs.Enqueue(ctx, s.db, jobs.EnqueueOptions{
		Kind: dead.kind, Payload: payload, MaxAttempts: dead.maxAttempts,
		OwnerUserID: dead.ownerUser.String, OwnerRole: dead.ownerRole.String,
	})
	if err != nil {
		return nil, internalError("retry the job", err)
	}
	return &athenav1.RetryJobResponse{JobId: fresh}, nil
}

// ListOccurrences returns schedule occurrences in period order. The list is
// bounded and carries no page token: schedules hold few periods.
func (s jobService) ListOccurrences(ctx context.Context, req *athenav1.ListOccurrencesRequest) (*athenav1.ListOccurrencesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	switch status := req.GetStatus(); status {
	case "", jobs.OccurrencePending, jobs.OccurrenceLeased, jobs.OccurrenceDone, jobs.OccurrenceSkipped, jobs.OccurrenceDead:
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, "status is one of pending, leased, done, skipped or dead")
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, 500)
	where, args := "1 = 1", []any{}
	if req.GetSchedule() != "" {
		where += " AND schedule_name = ?"
		args = append(args, req.GetSchedule())
	}
	if req.GetScope() != "" {
		where += " AND scope = ?"
		args = append(args, req.GetScope())
	}
	if req.GetStatus() != "" {
		where += " AND status = ?"
		args = append(args, req.GetStatus())
	}
	rows, err := s.db.QueryContext(ctx, `SELECT workspace, schedule_name, scope, period_key,
		timezone, catchup_policy, status, lease_owner, lease_expires_at, watermark,
		CAST(result AS CHAR), job_id FROM athena_schedule_occurrences
		WHERE `+where+` ORDER BY period_key ASC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, internalError("read occurrences", err)
	}
	defer rows.Close()
	res := &athenav1.ListOccurrencesResponse{}
	for rows.Next() {
		var occurrence athenav1.Occurrence
		var leaseOwner sql.NullString
		var leaseExpiry, watermark sql.NullTime
		var result, jobID sql.NullString
		if err := rows.Scan(&occurrence.Workspace, &occurrence.Schedule, &occurrence.Scope,
			&occurrence.Period, &occurrence.Timezone, &occurrence.CatchupPolicy, &occurrence.Status,
			&leaseOwner, &leaseExpiry, &watermark, &result, &jobID); err != nil {
			return nil, internalError("read occurrences", err)
		}
		occurrence.LeaseOwner = leaseOwner.String
		occurrence.LeaseExpiresAt, occurrence.Watermark = formatNullAt(leaseExpiry), formatNullAt(watermark)
		occurrence.Result, occurrence.JobId = result.String, jobID.String
		res.Occurrences = append(res.Occurrences, &occurrence)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read occurrences", err)
	}
	return res, nil
}
