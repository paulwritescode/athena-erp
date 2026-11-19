package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/store"

	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Document lifecycle (spec 5, ADR-0011). Submittable documents carry a
// docstate column that only the kernel changes. Submit, cancel and amend each
// run in one transaction that also writes the Document Version snapshot, the
// Activity Timeline row and the Record history, so a rejected hook leaves
// nothing behind.

// lifecycleColumns are the extra system columns of a submittable document.
func lifecycleColumns(d recordTypeDefinition) []store.Column {
	if !d.Submittable {
		return nil
	}
	name, _ := dialect.ColumnType("Text")
	reference, _ := dialect.ColumnType("Link")
	columns := []store.Column{{Name: "docstate", Type: dialect.BigIntType()}, {Name: "name", Type: name}}
	if d.Amendable {
		columns = append(columns, store.Column{Name: "amended_from", Type: reference})
	}
	return columns
}

// systemColumns lists every kernel column of the Record Type's table beyond
// id, revision, created_at and updated_at.
func (d recordTypeDefinition) systemColumns() []store.Column {
	return append(systemColumns(d.kind()), lifecycleColumns(d)...)
}

// documentIndexName names the unique index on a submittable document's name.
func documentIndexName(recordTypeName string) string {
	return fieldIndexName("uq", recordTypeName, "name")
}

// ensureLifecycle gives rows that predate the lifecycle a draft state and a
// name, and makes names unique. It is safe to repeat.
func ensureLifecycle(ctx context.Context, conn sqlConn, d recordTypeDefinition) error {
	if !d.Submittable {
		return nil
	}
	table := dialect.QuoteIdent(recordTypeTableName(d.Name))
	for _, statement := range []string{
		"UPDATE " + table + " SET `docstate` = 0 WHERE `docstate` IS NULL",
		"UPDATE " + table + " SET `name` = `id` WHERE `name` IS NULL",
		dialect.AddIndex(recordTypeTableName(d.Name), documentIndexName(d.Name), "name", true),
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// validateLifecycle checks the lifecycle flags of a definition against the
// saved revision.
func validateLifecycle(ctx context.Context, conn sqlConn, d recordTypeDefinition, previous *recordTypeDefinition) error {
	if (d.Submittable || d.Amendable) && d.kind() != kindDocument {
		return invalid("only documents can be submittable or amendable")
	}
	if d.Submittable && d.Class == "work" {
		return invalid("work documents have no submit step")
	}
	if d.Amendable && !d.Submittable {
		return invalid("a document must be submittable to be amendable")
	}
	if err := validateNaming(d); err != nil {
		return err
	}
	for _, field := range d.Fields {
		if d.Submittable && field.Name == "name" {
			return invalid(`the field key "name" is reserved on submittable documents; the kernel names each document`)
		}
		if field.AllowOnSubmit && !(d.kind() == kindDocument && d.Submittable) && d.kind() != kindChild {
			return invalid(fmt.Sprintf("%s: allow on submit needs a submittable document or a child Record Type", field.Label))
		}
	}
	if previous == nil {
		return nil
	}
	table := dialect.QuoteIdent(recordTypeTableName(d.Name))
	if previous.Submittable && !d.Submittable {
		var used bool
		if err := conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE `docstate` <> 0)").Scan(&used); err != nil {
			return internalError("check documents", err)
		}
		if used {
			return invalid("some documents are already submitted or cancelled, so the document cannot stop being submittable")
		}
	}
	if previous.Amendable && !d.Amendable {
		var used bool
		if err := conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE `amended_from` IS NOT NULL)").Scan(&used); err != nil {
			return internalError("check documents", err)
		}
		if used {
			return invalid("some documents are already amended, so the document cannot stop being amendable")
		}
	}
	return nil
}

func hookError(name lifecycle.Name, err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return err
	}
	return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s rejected the change: %v", name, err))
}

// fire runs the in-transaction hooks of one event.
func (s recordService) fire(ctx context.Context, name lifecycle.Name, event lifecycle.Event) error {
	event.Name = name
	if err := s.hooks.Fire(ctx, event); err != nil {
		return hookError(name, err)
	}
	return nil
}

// fireAfter runs the events that follow the commit. They cannot undo it, so
// a failure is logged with ids only.
func (s recordService) fireAfter(ctx context.Context, steps []lifecycle.Step, event lifecycle.Event) {
	event.Tx = nil
	for _, step := range steps {
		if step.InTx {
			continue
		}
		event.Name = step.Name
		if err := s.hooks.Fire(ctx, event); err != nil {
			log.Printf("lifecycle: %s hook for %s %s failed: %v", step.Name, event.RecordType, event.RecordID, err)
		}
	}
}

// fireSteps fires the in-transaction steps of a sequence in order, from index
// from up to but excluding index to.
func (s recordService) fireSteps(ctx context.Context, steps []lifecycle.Step, event lifecycle.Event, from, to int) error {
	for _, step := range steps[from:min(to, len(steps))] {
		if !step.InTx {
			continue
		}
		if err := s.fire(ctx, step.Name, event); err != nil {
			return err
		}
	}
	return nil
}

func (s recordService) submittable(ctx context.Context, key string) (recordTypeDefinition, error) {
	recordType, err := s.recordType(ctx, key)
	if err != nil {
		return recordType, err
	}
	if !recordType.Submittable {
		return recordType, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s is not a submittable document", recordType.Label))
	}
	return recordType, nil
}

func (s recordService) SubmitRecord(ctx context.Context, req *athenav1.SubmitRecordRequest) (res *athenav1.SubmitRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "submit_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	record, err := s.transition(ctx, requestID, lifecycle.Submit, req.GetRecordTypeKey(), req.GetId(), req.GetExpectedRevision())
	if err != nil {
		return nil, err
	}
	return &athenav1.SubmitRecordResponse{Record: record}, nil
}

func (s recordService) CancelRecord(ctx context.Context, req *athenav1.CancelRecordRequest) (res *athenav1.CancelRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "cancel_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	record, err := s.transition(ctx, requestID, lifecycle.Cancel, req.GetRecordTypeKey(), req.GetId(), req.GetExpectedRevision())
	if err != nil {
		return nil, err
	}
	return &athenav1.CancelRecordResponse{Record: record}, nil
}

// transition runs a submit or a cancel. The first in-transaction event fires
// before the state changes, the others after it, and the events after the
// commit last (lifecycle.Sequence).
func (s recordService) transition(ctx context.Context, requestID string, action lifecycle.Action, key, id string, expected int64) (*athenav1.Record, error) {
	recordType, err := s.submittable(ctx, key)
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError(string(action)+" record", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := authorizeRecord(ctx, tx, p, recordType, id, string(action)); err != nil {
		return nil, err
	}
	current, err := loadRecord(ctx, tx, recordType, id, dialect.ForUpdate())
	if err != nil {
		return nil, err
	}
	if err := checkSeparateDuties(p, recordType, string(action), current); err != nil {
		return nil, err
	}
	if current.GetRevision() != expected {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this record; reload it and try again")
	}
	from := lifecycle.State(current.GetDocstate())
	to, err := lifecycle.Next(from, action)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err.Error())
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("athena.state_from", from.String()), attribute.String("athena.state_to", to.String()))
	if action == lifecycle.Submit {
		if err := requireFilled(recordType, current); err != nil {
			return nil, err
		}
	}
	if action == lifecycle.Cancel {
		if label, err := liveDownstream(ctx, tx, recordType, id); err != nil {
			return nil, err
		} else if label != "" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("cannot cancel: a submitted %s links to this document; cancel that first", label))
		}
	}
	event := lifecycle.Event{RecordType: key, RecordID: id, RequestID: requestID, From: from, To: to, Tx: tx}
	steps := lifecycle.Sequence(action)
	if err := s.fireSteps(ctx, steps, event, 0, 1); err != nil {
		return nil, err
	}
	statement := fmt.Sprintf("UPDATE %s SET `docstate` = ?, `revision` = `revision` + 1, `modified_by` = ?, `def_version` = ?, `request_id` = ? WHERE `id` = ? AND `revision` = ?", dialect.QuoteIdent(recordTypeTableName(key)))
	if _, err := tx.ExecContext(ctx, statement, int32(to), actorName(ctx), recordType.Revision, requestID, id, expected); err != nil {
		return nil, writeError(string(action)+" record", err)
	}
	changes := []audit.Change{{Field: "docstate", Old: ptr(fmt.Sprint(int32(from))), New: ptr(fmt.Sprint(int32(to)))}}
	if action == lifecycle.Submit {
		name, err := s.assignSeriesName(ctx, tx, recordType, current)
		if err != nil {
			return nil, err
		}
		if name != "" {
			changes = append(changes, audit.Change{Field: "name", Old: ptr(current.GetName()), New: &name})
		}
	}
	pastEvent := map[lifecycle.Action]string{lifecycle.Submit: "submitted", lifecycle.Cancel: "cancelled"}[action]
	if err := writeAudit(ctx, tx, key, id, expected+1, pastEvent, changes); err != nil {
		return nil, err
	}
	updated, err := loadRecord(ctx, tx, recordType, id, "")
	if err != nil {
		return nil, err
	}
	if err := writeVersion(ctx, tx, recordType, updated, string(action), requestID); err != nil {
		return nil, err
	}
	if err := writeActivity(ctx, tx, activityRow{recordType: key, recordID: id, event: pastEvent, requestID: requestID, from: &from, to: &to}); err != nil {
		return nil, err
	}
	if err := s.fireSteps(ctx, steps, event, 1, len(steps)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError(string(action)+" record", err)
	}
	s.fireAfter(ctx, steps, event)
	record, err := s.load(ctx, recordType, id)
	if err != nil {
		return nil, err
	}
	maskRecord(hiddenFields(p, recordType), record)
	return record, nil
}

func (s recordService) AmendRecord(ctx context.Context, req *athenav1.AmendRecordRequest) (res *athenav1.AmendRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "amend_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.submittable(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	if !recordType.Amendable {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s is not amendable", recordType.Label))
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("amend record", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := authorizeRecord(ctx, tx, p, recordType, req.GetId(), auth.ActionAmend); err != nil {
		return nil, err
	}
	original, err := loadRecord(ctx, tx, recordType, req.GetId(), dialect.ForUpdate())
	if err != nil {
		return nil, err
	}
	if original.GetRevision() != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this record; reload it and try again")
	}
	from := lifecycle.State(original.GetDocstate())
	to, err := lifecycle.Next(from, lifecycle.Amend)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err.Error())
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("athena.state_from", from.String()), attribute.String("athena.state_to", to.String()))
	table := dialect.QuoteIdent(recordTypeTableName(recordType.Name))
	var amended bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE `amended_from` = ?)", req.GetId()).Scan(&amended); err != nil {
		return nil, internalError("check amendments", err)
	}
	if amended {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "this document was already amended")
	}
	columns, args, err := bindValues(forAmend(recordType), copiedValues(recordType, original), true)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "cannot amend: "+connectMessage(err))
	}
	newName, err := amendedName(ctx, tx, recordType, original)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, internalError("generate record ID", err)
	}
	system := map[string]any{"created_by": actorName(ctx), "modified_by": actorName(ctx), "def_version": recordType.Revision, "request_id": requestID, "owner": actorName(ctx), "docstate": int32(lifecycle.Draft), "name": newName, "amended_from": req.GetId()}
	if err := insertRow(ctx, tx, recordType, id.String(), system, columns, args); err != nil {
		return nil, err
	}
	if err := auditCreated(ctx, tx, recordType, id.String(), changedKeys(columns, args)); err != nil {
		return nil, err
	}
	if err := copyChildRecords(ctx, tx, recordType, original, id.String(), requestID); err != nil {
		return nil, err
	}
	successor, err := loadRecord(ctx, tx, recordType, id.String(), "")
	if err != nil {
		return nil, err
	}
	event := lifecycle.Event{RecordType: recordType.Name, RecordID: id.String(), RelatedID: req.GetId(), RequestID: requestID, From: from, To: to, Tx: tx}
	steps := lifecycle.Sequence(lifecycle.Amend)
	if err := s.fireSteps(ctx, steps, event, 0, len(steps)); err != nil {
		return nil, err
	}
	if err := writeVersion(ctx, tx, recordType, successor, "amend", requestID); err != nil {
		return nil, err
	}
	for _, row := range []activityRow{
		{recordType: recordType.Name, recordID: req.GetId(), event: "amended", requestID: requestID, related: id.String()},
		{recordType: recordType.Name, recordID: id.String(), event: "amended_from", requestID: requestID, related: req.GetId()},
	} {
		if err := writeActivity(ctx, tx, row); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("amend record", err)
	}
	s.fireAfter(ctx, steps, event)
	record, err := s.load(ctx, recordType, id.String())
	if err != nil {
		return nil, err
	}
	maskRecord(hiddenFields(p, recordType), record)
	return &athenav1.AmendRecordResponse{Record: record}, nil
}

// copiedValues returns the values an amended successor starts with. Unique,
// read-only and no_copy fields stay empty: a unique value is still held by the
// cancelled original, and a read-only field takes only its default.
func copiedValues(recordType recordTypeDefinition, record *athenav1.Record) map[string]*athenav1.FieldValue {
	values := map[string]*athenav1.FieldValue{}
	for _, field := range recordType.Fields {
		value := record.GetValues()[field.Name]
		if field.NoCopy || field.Unique || field.ReadOnly || value == nil || value.GetNullValue() || value.GetKind() == nil {
			continue
		}
		values[field.Name] = value
	}
	return values
}

// forAmend returns the definition an amended draft is bound against. A field
// that copiedValues leaves empty (unique, read only or no_copy) is not required
// yet: the draft is a working copy and the person fills it in. Submit refuses
// the document while a required field is still empty (requireFilled).
func forAmend(def recordTypeDefinition) recordTypeDefinition {
	fields := make([]recordTypeField, len(def.Fields))
	copy(fields, def.Fields)
	for i, field := range fields {
		if field.NoCopy || field.Unique || field.ReadOnly {
			fields[i].Required = false
		}
	}
	def.Fields = fields
	return def
}

// requireFilled refuses a submit while a required field is empty. Formula
// fields are checked when they are calculated.
func requireFilled(def recordTypeDefinition, record *athenav1.Record) error {
	for _, field := range def.Fields {
		if !field.Required || field.Formula != "" {
			continue
		}
		if value := record.GetValues()[field.Name]; value == nil || value.GetNullValue() || value.GetKind() == nil {
			return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("cannot submit: %s is required", field.Label))
		}
	}
	return nil
}

// amendedName returns the root document's name with the next suffix, so the
// chain reads X, X-1, X-2. It walks amended_from to the root.
func amendedName(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, original *athenav1.Record) (string, error) {
	query := "SELECT `name`, `amended_from` FROM " + dialect.QuoteIdent(recordTypeTableName(recordType.Name)) + " WHERE `id` = ?"
	name, parent, hops := original.GetName(), original.GetAmendedFrom(), 1
	for parent != "" {
		var next sql.NullString
		var nextName sql.NullString
		if err := tx.QueryRowContext(ctx, query, parent).Scan(&nextName, &next); err != nil {
			return "", internalError("name amended record", err)
		}
		name, parent, hops = nextName.String, next.String, hops+1
		if hops > 10000 {
			return "", internalError("name amended record", errors.New("amendment chain does not end"))
		}
	}
	return fmt.Sprintf("%s-%d", name, hops), nil
}

// copyChildRecords copies the original's child Records to the successor,
// following the same rules as copiedValues.
func copyChildRecords(ctx context.Context, tx *sql.Tx, parent recordTypeDefinition, original *athenav1.Record, newParentID, requestID string) error {
	children, err := childRecordTypes(ctx, tx, parent.Name)
	if err != nil {
		return err
	}
	for _, child := range children {
		rows, err := listChildren(ctx, tx, child, original.GetId())
		if err != nil {
			return err
		}
		for _, row := range rows {
			columns, args, err := bindValues(forAmend(child), copiedValues(child, row), true)
			if err != nil {
				return connect.NewError(connect.CodeFailedPrecondition, "cannot amend: "+connectMessage(err))
			}
			id, err := uuid.NewV7()
			if err != nil {
				return internalError("generate record ID", err)
			}
			system := map[string]any{"created_by": actorName(ctx), "modified_by": actorName(ctx), "def_version": child.Revision, "request_id": requestID, "parent_id": newParentID, "idx": row.GetIdx()}
			if err := insertRow(ctx, tx, child, id.String(), system, columns, args); err != nil {
				return err
			}
			if err := auditCreated(ctx, tx, child, id.String(), changedKeys(columns, args)); err != nil {
				return err
			}
		}
	}
	return nil
}

// changedKeys names the columns that received a value.
func changedKeys(columns []string, args []any) []string {
	var keys []string
	for i, column := range columns {
		if args[i] != nil {
			keys = append(keys, strings.Trim(column, "`"))
		}
	}
	return keys
}

// allRecordTypes loads every saved Record Type definition.
func allRecordTypes(ctx context.Context, q querier) ([]recordTypeDefinition, error) {
	rows, err := q.QueryContext(ctx, `SELECT definition, revision FROM athena_record_types ORDER BY name`)
	if err != nil {
		return nil, internalError("read Record Types", err)
	}
	defer rows.Close()
	var all []recordTypeDefinition
	for rows.Next() {
		var payload []byte
		var d recordTypeDefinition
		if err := rows.Scan(&payload, &d.Revision); err != nil {
			return nil, internalError("read Record Types", err)
		}
		revision := d.Revision
		if err := json.Unmarshal(payload, &d); err != nil {
			return nil, internalError("decode Record Type", err)
		}
		d.Revision = revision
		all = append(all, d)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read Record Types", err)
	}
	return all, nil
}

// childRecordTypes returns the child Record Types whose Records belong to
// parentKey.
func childRecordTypes(ctx context.Context, q querier, parentKey string) ([]recordTypeDefinition, error) {
	all, err := allRecordTypes(ctx, q)
	if err != nil {
		return nil, err
	}
	var children []recordTypeDefinition
	for _, d := range all {
		if d.ParentRecordType == parentKey {
			children = append(children, d)
		}
	}
	return children, nil
}

// listChildren reads the child Records of one parent in position order.
func listChildren(ctx context.Context, q querier, child recordTypeDefinition, parentID string) ([]*athenav1.Record, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE `parent_id` = ? ORDER BY `idx`", selectColumns(child), dialect.QuoteIdent(recordTypeTableName(child.Name))), parentID)
	if err != nil {
		return nil, internalError("read child records", err)
	}
	defer rows.Close()
	var records []*athenav1.Record
	for rows.Next() {
		record, err := scanRecord(rows, child)
		if err != nil {
			return nil, internalError("read child records", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read child records", err)
	}
	return records, nil
}

// liveDownstream returns the label of a Record Type that holds a submitted
// document linking to the document, or "" when none does (spec 5.3). A Link
// field on a child counts through its parent document.
func liveDownstream(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id string) (string, error) {
	all, err := allRecordTypes(ctx, tx)
	if err != nil {
		return "", err
	}
	byName := map[string]recordTypeDefinition{}
	for _, d := range all {
		byName[d.Name] = d
	}
	for _, d := range all {
		for _, field := range d.Fields {
			if field.Type != "Link" || field.TargetRecordType != recordType.Name {
				continue
			}
			var query string
			table, column := dialect.QuoteIdent(recordTypeTableName(d.Name)), dialect.QuoteIdent(field.Name)
			label := d.Label
			switch {
			case d.kind() == kindDocument && d.Submittable:
				query = "SELECT EXISTS(SELECT 1 FROM " + table + " WHERE " + column + " = ? AND `docstate` = 1 AND `id` <> ?)"
			case d.kind() == kindChild && byName[d.ParentRecordType].Submittable:
				owner := byName[d.ParentRecordType]
				ownerTable := dialect.QuoteIdent(recordTypeTableName(owner.Name))
				query = "SELECT EXISTS(SELECT 1 FROM " + table + " c JOIN " + ownerTable + " p ON p.`id` = c.`parent_id` WHERE c." + column + " = ? AND p.`docstate` = 1 AND p.`id` <> ?)"
				label = owner.Label
			default:
				continue
			}
			var found bool
			if err := tx.QueryRowContext(ctx, query, id, id).Scan(&found); err != nil {
				return "", internalError("check linked documents", err)
			}
			if found {
				return label, nil
			}
		}
	}
	return "", nil
}

// checkParentEditable keeps the lines of a submitted or cancelled document
// fixed. It locks the parent row so a concurrent submit cannot interleave. A
// submitted document's child Records may change only in fields marked
// allow_on_submit; none may be added or removed.
func checkParentEditable(ctx context.Context, tx *sql.Tx, child recordTypeDefinition, parentID, change string, keys []string) error {
	parent, _, err := loadRecordType(ctx, tx, child.ParentRecordType)
	if err != nil || parent == nil {
		return internalError("load parent Record Type", fmt.Errorf("parent %q: %v", child.ParentRecordType, err))
	}
	if !parent.Submittable {
		return nil
	}
	var state sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT `docstate` FROM "+dialect.QuoteIdent(recordTypeTableName(parent.Name))+" WHERE `id` = ? "+dialect.ForUpdate(), parentID).Scan(&state); err != nil {
		return internalError("check parent", err)
	}
	switch lifecycle.State(state.Int64) {
	case lifecycle.Draft:
		return nil
	case lifecycle.Submitted:
		if change == "update" {
			fields := fieldsByKey(child)
			for _, key := range keys {
				if !fields[key].AllowOnSubmit {
					return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s cannot be changed after the %s is submitted", fields[key].Label, parent.Label))
				}
			}
			return nil
		}
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("lines cannot be added or removed after the %s is submitted", parent.Label))
	}
	return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("the %s is cancelled and cannot be changed", parent.Label))
}

// snapshotValue converts a stored value to its JSON form.
func snapshotValue(value *athenav1.FieldValue) any {
	switch v := value.GetKind().(type) {
	case *athenav1.FieldValue_StringValue:
		return v.StringValue
	case *athenav1.FieldValue_BoolValue:
		return v.BoolValue
	}
	return nil
}

func snapshotValues(values map[string]*athenav1.FieldValue) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = snapshotValue(value)
	}
	return out
}

// buildSnapshot returns the document with its child Records as plain JSON data.
func buildSnapshot(ctx context.Context, q querier, recordType recordTypeDefinition, record *athenav1.Record) (map[string]any, error) {
	snapshot := map[string]any{"name": record.GetName(), "docstate": record.GetDocstate(), "revision": record.GetRevision(), "amended_from": record.GetAmendedFrom(), "values": snapshotValues(record.GetValues())}
	children, err := childRecordTypes(ctx, q, recordType.Name)
	if err != nil {
		return nil, err
	}
	lines := map[string]any{}
	for _, child := range children {
		rows, err := listChildren(ctx, q, child, record.GetId())
		if err != nil {
			return nil, err
		}
		items := make([]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, map[string]any{"id": row.GetId(), "idx": row.GetIdx(), "values": snapshotValues(row.GetValues())})
		}
		lines[child.Name] = items
	}
	snapshot["children"] = lines
	return snapshot, nil
}

// diffSnapshots lists what changed between two snapshots: field values, the
// state and each child Record Type's lines.
func diffSnapshots(previous, current map[string]any) map[string]any {
	diff := map[string]any{}
	compare := func(key string, from, to any) {
		if !reflect.DeepEqual(from, to) {
			diff[key] = map[string]any{"from": from, "to": to}
		}
	}
	compare("docstate", previous["docstate"], current["docstate"])
	oldValues, _ := previous["values"].(map[string]any)
	newValues, _ := current["values"].(map[string]any)
	for key, value := range newValues {
		compare(key, oldValues[key], value)
	}
	for key, value := range oldValues {
		if _, ok := newValues[key]; !ok {
			compare(key, value, nil)
		}
	}
	oldChildren, _ := previous["children"].(map[string]any)
	newChildren, _ := current["children"].(map[string]any)
	for key, value := range newChildren {
		compare("children."+key, oldChildren[key], value)
	}
	return diff
}

// writeVersion stores a Document Version: the snapshot and its diff from the
// previous version. The caller holds the document's row lock, so numbering
// cannot race.
func writeVersion(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, record *athenav1.Record, event, requestID string) error {
	snapshot, err := buildSnapshot(ctx, tx, recordType, record)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return internalError("write version", err)
	}
	var version int64
	var previousPayload []byte
	switch err := tx.QueryRowContext(ctx, "SELECT `version`, `snapshot` FROM athena_document_versions WHERE record_type_name = ? AND record_id = ? ORDER BY `version` DESC LIMIT 1", recordType.Name, record.GetId()).Scan(&version, &previousPayload); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return internalError("write version", err)
	}
	diff := map[string]any{}
	if previousPayload != nil {
		var previous, current map[string]any
		if err := json.Unmarshal(previousPayload, &previous); err != nil {
			return internalError("write version", err)
		}
		// Round trip so both sides have the same JSON types.
		if err := json.Unmarshal(payload, &current); err != nil {
			return internalError("write version", err)
		}
		diff = diffSnapshots(previous, current)
	}
	diffPayload, err := json.Marshal(diff)
	if err != nil {
		return internalError("write version", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return internalError("write version", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO athena_document_versions (id, record_type_name, record_id, `version`, revision, event, docstate, snapshot, diff, actor, request_id) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		id.String(), recordType.Name, record.GetId(), version+1, record.GetRevision(), event, record.GetDocstate(), payload, diffPayload, actorName(ctx), requestID); err != nil {
		return internalError("write version", err)
	}
	return nil
}

// activityRow is one Activity Timeline entry.
type activityRow struct {
	recordType, recordID, event, requestID, related string
	from, to                                        *lifecycle.State
	changed                                         []string
}

func writeActivity(ctx context.Context, tx *sql.Tx, row activityRow) error {
	id, err := uuid.NewV7()
	if err != nil {
		return internalError("write activity", err)
	}
	if row.changed == nil {
		row.changed = []string{}
	}
	changed, err := json.Marshal(row.changed)
	if err != nil {
		return internalError("write activity", err)
	}
	var from, to, related any
	if row.from != nil {
		from = int32(*row.from)
	}
	if row.to != nil {
		to = int32(*row.to)
	}
	if row.related != "" {
		related = row.related
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO athena_activity (id, record_type_name, record_id, event, actor, from_state, to_state, changed_fields, related_id, request_id) VALUES (?,?,?,?,?,?,?,?,?,?)",
		id.String(), row.recordType, row.recordID, row.event, actorName(ctx), from, to, changed, related, row.requestID); err != nil {
		return internalError("write activity", err)
	}
	return nil
}

func (s recordService) ListRecordVersions(ctx context.Context, req *athenav1.ListRecordVersionsRequest) (*athenav1.ListRecordVersionsResponse, error) {
	recordType, err := s.submittable(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	if err := authorizeRecord(ctx, s.db, p, recordType, req.GetId(), auth.ActionAudit); err != nil {
		return nil, err
	}
	hidden, childHidden, err := hiddenWithChildren(ctx, s.db, p, recordType)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT `version`, revision, event, docstate, snapshot, diff, actor, created_at FROM athena_document_versions WHERE record_type_name = ? AND record_id = ? ORDER BY `version` DESC LIMIT 200", recordType.Name, req.GetId())
	if err != nil {
		return nil, internalError("read versions", err)
	}
	defer rows.Close()
	res := &athenav1.ListRecordVersionsResponse{}
	for rows.Next() {
		var item athenav1.RecordVersion
		var snapshot, diff []byte
		var at time.Time
		if err := rows.Scan(&item.Version, &item.Revision, &item.Event, &item.Docstate, &snapshot, &diff, &item.Actor, &at); err != nil {
			return nil, internalError("read versions", err)
		}
		if len(hidden) > 0 || len(childHidden) > 0 {
			if snapshot, diff, err = maskVersion(snapshot, diff, hidden, childHidden); err != nil {
				return nil, internalError("read versions", err)
			}
		}
		item.Snapshot, item.Diff, item.At = string(snapshot), string(diff), at.UTC().Format(time.RFC3339)
		res.Versions = append(res.Versions, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read versions", err)
	}
	return res, nil
}

func (s recordService) ListRecordActivity(ctx context.Context, req *athenav1.ListRecordActivityRequest) (*athenav1.ListRecordActivityResponse, error) {
	recordType, err := s.submittable(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	if err := authorizeRecord(ctx, s.db, p, recordType, req.GetId(), auth.ActionAudit); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT event, actor, from_state, to_state, changed_fields, related_id, created_at FROM athena_activity WHERE record_type_name = ? AND record_id = ? ORDER BY created_at DESC, id DESC LIMIT 200", recordType.Name, req.GetId())
	if err != nil {
		return nil, internalError("read activity", err)
	}
	defer rows.Close()
	res := &athenav1.ListRecordActivityResponse{}
	for rows.Next() {
		var item athenav1.ActivityEvent
		var from, to sql.NullInt64
		var related sql.NullString
		var changed []byte
		var at time.Time
		if err := rows.Scan(&item.Event, &item.Actor, &from, &to, &changed, &related, &at); err != nil {
			return nil, internalError("read activity", err)
		}
		if err := json.Unmarshal(changed, &item.ChangedFields); err != nil {
			return nil, internalError("read activity", err)
		}
		item.FromState, item.ToState, item.RelatedId, item.At = int32(from.Int64), int32(to.Int64), related.String, at.UTC().Format(time.RFC3339)
		res.Events = append(res.Events, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read activity", err)
	}
	return res, nil
}
