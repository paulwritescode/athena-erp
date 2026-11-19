package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode/utf8"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/store"
	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

// recordDefinition is the JSON form stored in athena_records.definition.
type recordDefinition struct {
	Name        string        `json:"name"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	Fields      []recordField `json:"fields"`
}

type recordField struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	TargetRecord string `json:"target_record,omitempty"`
}

// recordKeyPattern leaves room for the athena_record_ prefix inside MariaDB's
// 64-character identifier limit.
var (
	recordKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
	fieldKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

// reservedColumns are system columns present in every Record table.
var reservedColumns = map[string]bool{"id": true, "revision": true, "created_at": true, "updated_at": true}

// Operation actions and states in athena_record_operations.
const (
	actionSave         = "save"
	actionDelete       = "delete"
	operationPending   = "pending"
	operationCompleted = "completed"
	operationFailed    = "failed"
)

// dialect generates every engine-specific statement. MariaDB is the only
// implementation.
var dialect store.Dialect = store.MariaDB{}

func recordTableName(name string) string {
	return dialect.ShortenIdent("athena_record_" + strings.ToLower(name))
}

// sqlConn is satisfied by *sql.Conn, the single session that holds the schema lock.
type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// recordService implements athena.v1.RecordService. It has no authentication
// yet, so the server must stay bound to loopback.
type recordService struct {
	db *sql.DB
}

func (s recordService) ListRecords(ctx context.Context, _ *athenav1.ListRecordsRequest) (*athenav1.ListRecordsResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT definition, revision FROM athena_records ORDER BY label`)
	if err != nil {
		return nil, internalError("list Records", err)
	}
	defer rows.Close()
	res := &athenav1.ListRecordsResponse{}
	for rows.Next() {
		var payload []byte
		var revision int64
		if err := rows.Scan(&payload, &revision); err != nil {
			return nil, internalError("read Record", err)
		}
		var record recordDefinition
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, internalError("decode Record", err)
		}
		res.Records = append(res.Records, recordToProto(record, revision))
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("list Records", err)
	}
	return res, nil
}

func (s recordService) SaveRecord(ctx context.Context, req *athenav1.SaveRecordRequest) (*athenav1.SaveRecordResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if req.GetRecord() == nil {
		return nil, invalid("record is required")
	}
	record := recordFromProto(req.GetRecord())
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, internalError("open connection", err)
	}
	defer conn.Close()
	unlock, err := lockSchema(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer unlock()

	previous, revision, err := loadRecord(ctx, conn, record.Name)
	if err != nil {
		return nil, internalError("load Record", err)
	}
	if revision != req.GetExpectedRevision() {
		if req.GetExpectedRevision() == 0 {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Sprintf("a Record with the key %q already exists", record.Name))
		}
		return nil, connect.NewError(connect.CodeAborted, "this Record was saved somewhere else; reload it and try again")
	}
	if err := validateRecord(ctx, conn, record); err != nil {
		return nil, err
	}
	steps, err := planMigration(ctx, conn, record, previous)
	if err != nil {
		return nil, err
	}
	opts, err := checkPlan(steps, req.GetConfirmDataLoss(), req.GetBackfills())
	if err != nil {
		return nil, err
	}
	operationID, err := startOperation(ctx, conn, actionSave, record, revision, opts)
	if err != nil {
		return nil, internalError("record save operation", err)
	}
	log.Printf("record save %s: %s", operationID, describeSteps(steps))
	saved, err := runOperation(ctx, conn, operationID, record, revision, opts)
	if err != nil {
		return nil, err
	}
	return &athenav1.SaveRecordResponse{Record: recordToProto(record, saved)}, nil
}

func (s recordService) DeleteRecord(ctx context.Context, req *athenav1.DeleteRecordRequest) (*athenav1.DeleteRecordResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, internalError("open connection", err)
	}
	defer conn.Close()
	unlock, err := lockSchema(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer unlock()

	record, revision, err := loadRecord(ctx, conn, req.GetKey())
	if err != nil {
		return nil, internalError("load Record", err)
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Sprintf("Record %q does not exist", req.GetKey()))
	}
	if revision != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "this Record was saved somewhere else; reload it and try again")
	}
	linked, err := linkingRecords(ctx, conn, record.Name)
	if err != nil {
		return nil, internalError("check links", err)
	}
	if len(linked) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s cannot be deleted because these Records link to it: %s. Remove those Link fields first", record.Label, strings.Join(linked, ", ")))
	}
	entries, err := countEntries(ctx, conn, record.Name)
	if err != nil {
		return nil, internalError("count entries", err)
	}
	if entries > 0 && !req.GetDeleteEntries() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s has %s; confirm that they should be deleted too", record.Label, plural(entries, "entry", "entries")))
	}
	operationID, err := startOperation(ctx, conn, actionDelete, *record, revision, saveOptions{})
	if err != nil {
		return nil, internalError("record delete operation", err)
	}
	if _, err := runDelete(ctx, conn, operationID, *record, revision, saveOptions{}); err != nil {
		return nil, err
	}
	return &athenav1.DeleteRecordResponse{}, nil
}

// countEntries returns the number of rows in a Record's table, or zero when
// the table does not exist.
func countEntries(ctx context.Context, conn sqlConn, key string) (int64, error) {
	table := recordTableName(key)
	exists, err := dialect.TableExists(ctx, conn, table)
	if err != nil || !exists {
		return 0, err
	}
	var count int64
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+dialect.QuoteIdent(table)).Scan(&count)
	return count, err
}

// linkingRecords returns the labels of other Records with a Link field to key.
func linkingRecords(ctx context.Context, conn sqlConn, key string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT definition FROM athena_records WHERE name <> ?`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var payload []byte
		var other recordDefinition
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &other); err != nil {
			return nil, err
		}
		for _, field := range other.Fields {
			if field.Type == "Link" && field.TargetRecord == key {
				labels = append(labels, other.Label)
				break
			}
		}
	}
	return labels, rows.Err()
}

// runDelete drops the Record's table, then removes the definition with a
// compare-and-swap on the revision. DROP TABLE IF EXISTS is safe to repeat,
// so an interrupted delete resumes from the start.
func runDelete(ctx context.Context, conn sqlConn, id string, record recordDefinition, expected int64, _ saveOptions) (int64, error) {
	if _, err := conn.ExecContext(ctx, dialect.DropTable(recordTableName(record.Name))); err != nil {
		log.Printf("record delete %s: drop table for %q: %v", id, record.Name, err)
		failOperation(conn, id, "drop_table", err)
		return 0, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("could not delete the table for %s (operation %s)", record.Label, id))
	}
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_operations SET last_step = 'table_dropped' WHERE id = ?`, id); err != nil {
		return 0, internalError("record delete progress", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, internalError("delete Record", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM athena_records WHERE name = ? AND revision = ?`, record.Name, expected)
	if err != nil {
		return 0, internalError("delete Record", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		failOperation(conn, id, "remove_definition", errors.New("revision changed"))
		return 0, connect.NewError(connect.CodeAborted, "this Record was saved somewhere else; reload it and try again")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_record_operations SET status = ?, last_step = 'definition_removed' WHERE id = ?`, operationCompleted, id); err != nil {
		return 0, internalError("delete Record", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, internalError("delete Record", err)
	}
	return 0, nil
}

// lockSchema serializes Record saves for the current database.
func lockSchema(ctx context.Context, conn sqlConn) (func(), error) {
	acquired, release, err := dialect.LockSchema(ctx, conn)
	if err != nil {
		return nil, internalError("lock schema", err)
	}
	if !acquired {
		return nil, connect.NewError(connect.CodeUnavailable, "another Record save is in progress; try again")
	}
	return release, nil
}

// loadRecord returns the active definition and revision, or nil and zero when
// the Record has never been saved.
func loadRecord(ctx context.Context, conn sqlConn, name string) (*recordDefinition, int64, error) {
	var payload []byte
	var revision int64
	err := conn.QueryRowContext(ctx, `SELECT definition, revision FROM athena_records WHERE name = ?`, name).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var record recordDefinition
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, 0, err
	}
	return &record, revision, nil
}

// validateRecord checks a definition on its own before any storage changes.
// Changes against the active revision are planned by planMigration.
func validateRecord(ctx context.Context, conn sqlConn, record recordDefinition) error {
	if !recordKeyPattern.MatchString(record.Name) {
		return invalid("the Record key must start with a letter and use up to 50 lowercase letters, digits or underscores")
	}
	if record.Label == "" || utf8.RuneCountInString(record.Label) > 255 {
		return invalid("the Record name is required and must be at most 255 characters")
	}
	if utf8.RuneCountInString(record.Description) > 2000 {
		return invalid("the description must be at most 2000 characters")
	}
	if len(record.Fields) == 0 {
		return invalid("add at least one field")
	}
	seen := map[string]bool{}
	for _, field := range record.Fields {
		if field.Label == "" {
			return invalid("every field needs a label")
		}
		if !fieldKeyPattern.MatchString(field.Name) {
			return invalid(fmt.Sprintf("field key %q must start with a letter and use lowercase letters, digits or underscores", field.Name))
		}
		if reservedColumns[field.Name] {
			return invalid(fmt.Sprintf("field key %q is reserved", field.Name))
		}
		if seen[field.Name] {
			return invalid(fmt.Sprintf("field key %q is used twice", field.Name))
		}
		seen[field.Name] = true
		if _, err := dialect.ColumnType(field.Type); err != nil {
			return invalid(fmt.Sprintf("%s: %v", field.Label, err))
		}
		if field.Type == "Link" {
			if field.TargetRecord == "" {
				return invalid(fmt.Sprintf("%s needs a Record to link to", field.Label))
			}
			var exists bool
			if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM athena_records WHERE name = ?)`, field.TargetRecord).Scan(&exists); err != nil {
				return internalError("check link target", err)
			}
			if !exists {
				return invalid(fmt.Sprintf("%s links to %q, which is not a saved Record", field.Label, field.TargetRecord))
			}
		} else if field.TargetRecord != "" {
			return invalid(fmt.Sprintf("%s is not a Link field and cannot have a link target", field.Label))
		}
	}
	return nil
}

// startOperation journals the operation and its definition before touching the schema.
func startOperation(ctx context.Context, conn sqlConn, action string, record recordDefinition, expected int64, opts saveOptions) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	options, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO athena_record_operations (id, record_name, action, expected_revision, definition, options, status, last_step) VALUES (?,?,?,?,?,?,?,'planned')`,
		id.String(), record.Name, action, expected, payload, options, operationPending)
	return id.String(), err
}

// runOperation applies the schema, then activates the definition with a
// compare-and-swap on the revision. MariaDB commits each DDL statement on its
// own, so applySchema is idempotent and a retry continues where it stopped.
func runOperation(ctx context.Context, conn sqlConn, id string, record recordDefinition, expected int64, opts saveOptions) (int64, error) {
	// The active definition is still the previous revision until activation,
	// including when an interrupted operation is resumed.
	previous, _, err := loadRecord(ctx, conn, record.Name)
	if err != nil {
		return 0, internalError("load Record", err)
	}
	if err := applySchema(ctx, conn, record, previous, opts); err != nil {
		log.Printf("record save %s: apply schema for %q: %v", id, record.Name, err)
		failOperation(conn, id, "schema", err)
		return 0, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("could not update the table for %s; the save was not applied (operation %s)", record.Label, id))
	}
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_operations SET last_step = 'schema_applied' WHERE id = ?`, id); err != nil {
		return 0, internalError("record save progress", err)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return 0, internalError("encode Record", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, internalError("activate Record", err)
	}
	defer func() { _ = tx.Rollback() }()
	conflict := connect.NewError(connect.CodeAborted, "this Record was saved somewhere else; reload it and try again")
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO athena_records (name, label, definition, revision) VALUES (?,?,?,1)`, record.Name, record.Label, payload)
		if dialect.Classify(err) == store.ErrDuplicateKey {
			failOperation(conn, id, "activate", err)
			return 0, conflict
		}
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE athena_records SET label = ?, definition = ?, revision = revision + 1 WHERE name = ? AND revision = ?`, record.Label, payload, record.Name, expected)
		if err == nil {
			if affected, _ := result.RowsAffected(); affected == 0 {
				failOperation(conn, id, "activate", errors.New("revision changed"))
				return 0, conflict
			}
		}
	}
	if err != nil {
		return 0, internalError("activate Record", err)
	}
	next := expected + 1
	if _, err := tx.ExecContext(ctx, `UPDATE athena_record_operations SET status = ?, last_step = 'activated', target_revision = ? WHERE id = ?`, operationCompleted, next, id); err != nil {
		return 0, internalError("activate Record", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, internalError("activate Record", err)
	}
	return next, nil
}

func failOperation(conn sqlConn, id, step string, cause error) {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	if _, err := conn.ExecContext(context.Background(), `UPDATE athena_record_operations SET status = ?, last_step = ?, error = ? WHERE id = ?`, operationFailed, step, message, id); err != nil {
		log.Printf("record save %s: mark failed: %v", id, err)
	}
}

type pendingOperation struct {
	id       string
	action   string
	options  saveOptions
	expected int64
	record   recordDefinition
}

// resumeOperations finishes saves interrupted by a crash or restart. An
// operation whose Record has since moved to another revision is marked failed.
func resumeOperations(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	unlock, err := lockSchema(ctx, conn)
	if err != nil {
		return err
	}
	defer unlock()
	operations, err := pendingOperations(ctx, conn)
	if err != nil {
		return err
	}
	for _, op := range operations {
		_, revision, err := loadRecord(ctx, conn, op.record.Name)
		if err != nil {
			return err
		}
		if revision != op.expected {
			failOperation(conn, op.id, "resume", fmt.Errorf("record is at revision %d, operation expected %d", revision, op.expected))
			log.Printf("record save %s: superseded, not resumed", op.id)
			continue
		}
		run := runOperation
		if op.action == actionDelete {
			run = runDelete
		}
		if _, err := run(ctx, conn, op.id, op.record, op.expected, op.options); err != nil {
			log.Printf("record save %s: resume failed: %v", op.id, err)
			continue
		}
		log.Printf("record %s %s: resumed and completed %q", op.action, op.id, op.record.Name)
	}
	return nil
}

func pendingOperations(ctx context.Context, conn sqlConn) ([]pendingOperation, error) {
	rows, err := conn.QueryContext(ctx, `SELECT id, action, expected_revision, definition, COALESCE(options, '{}') FROM athena_record_operations WHERE status = ? ORDER BY id`, operationPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var operations []pendingOperation
	for rows.Next() {
		var op pendingOperation
		var payload, options []byte
		if err := rows.Scan(&op.id, &op.action, &op.expected, &payload, &options); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(options, &op.options); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &op.record); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

// applySchema creates the Record's table if needed, then applies field
// changes from the previous revision. Every statement is safe to repeat.
// Field columns are nullable; Go enforces required fields.
func applySchema(ctx context.Context, conn sqlConn, record recordDefinition, previous *recordDefinition, opts saveOptions) error {
	table := recordTableName(record.Name)
	columns := make([]store.Column, 0, len(record.Fields))
	for _, field := range record.Fields {
		columnType, err := dialect.ColumnType(field.Type)
		if err != nil {
			return err
		}
		columns = append(columns, store.Column{Name: field.Name, Type: columnType})
	}
	statements := []string{dialect.CreateRecordTable(table, columns), dialect.AddRevisionColumn(table)}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return migrateFields(ctx, conn, record, previous, opts)
}

// ensureLink adds the foreign key for a Link field, or confirms that an
// existing constraint already points at the intended table.
func ensureLink(ctx context.Context, conn sqlConn, recordName string, field recordField) error {
	table := recordTableName(recordName)
	target := recordTableName(field.TargetRecord)
	constraint := recordForeignKeyName(recordName, field.Name)
	referenced, found, err := dialect.ForeignKeyTarget(ctx, conn, table, constraint)
	if err != nil {
		return err
	}
	if found {
		if referenced != target {
			return fmt.Errorf("link %q already references %q", field.Name, referenced)
		}
		return nil
	}
	_, err = conn.ExecContext(ctx, dialect.AddForeignKey(table, constraint, field.Name, target))
	return err
}

func recordForeignKeyName(recordName, fieldName string) string {
	sum := sha256.Sum256([]byte(recordName + ":" + fieldName))
	return fmt.Sprintf("fk_record_%x", sum[:8])
}

func recordFromProto(record *athenav1.Record) recordDefinition {
	definition := recordDefinition{Name: record.GetKey(), Label: strings.TrimSpace(record.GetLabel()), Description: strings.TrimSpace(record.GetDescription())}
	for _, field := range record.GetFields() {
		definition.Fields = append(definition.Fields, recordField{Name: field.GetKey(), Label: strings.TrimSpace(field.GetLabel()), Type: field.GetType(), Required: field.GetRequired(), TargetRecord: field.GetTargetRecordKey()})
	}
	return definition
}

func recordToProto(record recordDefinition, revision int64) *athenav1.Record {
	result := &athenav1.Record{Key: record.Name, Label: record.Label, Description: record.Description, Revision: revision}
	for _, field := range record.Fields {
		result.Fields = append(result.Fields, &athenav1.RecordField{Key: field.Name, Label: field.Label, Type: field.Type, Required: field.Required, TargetRecordKey: field.TargetRecord})
	}
	return result
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
