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
	"connectrpc.com/connect/v2"
	"github.com/go-sql-driver/mysql"
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

// entryService implements athena.v1.EntryService over the Record tables.
// It has no authentication yet, so the server must stay bound to loopback.
type entryService struct {
	db *sql.DB
}

func (s entryService) ListEntries(ctx context.Context, req *athenav1.ListEntriesRequest) (*athenav1.ListEntriesResponse, error) {
	record, err := s.record(ctx, req.GetRecordKey())
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
		if after, err = decodeCursor(req.GetPageToken(), record.Name); err != nil {
			return nil, err
		}
	}
	// UUIDv7 IDs sort by creation time, so the primary key is the keyset cursor.
	query := fmt.Sprintf("SELECT %s FROM `%s` WHERE `id` > ? ORDER BY `id` LIMIT ?", selectColumns(record), recordTableName(record.Name))
	rows, err := s.db.QueryContext(ctx, query, after, pageSize+1)
	if err != nil {
		return nil, internalError("list entries", err)
	}
	defer rows.Close()
	entries := make([]*athenav1.Entry, 0, pageSize)
	for rows.Next() {
		entry, err := scanEntry(rows, record)
		if err != nil {
			return nil, internalError("read entry", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("list entries", err)
	}
	res := &athenav1.ListEntriesResponse{}
	if len(entries) > pageSize {
		entries = entries[:pageSize]
		res.NextPageToken = encodeCursor(record.Name, entries[pageSize-1].GetId())
	}
	res.Entries = entries
	return res, nil
}

func (s entryService) GetEntry(ctx context.Context, req *athenav1.GetEntryRequest) (*athenav1.GetEntryResponse, error) {
	record, err := s.record(ctx, req.GetRecordKey())
	if err != nil {
		return nil, err
	}
	entry, err := s.load(ctx, record, req.GetId())
	if err != nil {
		return nil, err
	}
	return &athenav1.GetEntryResponse{Entry: entry}, nil
}

// localActor names who made a change until sign-in exists.
const localActor = "Local administrator"

func (s entryService) CreateEntry(ctx context.Context, req *athenav1.CreateEntryRequest) (*athenav1.CreateEntryResponse, error) {
	record, err := s.record(ctx, req.GetRecordKey())
	if err != nil {
		return nil, err
	}
	columns, args, err := bindValues(record, req.GetValues(), true)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, internalError("generate entry ID", err)
	}
	var changed []string
	for i, column := range columns {
		if args[i] != nil {
			changed = append(changed, strings.Trim(column, "`"))
		}
	}
	columns = append([]string{"`id`"}, columns...)
	args = append([]any{id.String()}, args...)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	statement := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)", recordTableName(record.Name), strings.Join(columns, ","), placeholders)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("create entry", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
		return nil, writeError("create entry", err)
	}
	if err := recordHistory(ctx, tx, record.Name, id.String(), 1, "created", changed); err != nil {
		return nil, internalError("record history", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("create entry", err)
	}
	entry, err := s.load(ctx, record, id.String())
	if err != nil {
		return nil, err
	}
	return &athenav1.CreateEntryResponse{Entry: entry}, nil
}

func (s entryService) UpdateEntry(ctx context.Context, req *athenav1.UpdateEntryRequest) (*athenav1.UpdateEntryResponse, error) {
	record, err := s.record(ctx, req.GetRecordKey())
	if err != nil {
		return nil, err
	}
	columns, args, err := bindValues(record, req.GetValues(), false)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("update entry", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadEntry(ctx, tx, record, req.GetId(), "FOR UPDATE")
	if err != nil {
		return nil, err
	}
	if current.GetRevision() != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this entry; reload it and try again")
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
		return &athenav1.UpdateEntryResponse{Entry: current}, nil
	}
	assignments := []string{"`revision` = `revision` + 1"}
	for _, column := range changedColumns {
		assignments = append(assignments, column+" = ?")
	}
	changedArgs = append(changedArgs, req.GetId(), req.GetExpectedRevision())
	statement := fmt.Sprintf("UPDATE `%s` SET %s WHERE `id` = ? AND `revision` = ?", recordTableName(record.Name), strings.Join(assignments, ", "))
	if _, err := tx.ExecContext(ctx, statement, changedArgs...); err != nil {
		return nil, writeError("update entry", err)
	}
	if err := recordHistory(ctx, tx, record.Name, req.GetId(), current.GetRevision()+1, "updated", changed); err != nil {
		return nil, internalError("record history", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("update entry", err)
	}
	entry, err := s.load(ctx, record, req.GetId())
	if err != nil {
		return nil, err
	}
	return &athenav1.UpdateEntryResponse{Entry: entry}, nil
}

func (s entryService) DeleteEntry(ctx context.Context, req *athenav1.DeleteEntryRequest) (*athenav1.DeleteEntryResponse, error) {
	record, err := s.record(ctx, req.GetRecordKey())
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("delete entry", err)
	}
	defer func() { _ = tx.Rollback() }()
	statement := fmt.Sprintf("DELETE FROM `%s` WHERE `id` = ? AND `revision` = ?", recordTableName(record.Name))
	result, err := tx.ExecContext(ctx, statement, req.GetId(), req.GetExpectedRevision())
	if err != nil {
		return nil, writeError("delete entry", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		_ = tx.Rollback()
		return nil, s.checkChanged(ctx, record, req.GetId(), result)
	}
	if err := recordHistory(ctx, tx, record.Name, req.GetId(), req.GetExpectedRevision(), "deleted", nil); err != nil {
		return nil, internalError("record history", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("delete entry", err)
	}
	return &athenav1.DeleteEntryResponse{}, nil
}

func (s entryService) ListEntryHistory(ctx context.Context, req *athenav1.ListEntryHistoryRequest) (*athenav1.ListEntryHistoryResponse, error) {
	record, err := s.record(ctx, req.GetRecordKey())
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT action, revision, changed_fields, actor, created_at FROM athena_entry_history WHERE record_name = ? AND entry_id = ? ORDER BY created_at DESC, id DESC LIMIT 200`, record.Name, req.GetId())
	if err != nil {
		return nil, internalError("read history", err)
	}
	defer rows.Close()
	res := &athenav1.ListEntryHistoryResponse{}
	for rows.Next() {
		var event athenav1.EntryEvent
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
	// Entries written before history existed: describe what the row itself records.
	entry, err := s.load(ctx, record, req.GetId())
	if err != nil {
		return nil, err
	}
	if entry.GetUpdatedAt() != entry.GetCreatedAt() || entry.GetRevision() > 1 {
		res.Events = append(res.Events, &athenav1.EntryEvent{Action: "updated", Revision: entry.GetRevision(), Actor: localActor, At: entry.GetUpdatedAt()})
	}
	res.Events = append(res.Events, &athenav1.EntryEvent{Action: "created", Revision: 1, Actor: localActor, At: entry.GetCreatedAt()})
	return res, nil
}

func recordHistory(ctx context.Context, tx *sql.Tx, recordName, entryID string, revision int64, action string, changed []string) error {
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
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_entry_history (id, record_name, entry_id, revision, action, changed_fields, actor) VALUES (?,?,?,?,?,?,?)`, id.String(), recordName, entryID, revision, action, payload, localActor)
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

// record loads a saved Record definition. Table and column names in later queries
// come only from this definition, never from the request.
func (s entryService) record(ctx context.Context, key string) (recordDefinition, error) {
	if s.db == nil {
		return recordDefinition{}, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT definition FROM athena_records WHERE name = ?`, key).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return recordDefinition{}, connect.NewError(connect.CodeNotFound, fmt.Sprintf("Record %q does not exist", key))
	}
	if err != nil {
		return recordDefinition{}, internalError("load Record", err)
	}
	var record recordDefinition
	if err := json.Unmarshal(payload, &record); err != nil {
		return recordDefinition{}, internalError("decode Record", err)
	}
	return record, nil
}

func (s entryService) load(ctx context.Context, record recordDefinition, id string) (*athenav1.Entry, error) {
	return loadEntry(ctx, s.db, record, id, "")
}

// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// loadEntry reads one entry. suffix is "" or "FOR UPDATE" inside a transaction.
func loadEntry(ctx context.Context, q querier, record recordDefinition, id, suffix string) (*athenav1.Entry, error) {
	query := fmt.Sprintf("SELECT %s FROM `%s` WHERE `id` = ? %s", selectColumns(record), recordTableName(record.Name), suffix)
	rows, err := q.QueryContext(ctx, query, id)
	if err != nil {
		return nil, internalError("read entry", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, internalError("read entry", err)
		}
		return nil, connect.NewError(connect.CodeNotFound, "the entry does not exist")
	}
	entry, err := scanEntry(rows, record)
	if err != nil {
		return nil, internalError("read entry", err)
	}
	return entry, nil
}

// checkChanged tells a missing entry apart from a revision conflict after a guarded write.
func (s entryService) checkChanged(ctx context.Context, record recordDefinition, id string, result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return internalError("check write", err)
	}
	if affected > 0 {
		return nil
	}
	var exists bool
	statement := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s` WHERE `id` = ?)", recordTableName(record.Name))
	if err := s.db.QueryRowContext(ctx, statement, id).Scan(&exists); err != nil {
		return internalError("check write", err)
	}
	if !exists {
		return connect.NewError(connect.CodeNotFound, "the entry does not exist")
	}
	return connect.NewError(connect.CodeAborted, "someone else changed this entry; reload it and try again")
}

func selectColumns(record recordDefinition) string {
	columns := []string{"`id`", "`revision`", "`created_at`", "`updated_at`"}
	for _, field := range record.Fields {
		columns = append(columns, "`"+field.Name+"`")
	}
	return strings.Join(columns, ",")
}

func scanEntry(rows *sql.Rows, record recordDefinition) (*athenav1.Entry, error) {
	var entry athenav1.Entry
	var created, updated time.Time
	targets := []any{&entry.Id, &entry.Revision, &created, &updated}
	values := make([]any, len(record.Fields))
	for i, field := range record.Fields {
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
	entry.CreatedAt = created.UTC().Format(time.RFC3339)
	entry.UpdatedAt = updated.UTC().Format(time.RFC3339)
	entry.Values = make(map[string]*athenav1.FieldValue, len(record.Fields))
	for i, field := range record.Fields {
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
		entry.Values[field.Name] = value
	}
	return &entry, nil
}

// trimDecimal removes the trailing zeros MariaDB pads DECIMAL(18,6) values with.
func trimDecimal(text string) string {
	if !strings.Contains(text, ".") {
		return text
	}
	return strings.TrimSuffix(strings.TrimRight(text, "0"), ".")
}

// bindValues validates request values against the Record's fields and returns
// quoted column names with matching bound arguments. Creating requires every
// required field; updating checks only the fields sent.
func bindValues(record recordDefinition, values map[string]*athenav1.FieldValue, creating bool) ([]string, []any, error) {
	fields := make(map[string]recordField, len(record.Fields))
	for _, field := range record.Fields {
		fields[field.Name] = field
	}
	for key := range values {
		if _, ok := fields[key]; !ok {
			return nil, nil, invalid(fmt.Sprintf("Record %q has no field %q", record.Name, key))
		}
	}
	var columns []string
	var args []any
	for _, field := range record.Fields {
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
		columns = append(columns, "`"+field.Name+"`")
		args = append(args, arg)
	}
	if !creating && len(columns) == 0 {
		return nil, nil, invalid("no field values to update")
	}
	return columns, args, nil
}

// fieldArgument converts one FieldValue into a database argument, or nil for null.
func fieldArgument(field recordField, value *athenav1.FieldValue) (any, error) {
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
			return nil, invalid(fmt.Sprintf("%s must be an entry ID", field.Label))
		}
	}
	return text, nil
}

type cursor struct {
	Record string `json:"r"`
	After  string `json:"a"`
}

func encodeCursor(record, after string) string {
	payload, _ := json.Marshal(cursor{Record: record, After: after})
	return base64.RawURLEncoding.EncodeToString(payload)
}

// decodeCursor rejects a token issued for a different Record.
func decodeCursor(token, record string) (string, error) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	var c cursor
	if err != nil || json.Unmarshal(payload, &c) != nil || c.Record != record {
		return "", invalid("page_token is not valid for this Record")
	}
	if _, err := uuid.Parse(c.After); err != nil {
		return "", invalid("page_token is not valid for this Record")
	}
	return c.After, nil
}

func invalid(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, message)
}

// internalError logs the cause on the server and hides driver detail from the client.
func internalError(action string, err error) error {
	log.Printf("entry service: %s: %v", action, err)
	return connect.NewError(connect.CodeInternal, "could not "+action)
}

// writeError maps MariaDB constraint failures to client errors.
func writeError(action string, err error) error {
	var dbErr *mysql.MySQLError
	if errors.As(err, &dbErr) {
		switch dbErr.Number {
		case 1452:
			return invalid("a linked entry does not exist")
		case 1451:
			return connect.NewError(connect.CodeFailedPrecondition, "other entries link to this entry")
		case 1048, 1364:
			return invalid("a required field is missing")
		case 1406, 1264, 1366:
			return invalid("a value does not fit its field")
		}
	}
	return internalError(action, err)
}
