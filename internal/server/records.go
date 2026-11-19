package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/store"
	"athena-erp/internal/types"
	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// decimalPattern matches the DECIMAL(18,6) column: up to 12 integer digits and 6 fraction digits.
var decimalPattern = regexp.MustCompile(`^-?[0-9]{1,12}(\.[0-9]{1,6})?$`)

// emailDomainDot requires a dot in the domain, which mail.ParseAddress allows
// to be missing. It mirrors the email pattern in web/src/fieldRules.ts.
var emailDomainDot = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// recordService implements athena.v1.RecordService over the Record Type tables.
// It has no authentication yet, so the server must stay bound to loopback.
type recordService struct {
	db    *sql.DB
	hooks *lifecycle.Registry
}

func (s recordService) ListRecords(ctx context.Context, req *athenav1.ListRecordsRequest) (*athenav1.ListRecordsResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	hidden := hiddenFields(p, recordType)
	scope, err := rowScopeFor(ctx, s.db, p, recordType, auth.ActionRead)
	if err != nil {
		return nil, err
	}
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	pageSize = min(pageSize, maxPageSize)
	after := ""
	if req.GetPageToken() != "" {
		if after, err = decodeCursor(req.GetPageToken(), recordType.Name); err != nil {
			return nil, err
		}
	}
	// UUIDv7 IDs sort by creation time, so the primary key is the keyset cursor.
	where, args := "`id` > ?", []any{after}
	if recordType.kind() == kindChild && req.GetParentId() != "" {
		if _, err := uuid.Parse(req.GetParentId()); err != nil {
			return nil, invalid("parent_id must be a record ID")
		}
		where += " AND `parent_id` = ?"
		args = append(args, req.GetParentId())
	} else if req.GetParentId() != "" {
		return nil, invalid("only child Record Types have a parent")
	}
	if live := liveClause(recordType); live != "" {
		where += " AND " + live
	}
	if !scope.unrestricted {
		where += " AND (" + scope.where + ")"
		args = append(args, scope.args...)
	}
	if req.GetFilter() != "" {
		predicate, err := filterPredicate(ctx, s.db, visibleDefinition(recordType, hidden), req.GetFilter())
		if err != nil {
			return nil, err
		}
		where += " AND (" + predicate.Where + ")"
		args = append(args, predicate.Args...)
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT ?", selectColumns(recordType), dialect.QuoteIdent(recordTypeTableName(recordType.Name)), where, dialect.QuoteIdent("id"))
	rows, err := s.db.QueryContext(ctx, query, append(args, pageSize+1)...)
	if err != nil {
		return nil, internalError("list records", err)
	}
	defer rows.Close()
	records := make([]*athenav1.Record, 0, pageSize)
	for rows.Next() {
		record, err := scanRecord(rows, recordType)
		if err != nil {
			return nil, internalError("read record", err)
		}
		maskRecord(hidden, record)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("list records", err)
	}
	res := &athenav1.ListRecordsResponse{}
	if len(records) > pageSize {
		records = records[:pageSize]
		res.NextPageToken = encodeCursor(recordType.Name, records[pageSize-1].GetId())
	}
	res.Records = records
	return res, nil
}

func (s recordService) GetRecord(ctx context.Context, req *athenav1.GetRecordRequest) (*athenav1.GetRecordResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	if err := authorizeRecord(ctx, s.db, p, recordType, req.GetId(), auth.ActionRead); err != nil {
		return nil, err
	}
	record, err := s.load(ctx, recordType, req.GetId())
	if err != nil {
		return nil, err
	}
	maskRecord(hiddenFields(p, recordType), record)
	return &athenav1.GetRecordResponse{Record: record}, nil
}

// localActor names who made a change through the athena command line, which
// opens the database directly and has no signed-in user.
const localActor = "Local administrator"

func (s recordService) CreateRecord(ctx context.Context, req *athenav1.CreateRecordRequest) (res *athenav1.CreateRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "create_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	hidden := hiddenFields(p, recordType)
	if err := checkWritable(hidden, recordType, req.GetValues()); err != nil {
		return nil, err
	}
	columns, args, err := bindValues(recordType, req.GetValues(), true)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, internalError("generate record ID", err)
	}
	if recordType.kind() != kindChild && req.GetParentId() != "" {
		return nil, invalid("only child Record Types have a parent")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("create record", err)
	}
	defer func() { _ = tx.Rollback() }()
	system := map[string]any{"created_by": actorName(ctx), "modified_by": actorName(ctx), "def_version": recordType.Revision, "request_id": requestID, "owner": actorName(ctx), "docstate": int32(lifecycle.Draft), "name": id.String()}
	switch recordType.kind() {
	case kindSetting:
		if err := requireNoRecord(ctx, tx, recordType); err != nil {
			return nil, err
		}
	case kindChild:
		idx, err := nextChildIndex(ctx, tx, recordType, req.GetParentId())
		if err != nil {
			return nil, err
		}
		if err := checkParentEditable(ctx, tx, recordType, req.GetParentId(), "create", nil); err != nil {
			return nil, err
		}
		system["parent_id"], system["idx"] = req.GetParentId(), idx
	}
	name, err := manualName(ctx, tx, recordType, req.GetName())
	if err != nil {
		return nil, err
	}
	if name != "" {
		system["name"] = name
	}
	event := lifecycle.Event{RecordType: recordType.Name, RecordID: id.String(), RequestID: requestID, To: lifecycle.Draft, Tx: tx}
	if recordType.kind() == kindDocument {
		if event.Doc, err = pendingDocument(ctx, recordType, id.String(), nil, columns, args); err != nil {
			return nil, err
		}
		if err := s.fireSteps(ctx, lifecycle.Sequence(lifecycle.SaveNew), event, 0, 4); err != nil {
			return nil, err
		}
		if columns, args, _, err = mergeRuleWrites(recordType, nil, event.Doc, columns, args, nil); err != nil {
			return nil, err
		}
	}
	if columns, args, _, err = applyExpressions(ctx, tx, recordType, nil, id.String(), columns, args); err != nil {
		return nil, err
	}
	changed := changedKeys(columns, args)
	if err := checkUnique(ctx, tx, recordType, columns, args, ""); err != nil {
		return nil, err
	}
	if err := checkLinkTargets(ctx, tx, recordType, columns, args); err != nil {
		return nil, err
	}
	if err := insertRow(ctx, tx, recordType, id.String(), system, columns, args); err != nil {
		return nil, err
	}
	if recordType.kind() == kindChild {
		if err := refreshParent(ctx, tx, recordType, req.GetParentId()); err != nil {
			return nil, err
		}
	}
	// A Record the creator could not open under their own Grants is refused.
	if err := authorizeRecord(ctx, tx, p, recordType, id.String(), auth.ActionCreate); err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, "you do not have permission to create a record like this")
	}
	if err := auditCreated(ctx, tx, recordType, id.String(), changed); err != nil {
		return nil, err
	}
	if name != "" {
		if err := auditNameOverride(ctx, tx, recordType.Name, id.String(), name); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("create record", err)
	}
	record, err := s.load(ctx, recordType, id.String())
	if err != nil {
		return nil, err
	}
	maskRecord(hidden, record)
	return &athenav1.CreateRecordResponse{Record: record}, nil
}

// insertRow inserts one Record with its system columns. system supplies the
// kernel's values by column name; columns and args come from bindValues.
func insertRow(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id string, system map[string]any, columns []string, args []any) error {
	insertColumns, insertArgs := []string{dialect.QuoteIdent("id")}, []any{id}
	for _, column := range recordType.systemColumns() {
		insertColumns = append(insertColumns, dialect.QuoteIdent(column.Name))
		insertArgs = append(insertArgs, system[column.Name])
	}
	columns = append(insertColumns, columns...)
	args = append(insertArgs, args...)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	statement := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", dialect.QuoteIdent(recordTypeTableName(recordType.Name)), strings.Join(columns, ","), placeholders)
	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return writeError("create record", err)
	}
	return nil
}

// requireNoRecord keeps a Setting to one Record. Locking the Record Type's
// definition row serializes concurrent first creates, which a row lock on the
// empty table could not.
func requireNoRecord(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition) error {
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM athena_record_types WHERE name = ? `+dialect.ForUpdate(), recordType.Name).Scan(&revision); err != nil {
		return internalError("lock Record Type", err)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+dialect.QuoteIdent(recordTypeTableName(recordType.Name))+")").Scan(&exists); err != nil {
		return internalError("check settings", err)
	}
	if exists {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s already has its record; edit it instead", recordType.Label))
	}
	return nil
}

// nextChildIndex checks that the parent exists and is live, and returns the
// next position among its children. The parent row lock keeps concurrent
// creates from taking the same position.
func nextChildIndex(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, parentID string) (int64, error) {
	if _, err := uuid.Parse(parentID); err != nil {
		return 0, invalid("a child record needs the ID of the record that owns it (parent_id)")
	}
	parent, _, err := loadRecordType(ctx, tx, recordType.ParentRecordType)
	if err != nil || parent == nil {
		return 0, internalError("load parent Record Type", fmt.Errorf("parent %q: %v", recordType.ParentRecordType, err))
	}
	query := "SELECT `id` FROM " + dialect.QuoteIdent(recordTypeTableName(parent.Name)) + " WHERE `id` = ?"
	if live := liveClause(*parent); live != "" {
		query += " AND " + live
	}
	var found string
	switch err := tx.QueryRowContext(ctx, query+" "+dialect.ForUpdate(), parentID).Scan(&found); {
	case errors.Is(err, sql.ErrNoRows):
		return 0, invalid(fmt.Sprintf("the %s that should own this record does not exist", parent.Label))
	case err != nil:
		return 0, internalError("check parent", err)
	}
	var next int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(`idx`), 0) + 1 FROM "+dialect.QuoteIdent(recordTypeTableName(recordType.Name))+" WHERE `parent_id` = ?", parentID).Scan(&next); err != nil {
		return 0, internalError("position child record", err)
	}
	return next, nil
}

// checkUnique refuses a value another live Record already holds, so people
// get a message naming the field. The unique index is the final guard against
// a race. A soft-deleted Master keeps its values reserved (the index sees it).
func checkUnique(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, columns []string, args []any, selfID string) error {
	labels := map[string]string{}
	unique := map[string]bool{}
	for _, field := range recordType.Fields {
		labels[field.Name], unique[field.Name] = field.Label, field.Unique
	}
	for i, column := range columns {
		key := strings.Trim(column, "`")
		if !unique[key] || args[i] == nil {
			continue
		}
		query := "SELECT EXISTS(SELECT 1 FROM " + dialect.QuoteIdent(recordTypeTableName(recordType.Name)) + " WHERE " + column + " = ?"
		params := []any{args[i]}
		if selfID != "" {
			query += " AND `id` <> ?"
			params = append(params, selfID)
		}
		if live := liveClause(recordType); live != "" {
			query += " AND " + live
		}
		var taken bool
		if err := tx.QueryRowContext(ctx, query+")", params...).Scan(&taken); err != nil {
			return internalError("check unique value", err)
		}
		if taken {
			return invalid(fmt.Sprintf("%s is already used by another record", labels[key]))
		}
	}
	return nil
}

func (s recordService) UpdateRecord(ctx context.Context, req *athenav1.UpdateRecordRequest) (res *athenav1.UpdateRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "update_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	hidden := hiddenFields(p, recordType)
	if err := checkWritable(hidden, recordType, req.GetValues()); err != nil {
		return nil, err
	}
	columns, args, err := bindValues(recordType, req.GetValues(), false)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("update record", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := authorizeRecord(ctx, tx, p, recordType, req.GetId(), auth.ActionWrite); err != nil {
		return nil, err
	}
	current, err := loadRecord(ctx, tx, recordType, req.GetId(), dialect.ForUpdate())
	if err != nil {
		return nil, err
	}
	if current.GetRevision() != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this record; reload it and try again")
	}
	// Only fields whose stored value differs count as changed; an unchanged save
	// keeps the revision and writes no history.
	fields := fieldsByKey(recordType)
	state := lifecycle.State(current.GetDocstate())
	if recordType.Submittable && state == lifecycle.Cancelled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "a cancelled document cannot be changed")
	}
	var changedColumns, changed []string
	var changedArgs []any
	for i, column := range columns {
		key := strings.Trim(column, "`")
		field := fields[key]
		if sameValue(field, current.GetValues()[key], args[i]) {
			continue
		}
		if recordType.Submittable && state == lifecycle.Submitted && !field.AllowOnSubmit {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s cannot be changed after the document is submitted", field.Label))
		}
		if field.ReadOnly {
			return nil, invalid(fmt.Sprintf("%s is read only", field.Label))
		}
		if field.SetOnce && !current.GetValues()[key].GetNullValue() && current.GetValues()[key].GetKind() != nil {
			return nil, invalid(fmt.Sprintf("%s can be set once and cannot change", field.Label))
		}
		changedColumns = append(changedColumns, column)
		changedArgs = append(changedArgs, args[i])
		changed = append(changed, key)
	}
	if len(changed) == 0 {
		maskRecord(hidden, current)
		return &athenav1.UpdateRecordResponse{Record: current}, nil
	}
	if recordType.kind() == kindChild {
		if err := checkParentEditable(ctx, tx, recordType, current.GetParentId(), "update", changed); err != nil {
			return nil, err
		}
	}
	event := lifecycle.Event{RecordType: recordType.Name, RecordID: req.GetId(), RequestID: requestID, From: state, To: state, Tx: tx}
	if recordType.kind() == kindDocument {
		if event.Doc, err = pendingDocument(ctx, recordType, req.GetId(), current, changedColumns, changedArgs); err != nil {
			return nil, err
		}
		if err := s.fireSteps(ctx, lifecycle.Sequence(lifecycle.SaveEdit), event, 0, 3); err != nil {
			return nil, err
		}
		if changedColumns, changedArgs, changed, err = mergeRuleWrites(recordType, current, event.Doc, changedColumns, changedArgs, changed); err != nil {
			return nil, err
		}
	}
	computedColumns, computedArgs, computedKeys, err := applyExpressions(ctx, tx, recordType, current, req.GetId(), changedColumns, changedArgs)
	if err != nil {
		return nil, err
	}
	changedColumns, changedArgs, changed = computedColumns, computedArgs, append(changed, computedKeys...)
	if err := checkUnique(ctx, tx, recordType, changedColumns, changedArgs, req.GetId()); err != nil {
		return nil, err
	}
	if err := checkLinkTargets(ctx, tx, recordType, changedColumns, changedArgs); err != nil {
		return nil, err
	}
	assignments := []string{dialect.QuoteIdent("revision") + " = " + dialect.QuoteIdent("revision") + " + 1"}
	for _, column := range changedColumns {
		assignments = append(assignments, column+" = ?")
	}
	for _, kernel := range []struct {
		column string
		value  any
	}{{"modified_by", actorName(ctx)}, {"def_version", recordType.Revision}, {"request_id", requestID}} {
		assignments = append(assignments, dialect.QuoteIdent(kernel.column)+" = ?")
		changedArgs = append(changedArgs, kernel.value)
	}
	changedArgs = append(changedArgs, req.GetId(), req.GetExpectedRevision())
	statement := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = ? AND `revision` = ?", dialect.QuoteIdent(recordTypeTableName(recordType.Name)), strings.Join(assignments, ", "))
	if _, err := tx.ExecContext(ctx, statement, changedArgs...); err != nil {
		return nil, writeError("update record", err)
	}
	if recordType.kind() == kindChild {
		if err := refreshParent(ctx, tx, recordType, current.GetParentId()); err != nil {
			return nil, err
		}
	}
	updated, err := loadRecord(ctx, tx, recordType, req.GetId(), "")
	if err != nil {
		return nil, err
	}
	if err := writeAudit(ctx, tx, recordType.Name, req.GetId(), current.GetRevision()+1, "updated", fieldChanges(changed, current, updated)); err != nil {
		return nil, err
	}
	if recordType.Submittable && state == lifecycle.Submitted {
		if err := writeActivity(ctx, tx, activityRow{recordType: recordType.Name, recordID: req.GetId(), event: "edited_after_submit", requestID: requestID, changed: changed}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("update record", err)
	}
	record, err := s.load(ctx, recordType, req.GetId())
	if err != nil {
		return nil, err
	}
	maskRecord(hidden, record)
	return &athenav1.UpdateRecordResponse{Record: record}, nil
}

func fieldsByKey(recordType recordTypeDefinition) map[string]recordTypeField {
	fields := make(map[string]recordTypeField, len(recordType.Fields))
	for _, field := range recordType.Fields {
		fields[field.Name] = field
	}
	return fields
}

// liveClause hides soft-deleted Records. Only Masters are soft-deleted.
func liveClause(recordType recordTypeDefinition) string {
	if recordType.kind() == kindMaster {
		return "`deleted_at` IS NULL"
	}
	return ""
}

func (s recordService) DeleteRecord(ctx context.Context, req *athenav1.DeleteRecordRequest) (res *athenav1.DeleteRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "delete_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	if recordType.kind() == kindSetting {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s is a setting; edit it instead of deleting it", recordType.Label))
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("delete record", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := authorizeRecord(ctx, tx, p, recordType, req.GetId(), auth.ActionDelete); err != nil {
		return nil, err
	}
	table := dialect.QuoteIdent(recordTypeTableName(recordType.Name))
	var parentID string
	switch recordType.kind() {
	case kindChild:
		if err := tx.QueryRowContext(ctx, "SELECT `parent_id` FROM "+table+" WHERE `id` = ?", req.GetId()).Scan(&parentID); err == nil {
			if err := checkParentEditable(ctx, tx, recordType, parentID, "delete", nil); err != nil {
				return nil, err
			}
		}
	case kindDocument:
		current, err := loadRecord(ctx, tx, recordType, req.GetId(), dialect.ForUpdate())
		if err != nil {
			return nil, err
		}
		if recordType.Submittable && lifecycle.State(current.GetDocstate()) != lifecycle.Draft {
			return nil, connect.NewError(connect.CodeFailedPrecondition, "only a draft can be deleted; a submitted document is cancelled instead")
		}
		event := lifecycle.Event{RecordType: recordType.Name, RecordID: req.GetId(), RequestID: requestID, From: lifecycle.Draft, To: lifecycle.Draft, Tx: tx}
		if event.Doc, err = pendingDocument(ctx, recordType, req.GetId(), current, nil, nil); err != nil {
			return nil, err
		}
		if err := s.fireSteps(ctx, lifecycle.Sequence(lifecycle.Trash), event, 0, 1); err != nil {
			return nil, err
		}
	}
	// A soft delete leaves the row in place, so the foreign key on a child's
	// parent_id never fires. Check for children here, in the same transaction.
	if err := ensureNoChildRecords(ctx, tx, recordType.Name, req.GetId()); err != nil {
		return nil, err
	}
	// A Master is soft-deleted: the row stays, marked, so nothing that refers
	// to it breaks and the data can be audited. Other kinds remove the row.
	statement := fmt.Sprintf("DELETE FROM %s WHERE `id` = ? AND `revision` = ?", table)
	args := []any{req.GetId(), req.GetExpectedRevision()}
	if recordType.kind() == kindMaster {
		statement = fmt.Sprintf("UPDATE %s SET `deleted_at` = %s, `revision` = `revision` + 1, `modified_by` = ?, `def_version` = ?, `request_id` = ? WHERE `id` = ? AND `revision` = ? AND `deleted_at` IS NULL", table, dialect.Now())
		args = append([]any{actorName(ctx), recordType.Revision, requestID}, args...)
	}
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return nil, writeError("delete record", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		_ = tx.Rollback()
		return nil, s.checkChanged(ctx, recordType, req.GetId(), result)
	}
	if recordType.kind() == kindChild && parentID != "" {
		if err := refreshParent(ctx, tx, recordType, parentID); err != nil {
			return nil, err
		}
	}
	revision := req.GetExpectedRevision()
	if recordType.kind() == kindMaster {
		revision++
	}
	if err := writeAudit(ctx, tx, recordType.Name, req.GetId(), revision, "deleted", nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("delete record", err)
	}
	if recordType.kind() == kindDocument {
		event := lifecycle.Event{RecordType: recordType.Name, RecordID: req.GetId(), RequestID: requestID}
		s.fireAfter(ctx, lifecycle.Sequence(lifecycle.Trash), event)
	}
	return &athenav1.DeleteRecordResponse{}, nil
}

// ensureNoChildRecords returns FailedPrecondition when any child Record Type
// of parentKey has a Record whose parent_id is id.
func ensureNoChildRecords(ctx context.Context, conn *sql.Tx, parentKey, id string) error {
	children, err := childRecordTypes(ctx, conn, parentKey)
	if err != nil {
		return err
	}
	for _, child := range children {
		var found int
		query := "SELECT 1 FROM " + dialect.QuoteIdent(recordTypeTableName(child.Name)) + " WHERE `parent_id` = ? LIMIT 1"
		switch err := conn.QueryRowContext(ctx, query, id).Scan(&found); {
		case err == nil:
			return connect.NewError(connect.CodeFailedPrecondition, "other records belong to this record")
		case !errors.Is(err, sql.ErrNoRows):
			return internalError("check child records", err)
		}
	}
	return nil
}
func (s recordService) ListRecordHistory(ctx context.Context, req *athenav1.ListRecordHistoryRequest) (*athenav1.ListRecordHistoryResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
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
	hidden := hiddenFields(p, recordType)
	rows, err := audit.Rows(ctx, s.db, recordType.Name, req.GetId(), 200)
	if err != nil {
		return nil, internalError("read history", err)
	}
	res := &athenav1.ListRecordHistoryResponse{}
	for _, row := range rows {
		event := &athenav1.RecordEvent{Action: row.Entry.Action, Revision: row.Entry.Revision, Actor: row.Entry.Actor, At: row.At.Format(time.RFC3339),
			RequestId: row.Entry.RequestID, Source: row.Entry.Source, Seq: row.Seq, Hash: row.Hash}
		for _, change := range row.Entry.Changes {
			event.ChangedFields = append(event.ChangedFields, change.Field)
		}
		event.Changes = maskChanges(hidden, row.Entry.Changes)
		res.Events = append(res.Events, event)
	}
	if len(res.Events) > 0 {
		return res, nil
	}
	// Records written before history existed: describe what the row itself records.
	record, err := s.load(ctx, recordType, req.GetId())
	if err != nil {
		return nil, err
	}
	if record.GetUpdatedAt() != record.GetCreatedAt() || record.GetRevision() > 1 {
		res.Events = append(res.Events, &athenav1.RecordEvent{Action: "updated", Revision: record.GetRevision(), Actor: localActor, At: record.GetUpdatedAt()})
	}
	res.Events = append(res.Events, &athenav1.RecordEvent{Action: "created", Revision: 1, Actor: localActor, At: record.GetCreatedAt()})
	return res, nil
}

// sameValue reports whether a stored value equals a bound argument from bindValues.
func sameValue(field recordTypeField, stored *athenav1.FieldValue, arg any) bool {
	switch v := arg.(type) {
	case nil:
		return stored == nil || stored.GetNullValue()
	case bool:
		b, ok := stored.GetKind().(*athenav1.FieldValue_BoolValue)
		return ok && b.BoolValue == v
	case int64:
		text, ok := stored.GetKind().(*athenav1.FieldValue_StringValue)
		return ok && field.Type == "Money" && text.StringValue == moneyText(field, v)
	case string:
		text, ok := stored.GetKind().(*athenav1.FieldValue_StringValue)
		return ok && (text.StringValue == v || (decimalPattern.MatchString(v) && trimDecimal(text.StringValue) == trimDecimal(v)))
	}
	return false
}

// recordType loads a saved Record Type definition. Table and column names in later queries
// come only from this definition, never from the request.
func (s recordService) recordType(ctx context.Context, key string) (recordTypeDefinition, error) {
	if s.db == nil {
		return recordTypeDefinition{}, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	return loadRecordTypeFrom(ctx, s.db, key)
}

// loadRecordTypeFrom reads a saved definition through a database or a
// transaction, so a Rule reads the same revision the document is saved under.
func loadRecordTypeFrom(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (recordTypeDefinition, error) {
	var payload []byte
	var revision int64
	err := q.QueryRowContext(ctx, `SELECT definition, revision FROM athena_record_types WHERE name = ?`, key).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return recordTypeDefinition{}, connect.NewError(connect.CodeNotFound, fmt.Sprintf("Record Type %q does not exist", key))
	}
	if err != nil {
		return recordTypeDefinition{}, internalError("load Record Type", err)
	}
	var recordType recordTypeDefinition
	if err := json.Unmarshal(payload, &recordType); err != nil {
		return recordTypeDefinition{}, internalError("decode Record Type", err)
	}
	recordType.Revision = revision
	return recordType, nil
}

func (s recordService) load(ctx context.Context, recordType recordTypeDefinition, id string) (*athenav1.Record, error) {
	return loadRecord(ctx, s.db, recordType, id, "")
}

// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// loadRecord reads one record. suffix is "" or "FOR UPDATE" inside a transaction.
func loadRecord(ctx context.Context, q querier, recordType recordTypeDefinition, id, suffix string) (*athenav1.Record, error) {
	where := "`id` = ?"
	if live := liveClause(recordType); live != "" {
		where += " AND " + live
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s %s", selectColumns(recordType), dialect.QuoteIdent(recordTypeTableName(recordType.Name)), where, suffix)
	rows, err := q.QueryContext(ctx, query, id)
	if err != nil {
		return nil, internalError("read record", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, internalError("read record", err)
		}
		return nil, connect.NewError(connect.CodeNotFound, "the record does not exist")
	}
	record, err := scanRecord(rows, recordType)
	if err != nil {
		return nil, internalError("read record", err)
	}
	return record, nil
}

// checkChanged tells a missing record apart from a revision conflict after a guarded write.
func (s recordService) checkChanged(ctx context.Context, recordType recordTypeDefinition, id string, result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return internalError("check write", err)
	}
	if affected > 0 {
		return nil
	}
	var exists bool
	where := "`id` = ?"
	if live := liveClause(recordType); live != "" {
		where += " AND " + live
	}
	statement := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s)", dialect.QuoteIdent(recordTypeTableName(recordType.Name)), where)
	if err := s.db.QueryRowContext(ctx, statement, id).Scan(&exists); err != nil {
		return internalError("check write", err)
	}
	if !exists {
		return connect.NewError(connect.CodeNotFound, "the record does not exist")
	}
	return connect.NewError(connect.CodeAborted, "someone else changed this record; reload it and try again")
}

func selectColumns(recordType recordTypeDefinition) string {
	columns := []string{dialect.QuoteIdent("id"), dialect.QuoteIdent("revision"), dialect.QuoteIdent("created_at"), dialect.QuoteIdent("updated_at")}
	for _, column := range recordType.systemColumns() {
		columns = append(columns, dialect.QuoteIdent(column.Name))
	}
	for _, field := range recordType.Fields {
		columns = append(columns, dialect.QuoteIdent(field.Name))
	}
	return strings.Join(columns, ",")
}

func scanRecord(rows *sql.Rows, recordType recordTypeDefinition) (*athenav1.Record, error) {
	var record athenav1.Record
	var created, updated time.Time
	targets := []any{&record.Id, &record.Revision, &created, &updated}
	// Rows written before system fields existed hold NULL in them.
	var createdBy, modifiedBy, requestID, owner, parentID sql.NullString
	var defVersion, idx, docstate sql.NullInt64
	var name, amendedFrom sql.NullString
	var deletedAt sql.NullTime
	system := map[string]any{"created_by": &createdBy, "modified_by": &modifiedBy, "request_id": &requestID, "owner": &owner, "parent_id": &parentID, "def_version": &defVersion, "idx": &idx, "deleted_at": &deletedAt, "docstate": &docstate, "name": &name, "amended_from": &amendedFrom}
	for _, column := range recordType.systemColumns() {
		targets = append(targets, system[column.Name])
	}
	values := make([]any, len(recordType.Fields))
	for i, field := range recordType.Fields {
		switch field.Type {
		case "Checkbox":
			values[i] = new(sql.NullBool)
		case "Date":
			values[i] = new(sql.NullTime)
		case "Money":
			values[i] = new(sql.NullInt64)
		default:
			values[i] = new(sql.NullString)
		}
		targets = append(targets, values[i])
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	record.CreatedAt = created.UTC().Format(time.RFC3339)
	record.UpdatedAt = updated.UTC().Format(time.RFC3339)
	record.CreatedBy, record.ModifiedBy, record.Owner, record.RequestId, record.ParentId = createdBy.String, modifiedBy.String, owner.String, requestID.String, parentID.String
	record.DefVersion, record.Idx = defVersion.Int64, idx.Int64
	record.Docstate, record.Name, record.AmendedFrom = int32(docstate.Int64), name.String, amendedFrom.String
	record.Values = make(map[string]*athenav1.FieldValue, len(recordType.Fields))
	for i, field := range recordType.Fields {
		value := &athenav1.FieldValue{}
		switch v := values[i].(type) {
		case *sql.NullBool:
			if v.Valid {
				value.Kind = &athenav1.FieldValue_BoolValue{BoolValue: v.Bool}
			}
		case *sql.NullTime:
			if v.Valid {
				value.Kind = &athenav1.FieldValue_StringValue{StringValue: v.Time.Format(time.DateOnly)}
			}
		case *sql.NullInt64:
			if v.Valid {
				value.Kind = &athenav1.FieldValue_StringValue{StringValue: moneyText(field, v.Int64)}
			}
		case *sql.NullString:
			if v.Valid {
				text := v.String
				if field.Type == "Number" || field.Type == "Currency" || field.Type == "Quantity" {
					text = trimDecimal(text)
				}
				value.Kind = &athenav1.FieldValue_StringValue{StringValue: text}
			}
		}
		if value.Kind == nil {
			value.Kind = &athenav1.FieldValue_NullValue{NullValue: true}
		}
		record.Values[field.Name] = value
	}
	return &record, nil
}

// trimDecimal removes the trailing zeros MariaDB pads DECIMAL(18,6) values with.
func trimDecimal(text string) string {
	if !strings.Contains(text, ".") {
		return text
	}
	return strings.TrimSuffix(strings.TrimRight(text, "0"), ".")
}

// bindValues validates request values against the Record Type's fields and returns
// quoted column names with matching bound arguments. Creating requires every
// required field and fills defaults; updating checks only the fields sent.
func bindValues(recordType recordTypeDefinition, values map[string]*athenav1.FieldValue, creating bool) ([]string, []any, error) {
	fields := fieldsByKey(recordType)
	for key := range values {
		if _, ok := fields[key]; !ok {
			return nil, nil, invalid(fmt.Sprintf("Record Type %q has no field %q", recordType.Name, key))
		}
	}
	var columns []string
	var args []any
	for _, field := range recordType.Fields {
		value, present := values[field.Name]
		if field.Formula != "" {
			// The kernel stores the result; a client may only echo an empty value.
			if arg, err := fieldArgument(field, value); present && (err != nil || arg != nil) {
				return nil, nil, invalid(fmt.Sprintf("%s is calculated and cannot be set", field.Label))
			}
			continue
		}
		var arg any
		if present {
			var err error
			if arg, err = fieldArgument(field, value); err != nil {
				return nil, nil, err
			}
		}
		if creating {
			fallback, err := createDefault(field)
			if err != nil {
				return nil, nil, err
			}
			// A read-only field takes only its default, even when the client echoes it back.
			if field.ReadOnly && arg != nil && arg != fallback {
				return nil, nil, invalid(fmt.Sprintf("%s is read only", field.Label))
			}
			if arg == nil {
				arg = fallback
			}
		}
		if !present && arg == nil {
			if creating && field.Required {
				return nil, nil, invalid(fmt.Sprintf("%s is required", field.Label))
			}
			continue
		}
		if arg == nil && field.Required {
			return nil, nil, invalid(fmt.Sprintf("%s is required", field.Label))
		}
		columns = append(columns, dialect.QuoteIdent(field.Name))
		args = append(args, arg)
	}
	if !creating && len(columns) == 0 {
		return nil, nil, invalid("no field values to update")
	}
	return columns, args, nil
}

// createDefault returns the database argument for a field's default, or nil.
func createDefault(field recordTypeField) (any, error) {
	if field.Default == "" {
		return nil, nil
	}
	return defaultArgument(field)
}

// fieldArgument converts one FieldValue into a database argument, or nil for
// null. It applies the type's own rules, then the field's attributes. A Money
// field returns int64 minor units; every other type returns a string or bool.
func fieldArgument(field recordTypeField, value *athenav1.FieldValue) (any, error) {
	if value == nil || value.GetNullValue() {
		return nil, nil
	}
	if field.Type == "Checkbox" {
		v, ok := value.GetKind().(*athenav1.FieldValue_BoolValue)
		if !ok {
			return nil, invalid(fmt.Sprintf("%s must be true or false", field.Label))
		}
		return v.BoolValue, nil
	}
	v, ok := value.GetKind().(*athenav1.FieldValue_StringValue)
	if !ok {
		return nil, invalid(fmt.Sprintf("%s must be text", field.Label))
	}
	text := strings.TrimSpace(v.StringValue)
	if text == "" {
		return nil, nil
	}
	limit := textLimits[field.Type]
	if field.MaxLength > 0 && field.MaxLength < limit {
		limit = field.MaxLength
	}
	if limit > 0 && utf8.RuneCountInString(text) > limit {
		return nil, invalid(fmt.Sprintf("%s is longer than %d characters", field.Label, limit))
	}
	if field.Pattern != "" {
		re, err := compilePattern(field.Pattern)
		if err != nil {
			return nil, internalError("read pattern", err)
		}
		if !re.MatchString(text) {
			return nil, invalid(fmt.Sprintf("%s does not match the required format", field.Label))
		}
	}
	switch field.Type {
	case "Email":
		if address, err := mail.ParseAddress(text); err != nil || address.Address != text || !emailDomainDot.MatchString(text) {
			return nil, invalid(fmt.Sprintf("%s must be an email address", field.Label))
		}
	case "Number", "Currency":
		if !decimalPattern.MatchString(text) {
			return nil, invalid(fmt.Sprintf("%s must be a number with up to 12 digits before and 6 after the decimal point", field.Label))
		}
		if err := checkRange(field, text); err != nil {
			return nil, err
		}
	case "Quantity":
		if !decimalPattern.MatchString(text) {
			return nil, invalid(fmt.Sprintf("%s must be a number with up to 12 digits before and 6 after the decimal point", field.Label))
		}
		scale := maxQuantityScale
		if field.Scale != nil {
			scale = *field.Scale
		}
		if _, fraction, _ := strings.Cut(text, "."); len(fraction) > scale {
			return nil, invalid(fmt.Sprintf("%s must have at most %d decimal places", field.Label, scale))
		}
		if err := checkRange(field, text); err != nil {
			return nil, err
		}
	case "Money":
		minor, err := moneyMinor(field, text)
		if err != nil {
			if errors.Is(err, types.ErrRounding) {
				return nil, invalid(fmt.Sprintf("%s must have at most %d decimal places", field.Label, *field.Scale))
			}
			return nil, invalid(fmt.Sprintf("%s must be an amount in %s with at most %d decimal places", field.Label, field.Currency, *field.Scale))
		}
		if err := checkRange(field, text); err != nil {
			return nil, err
		}
		return minor, nil
	case "Date":
		if _, err := time.Parse(time.DateOnly, text); err != nil {
			return nil, invalid(fmt.Sprintf("%s must be a date as YYYY-MM-DD", field.Label))
		}
		if err := checkRange(field, text); err != nil {
			return nil, err
		}
	case "Enum":
		for _, choice := range field.Values {
			if choice.Value == text {
				return text, nil
			}
		}
		return nil, invalid(fmt.Sprintf("%s must be one of the listed choices", field.Label))
	case "Link":
		if _, err := uuid.Parse(text); err != nil {
			return nil, invalid(fmt.Sprintf("%s must be an record ID", field.Label))
		}
	}
	return text, nil
}

type cursor struct {
	RecordType string `json:"r"`
	After      string `json:"a"`
}

func encodeCursor(recordType, after string) string {
	payload, _ := json.Marshal(cursor{RecordType: recordType, After: after})
	return base64.RawURLEncoding.EncodeToString(payload)
}

// decodeCursor rejects a token issued for a different Record Type.
func decodeCursor(token, recordType string) (string, error) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	var c cursor
	if err != nil || json.Unmarshal(payload, &c) != nil || c.RecordType != recordType {
		return "", invalid("page_token is not valid for this Record Type")
	}
	if _, err := uuid.Parse(c.After); err != nil {
		return "", invalid("page_token is not valid for this Record Type")
	}
	return c.After, nil
}

func invalid(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, message)
}

// internalError logs the cause on the server and hides driver detail from the client.
func internalError(action string, err error) error {
	log.Printf("record service: %s: %v", action, err)
	return connect.NewError(connect.CodeInternal, "could not "+action)
}

// writeError maps database constraint failures to client errors.
func writeError(action string, err error) error {
	switch dialect.Classify(err) {
	case store.ErrForeignKeyMissingParent:
		return invalid("a linked record does not exist")
	case store.ErrForeignKeyInUse:
		return connect.NewError(connect.CodeFailedPrecondition, "other records link to this record")
	case store.ErrDuplicateKey:
		return invalid("a value that must be unique is already used, possibly by a deleted record")
	case store.ErrNotNull:
		return invalid("a required field is missing")
	case store.ErrValueTooLong:
		return invalid("a value does not fit its field")
	}
	return internalError(action, err)
}
