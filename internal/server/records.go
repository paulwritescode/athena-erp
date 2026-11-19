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
	"athena-erp/internal/store"
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
	db *sql.DB
}

func (s recordService) ListRecords(ctx context.Context, req *athenav1.ListRecordsRequest) (*athenav1.ListRecordsResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
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
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s > ? ORDER BY %s LIMIT ?", selectColumns(recordType), dialect.QuoteIdent(recordTypeTableName(recordType.Name)), dialect.QuoteIdent("id"), dialect.QuoteIdent("id"))
	rows, err := s.db.QueryContext(ctx, query, after, pageSize+1)
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
	record, err := s.load(ctx, recordType, req.GetId())
	if err != nil {
		return nil, err
	}
	return &athenav1.GetRecordResponse{Record: record}, nil
}

// localActor names who made a change until sign-in exists.
const localActor = "Local administrator"

func (s recordService) CreateRecord(ctx context.Context, req *athenav1.CreateRecordRequest) (*athenav1.CreateRecordResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
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
	var changed []string
	for i, column := range columns {
		if args[i] != nil {
			changed = append(changed, strings.Trim(column, "`"))
		}
	}
	columns = append([]string{dialect.QuoteIdent("id")}, columns...)
	args = append([]any{id.String()}, args...)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	statement := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", dialect.QuoteIdent(recordTypeTableName(recordType.Name)), strings.Join(columns, ","), placeholders)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("create record", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return nil, writeError("create record", err)
	}
	if err := writeHistory(ctx, tx, recordType.Name, id.String(), 1, "created", changed); err != nil {
		return nil, internalError("record type history", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("create record", err)
	}
	record, err := s.load(ctx, recordType, id.String())
	if err != nil {
		return nil, err
	}
	return &athenav1.CreateRecordResponse{Record: record}, nil
}

func (s recordService) UpdateRecord(ctx context.Context, req *athenav1.UpdateRecordRequest) (*athenav1.UpdateRecordResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
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
	current, err := loadRecord(ctx, tx, recordType, req.GetId(), dialect.ForUpdate())
	if err != nil {
		return nil, err
	}
	if current.GetRevision() != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this record; reload it and try again")
	}
	// Only fields whose stored value differs count as changed; an unchanged save
	// keeps the revision and writes no history.
	var changedColumns, changed []string
	var changedArgs []any
	for i, column := range columns {
		key := strings.Trim(column, "`")
		if sameValue(current.GetValues()[key], args[i]) {
			continue
		}
		changedColumns = append(changedColumns, column)
		changedArgs = append(changedArgs, args[i])
		changed = append(changed, key)
	}
	if len(changed) == 0 {
		return &athenav1.UpdateRecordResponse{Record: current}, nil
	}
	assignments := []string{dialect.QuoteIdent("revision") + " = " + dialect.QuoteIdent("revision") + " + 1"}
	for _, column := range changedColumns {
		assignments = append(assignments, column+" = ?")
	}
	changedArgs = append(changedArgs, req.GetId(), req.GetExpectedRevision())
	statement := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = ? AND `revision` = ?", dialect.QuoteIdent(recordTypeTableName(recordType.Name)), strings.Join(assignments, ", "))
	if _, err := tx.ExecContext(ctx, statement, changedArgs...); err != nil {
		return nil, writeError("update record", err)
	}
	if err := writeHistory(ctx, tx, recordType.Name, req.GetId(), current.GetRevision()+1, "updated", changed); err != nil {
		return nil, internalError("record type history", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("update record", err)
	}
	record, err := s.load(ctx, recordType, req.GetId())
	if err != nil {
		return nil, err
	}
	return &athenav1.UpdateRecordResponse{Record: record}, nil
}

func (s recordService) DeleteRecord(ctx context.Context, req *athenav1.DeleteRecordRequest) (*athenav1.DeleteRecordResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("delete record", err)
	}
	defer func() { _ = tx.Rollback() }()
	statement := fmt.Sprintf("DELETE FROM %s WHERE `id` = ? AND `revision` = ?", dialect.QuoteIdent(recordTypeTableName(recordType.Name)))
	result, err := tx.ExecContext(ctx, statement, req.GetId(), req.GetExpectedRevision())
	if err != nil {
		return nil, writeError("delete record", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		_ = tx.Rollback()
		return nil, s.checkChanged(ctx, recordType, req.GetId(), result)
	}
	if err := writeHistory(ctx, tx, recordType.Name, req.GetId(), req.GetExpectedRevision(), "deleted", nil); err != nil {
		return nil, internalError("record type history", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("delete record", err)
	}
	return &athenav1.DeleteRecordResponse{}, nil
}

func (s recordService) ListRecordHistory(ctx context.Context, req *athenav1.ListRecordHistoryRequest) (*athenav1.ListRecordHistoryResponse, error) {
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT action, revision, changed_fields, actor, created_at FROM athena_record_history WHERE record_type_name = ? AND record_id = ? ORDER BY created_at DESC, id DESC LIMIT 200`, recordType.Name, req.GetId())
	if err != nil {
		return nil, internalError("read history", err)
	}
	defer rows.Close()
	res := &athenav1.ListRecordHistoryResponse{}
	for rows.Next() {
		var event athenav1.RecordEvent
		var changed []byte
		var at time.Time
		if err := rows.Scan(&event.Action, &event.Revision, &changed, &event.Actor, &at); err != nil {
			return nil, internalError("read history", err)
		}
		if err := json.Unmarshal(changed, &event.ChangedFields); err != nil {
			return nil, internalError("read history", err)
		}
		event.At = at.UTC().Format(time.RFC3339)
		res.Events = append(res.Events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("read history", err)
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

func writeHistory(ctx context.Context, tx *sql.Tx, recordTypeName, recordID string, revision int64, action string, changed []string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if changed == nil {
		changed = []string{}
	}
	payload, err := json.Marshal(changed)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_record_history (id, record_type_name, record_id, revision, action, changed_fields, actor) VALUES (?,?,?,?,?,?,?)`, id.String(), recordTypeName, recordID, revision, action, payload, localActor)
	return err
}

// sameValue reports whether a stored value equals a bound argument from bindValues.
func sameValue(stored *athenav1.FieldValue, arg any) bool {
	switch v := arg.(type) {
	case nil:
		return stored == nil || stored.GetNullValue()
	case bool:
		b, ok := stored.GetKind().(*athenav1.FieldValue_BoolValue)
		return ok && b.BoolValue == v
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
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT definition FROM athena_record_types WHERE name = ?`, key).Scan(&payload)
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
	query := fmt.Sprintf("SELECT %s FROM %s WHERE `id` = ? %s", selectColumns(recordType), dialect.QuoteIdent(recordTypeTableName(recordType.Name)), suffix)
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
	statement := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE `id` = ?)", dialect.QuoteIdent(recordTypeTableName(recordType.Name)))
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
	for _, field := range recordType.Fields {
		columns = append(columns, dialect.QuoteIdent(field.Name))
	}
	return strings.Join(columns, ",")
}

func scanRecord(rows *sql.Rows, recordType recordTypeDefinition) (*athenav1.Record, error) {
	var record athenav1.Record
	var created, updated time.Time
	targets := []any{&record.Id, &record.Revision, &created, &updated}
	values := make([]any, len(recordType.Fields))
	for i, field := range recordType.Fields {
		switch field.Type {
		case "Checkbox":
			values[i] = new(sql.NullBool)
		case "Date":
			values[i] = new(sql.NullTime)
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
		case *sql.NullString:
			if v.Valid {
				text := v.String
				if field.Type == "Number" || field.Type == "Currency" {
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
// required field; updating checks only the fields sent.
func bindValues(recordType recordTypeDefinition, values map[string]*athenav1.FieldValue, creating bool) ([]string, []any, error) {
	fields := make(map[string]recordTypeField, len(recordType.Fields))
	for _, field := range recordType.Fields {
		fields[field.Name] = field
	}
	for key := range values {
		if _, ok := fields[key]; !ok {
			return nil, nil, invalid(fmt.Sprintf("Record Type %q has no field %q", recordType.Name, key))
		}
	}
	var columns []string
	var args []any
	for _, field := range recordType.Fields {
		value, present := values[field.Name]
		if !present {
			if creating && field.Required {
				return nil, nil, invalid(fmt.Sprintf("%s is required", field.Label))
			}
			continue
		}
		arg, err := fieldArgument(field, value)
		if err != nil {
			return nil, nil, err
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

// fieldArgument converts one FieldValue into a database argument, or nil for null.
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
	limit := map[string]int{"Text": 255, "Email": 320, "Phone": 40, "Select": 120, "Long text": 65535}[field.Type]
	if limit > 0 && utf8.RuneCountInString(text) > limit {
		return nil, invalid(fmt.Sprintf("%s is longer than %d characters", field.Label, limit))
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
	case "Date":
		if _, err := time.Parse(time.DateOnly, text); err != nil {
			return nil, invalid(fmt.Sprintf("%s must be a date as YYYY-MM-DD", field.Label))
		}
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
	case store.ErrNotNull:
		return invalid("a required field is missing")
	case store.ErrValueTooLong:
		return invalid("a value does not fit its field")
	}
	return internalError(action, err)
}
