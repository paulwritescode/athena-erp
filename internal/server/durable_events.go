package server

import (
	"context"

	"athena-erp/internal/events"
	"athena-erp/internal/lifecycle"
)

// registerDurableEvents attaches the event bus to the in-transaction submit
// and cancel hooks, so every transition writes its envelope and outbox row
// in the same transaction as the state change. The dispatcher delivers
// after the commit; a hook error rolls the transition back.
func registerDurableEvents(hooks *lifecycle.Registry) {
	hooks.On(lifecycle.OnSubmit, "", emitDocumentEvent(events.TypeDocumentSubmitted))
	hooks.On(lifecycle.OnCancel, "", emitDocumentEvent(events.TypeDocumentCancelled))
}

func emitDocumentEvent(eventType string) lifecycle.Hook {
	return func(ctx context.Context, e *lifecycle.Event) error {
		if e.Tx == nil {
			return nil
		}
		_, err := events.Emit(ctx, e.Tx, events.Envelope{
			EventType:       eventType,
			Workspace:       e.Workspace,
			Producer:        "lifecycle",
			SubjectType:     e.RecordType,
			SubjectID:       e.RecordID,
			SubjectRevision: e.Revision,
			Actor:           e.Actor,
			CorrelationID:   e.RequestID,
			Payload:         map[string]any{"from": e.From.String(), "to": e.To.String()},
		})
		return err
	}
}
