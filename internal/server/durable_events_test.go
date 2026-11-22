package server

import (
	"testing"

	"athena-erp/internal/lifecycle"
)

// TestSubmitAndCancelEmitDurableEvents submits and cancels a document with
// the bus hooks registered and checks each transition wrote its envelope
// and pending outbox row in the same transaction: the subject, revision,
// Workspace and correlation all land on the envelope.
func TestSubmitAndCancelEmitDurableEvents(t *testing.T) {
	recordTypes, records := newServices(t)
	lifecycleFixture(t, recordTypes)
	hooks := lifecycle.NewRegistry()
	registerDurableEvents(hooks)
	records.hooks = hooks
	records.workspace = "testws"

	order := newOrder(t, records, "Emit")
	submitted := submit(t, records, "order", order)
	checkEnvelope(t, records, "document.submitted", order.GetId(), submitted.GetRevision())
	cancelled, err := cancel(records, "order", submitted)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	checkEnvelope(t, records, "document.cancelled", order.GetId(), cancelled.GetRevision())
}

func checkEnvelope(t *testing.T, records recordService, eventType, recordID string, revision int64) {
	t.Helper()
	var id, producer, subjectType, subjectID, actor, correlation, outboxStatus string
	var subjectRevision int64
	var workspace string
	if err := records.db.QueryRow(`SELECT e.id, e.producer, e.subject_type, e.subject_id,
		e.subject_revision, e.actor, e.workspace, e.correlation_id, o.status
		FROM athena_events e JOIN athena_outbox o ON o.event_id = e.id
		WHERE e.event_type = ? AND e.subject_type = 'order' AND e.subject_id = ?`,
		eventType, recordID).Scan(&id, &producer, &subjectType, &subjectID,
		&subjectRevision, &actor, &workspace, &correlation, &outboxStatus); err != nil {
		t.Fatalf("%s envelope: %v", eventType, err)
	}
	if id == "" || producer != "lifecycle" || subjectType != "order" || subjectID != recordID ||
		subjectRevision != revision || actor == "" || workspace != "testws" ||
		correlation == "" || outboxStatus != "pending" {
		t.Fatalf("%s envelope mismatch: producer %q subject %q/%q rev %d actor %q workspace %q correlation %q outbox %q",
			eventType, producer, subjectType, subjectID, subjectRevision, actor, workspace, correlation, outboxStatus)
	}
}
