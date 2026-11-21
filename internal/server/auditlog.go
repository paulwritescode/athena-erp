package server

import (
	"context"
	"database/sql"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
)

// writeAudit appends one Audit Log row in the caller's transaction, so the
// row exists exactly when the change commits. The request ID and source come
// from ctx.
func writeAudit(ctx context.Context, tx *sql.Tx, recordTypeName, recordID string, revision int64, action string, changes []audit.Change) error {
	requestID, source := requestFrom(ctx)
	_, err := audit.Append(ctx, tx, dialect, audit.Entry{
		RecordType: recordTypeName, RecordID: recordID, Revision: revision, Action: action,
		Changes: changes, Actor: actorName(ctx), Source: source, RequestID: requestID,
	})
	if err != nil {
		return internalError("write audit log", err)
	}
	return nil
}

// valueText renders a stored value for the Audit Log. An empty value is nil.
func valueText(value *athenav1.FieldValue) *string {
	switch v := value.GetKind().(type) {
	case *athenav1.FieldValue_StringValue:
		return &v.StringValue
	case *athenav1.FieldValue_BoolValue:
		text := "false"
		if v.BoolValue {
			text = "true"
		}
		return &text
	case *athenav1.FieldValue_ImageValue:
		path := v.ImageValue.GetPath()
		return &path
	}
	return nil
}

// fieldChanges lists the keys whose value differs between before and after.
// A nil before means the Record is new, so every key with a value counts.
func fieldChanges(keys []string, before, after *athenav1.Record) []audit.Change {
	changes := []audit.Change{}
	for _, key := range keys {
		change := audit.Change{Field: key, New: valueText(after.GetValues()[key])}
		if before != nil {
			change.Old = valueText(before.GetValues()[key])
		}
		changes = append(changes, change)
	}
	return changes
}

// auditCreated records a new Record at revision 1 with the values it holds.
func auditCreated(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id string, keys []string) error {
	created, err := loadRecord(ctx, tx, recordType, id, "")
	if err != nil {
		return err
	}
	return writeAudit(ctx, tx, recordType.Name, id, 1, "created", fieldChanges(keys, nil, created))
}

func ptr(s string) *string { return &s }
