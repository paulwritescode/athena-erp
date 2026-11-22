package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Handler applies one event for one consumer inside the consumer's
// transaction: it runs its local effect and returns, and the dispatcher
// commits the effect with the inbox claim. A nil error commits; any other
// error rolls back and schedules a retry.
type Handler func(ctx context.Context, tx *sql.Tx, event Envelope) error

// Dispatcher delivers due outbox rows to subscribed consumers. It leases
// one row at a time, fans out to every enabled subscription with a
// registered handler, and marks the row dispatched, retries it with
// backoff, or parks it dead. A subscription without a registered handler
// is skipped: retrying would never help, because handlers are code, not
// state. DB and Errors are required.
type Dispatcher struct {
	DB     *sql.DB
	Errors Classifier
	// Handlers runs one consumer each, by consumer name.
	Handlers map[string]Handler
	// Owner names the lease holder in leased_by; empty means dispatcher.
	Owner string
	// LeaseFor bounds a claim; empty means one minute.
	LeaseFor time.Duration
	// MaxAttempts caps deliveries per row; zero means five.
	MaxAttempts int
	// Backoff waits between attempts; nil means exponential seconds capped
	// at five minutes.
	Backoff func(attempts int) time.Duration
	// Now is the clock; nil means time.Now in UTC.
	Now func() time.Time
}

// DispatchNext delivers one due outbox row and reports whether it
// delivered one. A crash between the business commit and the dispatch
// leaves the row pending, so the next call recovers it.
func (d *Dispatcher) DispatchNext(ctx context.Context) (bool, error) {
	ctx, span := otel.Tracer("athena/events").Start(ctx, "athena.events.dispatch_next")
	defer span.End()
	outcome := "none"
	delivered := false
	defer func() { span.SetAttributes(attribute.String("athena.outcome", outcome)) }()
	row, ok, err := d.claim(ctx)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return false, err
	}
	if !ok {
		return false, nil
	}
	span.SetAttributes(attribute.String("athena.event_id", row.eventID), attribute.String("athena.event_type", row.eventType))
	event, err := d.load(ctx, row)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return false, d.fail(ctx, row, err)
	}
	subs, err := Match(ctx, d.DB, event.EventType)
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		return false, d.fail(ctx, row, err)
	}
	span.SetAttributes(attribute.Int("athena.subscriptions", len(subs)))
	for _, sub := range subs {
		handler, registered := d.Handlers[sub.Consumer]
		if !registered {
			continue
		}
		applied, err := d.deliver(ctx, sub, event, handler)
		if err != nil {
			outcome = "error"
			span.RecordError(err)
			return false, d.fail(ctx, row, err)
		}
		if applied {
			delivered = true
		}
	}
	if err := d.finish(ctx, row); err != nil {
		outcome = "error"
		span.RecordError(err)
		return false, err
	}
	outcome = "dispatched"
	if !delivered {
		outcome = "dispatched_noop"
	}
	return true, nil
}

// outboxRow is a leased athena_outbox row. Rows written before migration
// 17 carry no event id; the dispatcher still delivers their payload.
type outboxRow struct {
	id        string
	eventID   string
	legacy    bool
	eventType string
	payload   string
	attempts  int
}

// claim leases one due outbox row: pending and due, or leased past its
// expiry. The lease token selects the row back without racing a fellow
// dispatcher.
func (d *Dispatcher) claim(ctx context.Context) (outboxRow, bool, error) {
	now := d.now()
	owner := d.Owner
	if owner == "" {
		owner = "dispatcher"
	}
	token, err := newToken(owner)
	if err != nil {
		return outboxRow{}, false, err
	}
	lease := d.LeaseFor
	if lease == 0 {
		lease = time.Minute
	}
	// Retried rows sort ahead of fresh pending rows; every due row is
	// eventually claimed, so the order only favors finishing work.
	result, err := d.DB.ExecContext(ctx, `UPDATE athena_outbox
		SET status = 'leased', leased_by = ?, leased_until = ?
		WHERE (status = 'pending' AND (available_at IS NULL OR available_at <= ?))
		   OR (status = 'leased' AND leased_until < ?)
		ORDER BY available_at IS NULL, available_at LIMIT 1`,
		token, now.Add(lease), now, now)
	if err != nil {
		return outboxRow{}, false, fmt.Errorf("events: claim outbox: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return outboxRow{}, false, fmt.Errorf("events: claim outbox: %w", err)
	}
	if affected == 0 {
		return outboxRow{}, false, nil
	}
	var row outboxRow
	var eventID sql.NullString
	if err := d.DB.QueryRowContext(ctx, `SELECT id, event_id, event_type, CAST(payload AS CHAR), attempts
		FROM athena_outbox WHERE leased_by = ? AND status = 'leased'`, token).
		Scan(&row.id, &eventID, &row.eventType, &row.payload, &row.attempts); err != nil {
		return outboxRow{}, false, fmt.Errorf("events: read claimed outbox: %w", err)
	}
	row.eventID = eventID.String
	row.legacy = !eventID.Valid
	if row.legacy {
		row.eventID = row.id
	}
	return row, true, nil
}

// load reads the event envelope. Legacy outbox rows have no envelope, so
// the dispatcher delivers their stored type and payload under the outbox
// row id.
func (d *Dispatcher) load(ctx context.Context, row outboxRow) (Envelope, error) {
	if row.legacy {
		return Envelope{ID: row.id, EventType: row.eventType, SchemaVersion: 1,
			Payload: decodePayload(row.payload)}, nil
	}
	var event Envelope
	var payload string
	var causation sql.NullString
	if err := d.DB.QueryRowContext(ctx, `SELECT id, sequence, event_type, schema_version, workspace,
		producer, subject_type, subject_id, subject_revision, actor, operation_id,
		correlation_id, causation_id, CAST(payload AS CHAR)
		FROM athena_events WHERE id = ?`, row.eventID).Scan(
		&event.ID, &event.Sequence, &event.EventType, &event.SchemaVersion,
		&event.Workspace, &event.Producer, &event.SubjectType, &event.SubjectID,
		&event.SubjectRevision, &event.Actor, &event.OperationID,
		&event.CorrelationID, &causation, &payload); err != nil {
		return Envelope{}, fmt.Errorf("events: read event: %w", err)
	}
	event.CausationID = causation.String
	event.Payload = decodePayload(payload)
	return event, nil
}

func decodePayload(raw string) map[string]any {
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// deliver runs one consumer's handler with its inbox claim in a single
// transaction. It reports whether the handler ran: false means the inbox
// already held the pair and the redelivery was skipped.
func (d *Dispatcher) deliver(ctx context.Context, sub Subscription, event Envelope, handler Handler) (bool, error) {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("events: begin delivery: %w", err)
	}
	defer tx.Rollback()
	claimed, err := Claim(ctx, tx, d.Errors, sub.Consumer, event.ID, sub.EffectKey, "")
	if err != nil {
		return false, err
	}
	if !claimed {
		_ = tx.Rollback()
		return false, nil
	}
	if err := handler(ctx, tx, event); err != nil {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("athena.failed_consumer", sub.Consumer))
		return false, fmt.Errorf("events: consumer %s: %w", sub.Consumer, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("events: commit delivery: %w", err)
	}
	return true, nil
}

// fail counts one attempt: the row retries with backoff, or parks dead
// with the error once attempts run out. Exhaustion is row-level: a dead
// row keeps its last error for the operator, and per-subscription
// exhaust behavior is not honored yet (one row cannot express one
// outcome per consumer).
func (d *Dispatcher) fail(ctx context.Context, row outboxRow, cause error) error {
	max := d.MaxAttempts
	if max == 0 {
		max = 5
	}
	backoff := d.Backoff
	if backoff == nil {
		backoff = defaultBackoff
	}
	attempts := row.attempts + 1
	if attempts >= max {
		_, err := d.DB.ExecContext(ctx, `UPDATE athena_outbox
			SET status = 'dead', attempts = ?, last_error = ?, leased_by = NULL, leased_until = NULL
			WHERE id = ?`, attempts, cause.Error(), row.id)
		if err != nil {
			return fmt.Errorf("events: park outbox dead: %w", err)
		}
		return cause
	}
	_, err := d.DB.ExecContext(ctx, `UPDATE athena_outbox
		SET status = 'pending', attempts = ?, available_at = ?, last_error = ?, leased_by = NULL, leased_until = NULL
		WHERE id = ?`, attempts, d.now().Add(backoff(row.attempts)), cause.Error(), row.id)
	if err != nil {
		return fmt.Errorf("events: retry outbox: %w", err)
	}
	return cause
}

// finish marks a fully delivered row dispatched.
func (d *Dispatcher) finish(ctx context.Context, row outboxRow) error {
	if _, err := d.DB.ExecContext(ctx, `UPDATE athena_outbox
		SET status = 'dispatched', dispatched_at = ?, leased_by = NULL, leased_until = NULL
		WHERE id = ?`, d.now(), row.id); err != nil {
		return fmt.Errorf("events: finish outbox: %w", err)
	}
	return nil
}

// defaultBackoff waits 2^attempts seconds, capped at five minutes.
func defaultBackoff(attempts int) time.Duration {
	wait := time.Second << min(attempts, 8)
	if wait > 5*time.Minute {
		return 5 * time.Minute
	}
	return wait
}

func (d *Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
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
