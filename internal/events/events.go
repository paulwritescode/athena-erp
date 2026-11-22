// Package events is the durable event bus of the kernel (batch-2 G8,
// ADR-0028, Proposed): producers write a business event envelope and an
// outbox row in their own transaction, a dispatcher delivers after the
// commit, and each consumer records its inbox row and local effect in one
// transaction, skipping repeats by consumer plus effect key. Packs declare
// their subscriptions; the tables live in system migration 17.
package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"athena-erp/internal/store"

	"github.com/google/uuid"
)

// Event types the kernel produces. Packs add their own under their name.
const (
	// TypePostingPosted is the posting engine's event per post.
	TypePostingPosted = "posting.posted"
	// TypeDocumentSubmitted fires when a document submits.
	TypeDocumentSubmitted = "document.submitted"
	// TypeDocumentCancelled fires when a document cancels.
	TypeDocumentCancelled = "document.cancelled"
)

// Outbox statuses of an athena_outbox row.
const (
	StatusPending    = "pending"
	StatusLeased     = "leased"
	StatusDispatched = "dispatched"
	StatusDead       = "dead"
)

// Inbox result of an athena_inbox row. The row exists only when the
// consumer's transaction committed, so claimed and applied are one state.
const ResultApplied = "applied"

// Subscription modes of an athena_event_subscriptions row.
const (
	ModeAsync = "async"
	ModeSync  = "sync"
)

// Exhaustion behaviour when a delivery runs out of attempts.
const (
	ExhaustDead = "dead"
	ExhaustSkip = "skip"
)

// Classifier is the error-classification half of store.Dialect, the only
// half the bus needs to tell a duplicate inbox claim from a failure.
type Classifier interface {
	Classify(error) store.ErrorKind
}

// Querier runs statements on a database or an open transaction.
type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Envelope is one durable business event: its immutable id, type and schema
// version, the Workspace and producer, the subject with its revision, the
// actor and Operation_ID, correlation and causation ids, the recorded
// sequence and a minimal payload of ids and counts. It never carries full
// Record values.
type Envelope struct {
	ID              string
	Sequence        int64
	EventType       string
	SchemaVersion   int
	Workspace       string
	Producer        string
	SubjectType     string
	SubjectID       string
	SubjectRevision int64
	Actor           string
	OperationID     string
	CorrelationID   string
	CausationID     string
	Payload         map[string]any
}

// Emit writes the envelope to athena_events and its outbox row to
// athena_outbox in the caller's transaction, so the event commits or rolls
// back with the business change. Nothing is delivered before the commit;
// the dispatcher picks the row up afterwards. It returns the event id.
func Emit(ctx context.Context, tx *sql.Tx, envelope Envelope) (string, error) {
	if envelope.EventType == "" {
		return "", fmt.Errorf("events: emit needs an event type")
	}
	if envelope.Producer == "" {
		return "", fmt.Errorf("events: emit needs a producer")
	}
	if envelope.SubjectType == "" || envelope.SubjectID == "" {
		return "", fmt.Errorf("events: emit needs a subject type and id")
	}
	id := envelope.ID
	if id == "" {
		v7, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		id = v7.String()
	}
	correlation := envelope.CorrelationID
	if correlation == "" {
		v7, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		correlation = v7.String()
	}
	version := envelope.SchemaVersion
	if version == 0 {
		version = 1
	}
	payload, err := json.Marshal(nonNilPayload(envelope.Payload))
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_events
		(id, event_type, schema_version, workspace, producer, subject_type, subject_id,
		 subject_revision, actor, operation_id, correlation_id, causation_id, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, envelope.EventType, version, envelope.Workspace, envelope.Producer,
		envelope.SubjectType, envelope.SubjectID, envelope.SubjectRevision,
		envelope.Actor, envelope.OperationID, correlation, nullText(envelope.CausationID),
		string(payload)); err != nil {
		return "", fmt.Errorf("events: insert event: %w", err)
	}
	outboxID, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO athena_outbox (id, event_id, event_type, payload, status) VALUES (?, ?, ?, ?, 'pending')`,
		outboxID.String(), id, envelope.EventType, string(payload)); err != nil {
		return "", fmt.Errorf("events: insert outbox: %w", err)
	}
	return id, nil
}

func nonNilPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	return payload
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Claim records the consumer's inbox row for the event and effect key in
// the caller's transaction, beside the consumer's local effect. It reports
// false when the consumer already recorded that pair, so a redelivered
// event applies its effect once. The caller commits or rolls the effect
// back with the claim; detail carries a small outcome note such as a
// created task id, or nothing.
func Claim(ctx context.Context, tx *sql.Tx, classifier Classifier, consumer, eventID, effectKey, detail string) (bool, error) {
	if consumer == "" || eventID == "" {
		return false, fmt.Errorf("events: claim needs a consumer and an event id")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO athena_inbox (consumer, event_id, effect_key, result, detail) VALUES (?, ?, ?, 'applied', ?)`,
		consumer, eventID, effectKey, nullText(detail))
	if err == nil {
		return true, nil
	}
	if classifier.Classify(err) == store.ErrDuplicateKey {
		return false, nil
	}
	return false, fmt.Errorf("events: claim inbox: %w", err)
}

// Claimed reports whether the consumer already recorded the event and
// effect key. Consumers use Claim in their effect transaction; this read
// serves replays and operator checks.
func Claimed(ctx context.Context, q Querier, consumer, eventID, effectKey string) (bool, error) {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM athena_inbox WHERE consumer = ? AND event_id = ? AND effect_key = ?`,
		consumer, eventID, effectKey).Scan(&count); err != nil {
		return false, fmt.Errorf("events: read inbox: %w", err)
	}
	return count > 0, nil
}

// Subscription is one consumer's declared interest in an event type: how
// to run (mode), what to run (action), the effect key its inbox claims
// carry, what to do when delivery exhausts its attempts, and the Pack
// that declared it, empty for kernel consumers.
type Subscription struct {
	Consumer        string
	EventType       string
	Mode            string
	Action          string
	EffectKey       string
	ExhaustBehavior string
	Pack            string
	Enabled         bool
}

// Declare stores a subscription, replacing the consumer's previous row for
// the event type. Pack installs call it; kernel consumers call it at
// startup. Enabled is written as given: pass true to receive events.
func Declare(ctx context.Context, q Querier, classifier Classifier, sub Subscription) error {
	if sub.Consumer == "" || sub.EventType == "" || sub.Action == "" {
		return fmt.Errorf("events: a subscription needs a consumer, an event type and an action")
	}
	mode := sub.Mode
	if mode == "" {
		mode = ModeAsync
	}
	exhaust := sub.ExhaustBehavior
	if exhaust == "" {
		exhaust = ExhaustDead
	}
	_, err := q.ExecContext(ctx, `INSERT INTO athena_event_subscriptions
		(consumer, event_type, mode, action, effect_key, exhaust_behavior, pack, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.Consumer, sub.EventType, mode, sub.Action, sub.EffectKey, exhaust,
		nullText(sub.Pack), sub.Enabled)
	if err == nil {
		return nil
	}
	if classifier.Classify(err) != store.ErrDuplicateKey {
		return fmt.Errorf("events: declare subscription: %w", err)
	}
	_, err = q.ExecContext(ctx, `UPDATE athena_event_subscriptions
		SET mode = ?, action = ?, effect_key = ?, exhaust_behavior = ?, pack = ?, enabled = ?
		WHERE consumer = ? AND event_type = ?`,
		mode, sub.Action, sub.EffectKey, exhaust, nullText(sub.Pack), sub.Enabled,
		sub.Consumer, sub.EventType)
	if err != nil {
		return fmt.Errorf("events: replace subscription: %w", err)
	}
	return nil
}

// Match lists the enabled subscriptions for an event type, in consumer
// order, for the dispatcher fan-out.
func Match(ctx context.Context, q Querier, eventType string) ([]Subscription, error) {
	rows, err := q.QueryContext(ctx, `SELECT consumer, event_type, mode, action, effect_key, exhaust_behavior, pack, enabled
		FROM athena_event_subscriptions WHERE event_type = ? AND enabled = 1 ORDER BY consumer`, eventType)
	if err != nil {
		return nil, fmt.Errorf("events: match subscriptions: %w", err)
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		var pack sql.NullString
		var enabled int
		if err := rows.Scan(&sub.Consumer, &sub.EventType, &sub.Mode, &sub.Action,
			&sub.EffectKey, &sub.ExhaustBehavior, &pack, &enabled); err != nil {
			return nil, fmt.Errorf("events: scan subscription: %w", err)
		}
		sub.Pack = pack.String
		sub.Enabled = enabled != 0
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: read subscriptions: %w", err)
	}
	return out, nil
}
