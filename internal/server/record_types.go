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

// recordTypeDefinition is the JSON form stored in athena_record_types.definition.
type recordTypeDefinition struct {
	Name        string            `json:"name"`
	Label       string            `json:"label"`
	Description string            `json:"description"`
	Fields      []recordTypeField `json:"fields"`
}

type recordTypeField struct {
	Name             string `json:"name"`
	Label            string `json:"label"`
	Type             string `json:"type"`
	Required         bool   `json:"required"`
	TargetRecordType string `json:"target_record_type,omitempty"`
}

// recordTypeKeyPattern caps a key at 50 characters, so the table name
// p_workspace__<key> (13 characters of prefix) fits the 63-character limit
// without shortening (ADR-0009).
var (
	recordTypeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
	fieldKeyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

// reservedColumns are system columns present in every Record Type table.
var reservedColumns = map[string]bool{"id": true, "revision": true, "created_at": true, "updated_at": true}

// Operation actions and states in athena_record_type_operations.
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

// workspacePack is the reserved Pack name for Record Types authored in the
// Workspace itself, before any Pack exists.
const workspacePack = "workspace"

// recordTypeTableName names the table of a Record Type: p_<pack>__<key>,
// lowercase, shortened deterministically at the 63-byte limit every engine
// accepts.
func recordTypeTableName(key string) string {
	return store.ShortenIdentifier("p_"+workspacePack+"__"+strings.ToLower(key), store.GeneratedIdentLimit)
}

// sqlConn is satisfied by *sql.Conn, the single session that holds the schema lock.
type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// recordTypeService implements athena.v1.RecordTypeService. It has no authentication
// yet, so the server must stay bound to loopback.
type recordTypeService struct {
	db *sql.DB
}

func (s recordTypeService) ListRecordTypes(ctx context.Context, _ *athenav1.ListRecordTypesRequest) (*athenav1.ListRecordTypesResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT definition, revision FROM athena_record_types ORDER BY label`)
	if err != nil {
		return nil, internalError("list Record Types", err)
	}
	defer rows.Close()
	res := &athenav1.ListRecordTypesResponse{}
	for rows.Next() {
		var payload []byte
		var revision int64
		if err := rows.Scan(&payload, &revision); err != nil {
			return nil, internalError("read Record Type", err)
		}
		var recordType recordTypeDefinition
		if err := json.Unmarshal(payload, &recordType); err != nil {
			return nil, internalError("decode Record Type", err)
		}
		res.RecordTypes = append(res.RecordTypes, recordTypeToProto(recordType, revision))
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("list Record Types", err)
	}
	return res, nil
}

func (s recordTypeService) SaveRecordType(ctx context.Context, req *athenav1.SaveRecordTypeRequest) (*athenav1.SaveRecordTypeResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if req.GetRecordType() == nil {
		return nil, invalid("record type is required")
	}
	recordType := recordTypeFromProto(req.GetRecordType())
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

	previous, revision, err := loadRecordType(ctx, conn, recordType.Name)
	if err != nil {
		return nil, internalError("load Record Type", err)
	}
	if revision != req.GetExpectedRevision() {
		if req.GetExpectedRevision() == 0 {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Sprintf("a Record Type with the key %q already exists", recordType.Name))
		}
		return nil, connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	}
	if err := validateRecordType(ctx, conn, recordType); err != nil {
		return nil, err
	}
	steps, err := planMigration(ctx, conn, recordType, previous)
	if err != nil {
		return nil, err
	}
	opts, err := checkPlan(steps, req.GetConfirmDataLoss(), req.GetBackfills())
	if err != nil {
		return nil, err
	}
	operationID, err := startOperation(ctx, conn, actionSave, recordType, revision, opts)
	if err != nil {
		return nil, internalError("record type save operation", err)
	}
	log.Printf("record type save %s: %s", operationID, describeSteps(steps))
	saved, err := runOperation(ctx, conn, operationID, recordType, revision, opts)
	if err != nil {
		return nil, err
	}
	return &athenav1.SaveRecordTypeResponse{RecordType: recordTypeToProto(recordType, saved)}, nil
}

func (s recordTypeService) DeleteRecordType(ctx context.Context, req *athenav1.DeleteRecordTypeRequest) (*athenav1.DeleteRecordTypeResponse, error) {
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

	recordType, revision, err := loadRecordType(ctx, conn, req.GetKey())
	if err != nil {
		return nil, internalError("load Record Type", err)
	}
	if recordType == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Sprintf("Record Type %q does not exist", req.GetKey()))
	}
	if revision != req.GetExpectedRevision() {
		return nil, connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	}
	linked, err := linkingRecordTypes(ctx, conn, recordType.Name)
	if err != nil {
		return nil, internalError("check links", err)
	}
	if len(linked) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s cannot be deleted because these Record Types link to it: %s. Remove those Link fields first", recordType.Label, strings.Join(linked, ", ")))
	}
	records, err := countRecords(ctx, conn, recordType.Name)
	if err != nil {
		return nil, internalError("count records", err)
	}
	if records > 0 && !req.GetDeleteRecords() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s has %s; confirm that they should be deleted too", recordType.Label, plural(records, "record", "records")))
	}
	operationID, err := startOperation(ctx, conn, actionDelete, *recordType, revision, saveOptions{})
	if err != nil {
		return nil, internalError("record type delete operation", err)
	}
	if _, err := runDelete(ctx, conn, operationID, *recordType, revision, saveOptions{}); err != nil {
		return nil, err
	}
	return &athenav1.DeleteRecordTypeResponse{}, nil
}

// countRecords returns the number of rows in a Record Type's table, or zero when
// the table does not exist.
func countRecords(ctx context.Context, conn sqlConn, key string) (int64, error) {
	table := recordTypeTableName(key)
	exists, err := dialect.TableExists(ctx, conn, table)
	if err != nil || !exists {
		return 0, err
	}
	var count int64
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+dialect.QuoteIdent(table)).Scan(&count)
	return count, err
}

// linkingRecordTypes returns the labels of other Record Types with a Link field to key.
func linkingRecordTypes(ctx context.Context, conn sqlConn, key string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT definition FROM athena_record_types WHERE name <> ?`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var payload []byte
		var other recordTypeDefinition
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &other); err != nil {
			return nil, err
		}
		for _, field := range other.Fields {
			if field.Type == "Link" && field.TargetRecordType == key {
				labels = append(labels, other.Label)
				break
			}
		}
	}
	return labels, rows.Err()
}

// runDelete drops the Record Type's table, then removes the definition with a
// compare-and-swap on the revision. DROP TABLE IF EXISTS is safe to repeat,
// so an interrupted delete resumes from the start.
func runDelete(ctx context.Context, conn sqlConn, id string, recordType recordTypeDefinition, expected int64, _ saveOptions) (int64, error) {
	if _, err := conn.ExecContext(ctx, dialect.DropTable(recordTypeTableName(recordType.Name))); err != nil {
		log.Printf("record type delete %s: drop table for %q: %v", id, recordType.Name, err)
		failOperation(conn, id, "drop_table", err)
		return 0, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("could not delete the table for %s (operation %s)", recordType.Label, id))
	}
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_type_operations SET last_step = 'table_dropped' WHERE id = ?`, id); err != nil {
		return 0, internalError("record type delete progress", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, internalError("delete Record Type", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM athena_record_types WHERE name = ? AND revision = ?`, recordType.Name, expected)
	if err != nil {
		return 0, internalError("delete Record Type", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		failOperation(conn, id, "remove_definition", errors.New("revision changed"))
		return 0, connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE athena_record_type_operations SET status = ?, last_step = 'definition_removed' WHERE id = ?`, operationCompleted, id); err != nil {
		return 0, internalError("delete Record Type", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, internalError("delete Record Type", err)
	}
	return 0, nil
}

// lockSchema serializes Record Type saves for the current database.
func lockSchema(ctx context.Context, conn sqlConn) (func(), error) {
	acquired, release, err := dialect.LockSchema(ctx, conn)
	if err != nil {
		return nil, internalError("lock schema", err)
	}
	if !acquired {
		return nil, connect.NewError(connect.CodeUnavailable, "another Record Type save is in progress; try again")
	}
	return release, nil
}

// loadRecordType returns the active definition and revision, or nil and zero when
// the Record Type has never been saved.
func loadRecordType(ctx context.Context, conn sqlConn, name string) (*recordTypeDefinition, int64, error) {
	var payload []byte
	var revision int64
	err := conn.QueryRowContext(ctx, `SELECT definition, revision FROM athena_record_types WHERE name = ?`, name).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var recordType recordTypeDefinition
	if err := json.Unmarshal(payload, &recordType); err != nil {
		return nil, 0, err
	}
	return &recordType, revision, nil
}

// validateRecordType checks a definition on its own before any storage changes.
// Changes against the active revision are planned by planMigration.
func validateRecordType(ctx context.Context, conn sqlConn, recordType recordTypeDefinition) error {
	if !recordTypeKeyPattern.MatchString(recordType.Name) {
		return invalid("the Record Type key must start with a letter and use up to 50 lowercase letters, digits or underscores")
	}
	if recordType.Label == "" || utf8.RuneCountInString(recordType.Label) > 255 {
		return invalid("the Record Type name is required and must be at most 255 characters")
	}
	if utf8.RuneCountInString(recordType.Description) > 2000 {
		return invalid("the description must be at most 2000 characters")
	}
	if len(recordType.Fields) == 0 {
		return invalid("add at least one field")
	}
	seen := map[string]bool{}
	for _, field := range recordType.Fields {
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
			if field.TargetRecordType == "" {
				return invalid(fmt.Sprintf("%s needs a Record Type to link to", field.Label))
			}
			var exists bool
			if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM athena_record_types WHERE name = ?)`, field.TargetRecordType).Scan(&exists); err != nil {
				return internalError("check link target", err)
			}
			if !exists {
				return invalid(fmt.Sprintf("%s links to %q, which is not a saved Record Type", field.Label, field.TargetRecordType))
			}
		} else if field.TargetRecordType != "" {
			return invalid(fmt.Sprintf("%s is not a Link field and cannot have a link target", field.Label))
		}
	}
	return nil
}

// startOperation journals the operation and its definition before touching the schema.
func startOperation(ctx context.Context, conn sqlConn, action string, recordType recordTypeDefinition, expected int64, opts saveOptions) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(recordType)
	if err != nil {
		return "", err
	}
	options, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO athena_record_type_operations (id, record_type_name, action, expected_revision, definition, options, status, last_step) VALUES (?,?,?,?,?,?,?,'planned')`,
		id.String(), recordType.Name, action, expected, payload, options, operationPending)
	return id.String(), err
}

// runOperation applies the schema, then activates the definition with a
// compare-and-swap on the revision. MariaDB commits each DDL statement on its
// own, so applySchema is idempotent and a retry continues where it stopped.
func runOperation(ctx context.Context, conn sqlConn, id string, recordType recordTypeDefinition, expected int64, opts saveOptions) (int64, error) {
	// The active definition is still the previous revision until activation,
	// including when an interrupted operation is resumed.
	previous, _, err := loadRecordType(ctx, conn, recordType.Name)
	if err != nil {
		return 0, internalError("load Record Type", err)
	}
	if err := applySchema(ctx, conn, recordType, previous, opts); err != nil {
		log.Printf("record type save %s: apply schema for %q: %v", id, recordType.Name, err)
		failOperation(conn, id, "schema", err)
		return 0, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("could not update the table for %s; the save was not applied (operation %s)", recordType.Label, id))
	}
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_type_operations SET last_step = 'schema_applied' WHERE id = ?`, id); err != nil {
		return 0, internalError("record type save progress", err)
	}
	payload, err := json.Marshal(recordType)
	if err != nil {
		return 0, internalError("encode Record Type", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, internalError("activate Record Type", err)
	}
	defer func() { _ = tx.Rollback() }()
	conflict := connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO athena_record_types (name, label, definition, revision) VALUES (?,?,?,1)`, recordType.Name, recordType.Label, payload)
		if dialect.Classify(err) == store.ErrDuplicateKey {
			failOperation(conn, id, "activate", err)
			return 0, conflict
		}
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE athena_record_types SET label = ?, definition = ?, revision = revision + 1 WHERE name = ? AND revision = ?`, recordType.Label, payload, recordType.Name, expected)
		if err == nil {
			if affected, _ := result.RowsAffected(); affected == 0 {
				failOperation(conn, id, "activate", errors.New("revision changed"))
				return 0, conflict
			}
		}
	}
	if err != nil {
		return 0, internalError("activate Record Type", err)
	}
	next := expected + 1
	if _, err := tx.ExecContext(ctx, `UPDATE athena_record_type_operations SET status = ?, last_step = 'activated', target_revision = ? WHERE id = ?`, operationCompleted, next, id); err != nil {
		return 0, internalError("activate Record Type", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, internalError("activate Record Type", err)
	}
	return next, nil
}

func failOperation(conn sqlConn, id, step string, cause error) {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	if _, err := conn.ExecContext(context.Background(), `UPDATE athena_record_type_operations SET status = ?, last_step = ?, error = ? WHERE id = ?`, operationFailed, step, message, id); err != nil {
		log.Printf("record type save %s: mark failed: %v", id, err)
	}
}

type pendingOperation struct {
	id         string
	action     string
	options    saveOptions
	expected   int64
	recordType recordTypeDefinition
}

// resumeOperations finishes saves interrupted by a crash or restart. An
// operation whose Record Type has since moved to another revision is marked failed.
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
		_, revision, err := loadRecordType(ctx, conn, op.recordType.Name)
		if err != nil {
			return err
		}
		if revision != op.expected {
			failOperation(conn, op.id, "resume", fmt.Errorf("record type is at revision %d, operation expected %d", revision, op.expected))
			log.Printf("record type save %s: superseded, not resumed", op.id)
			continue
		}
		run := runOperation
		if op.action == actionDelete {
			run = runDelete
		}
		if _, err := run(ctx, conn, op.id, op.recordType, op.expected, op.options); err != nil {
			log.Printf("record type save %s: resume failed: %v", op.id, err)
			continue
		}
		log.Printf("record type %s %s: resumed and completed %q", op.action, op.id, op.recordType.Name)
	}
	return nil
}

func pendingOperations(ctx context.Context, conn sqlConn) ([]pendingOperation, error) {
	rows, err := conn.QueryContext(ctx, `SELECT id, action, expected_revision, definition, COALESCE(options, '{}') FROM athena_record_type_operations WHERE status = ? ORDER BY id`, operationPending)
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
		if err := json.Unmarshal(payload, &op.recordType); err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	return operations, rows.Err()
}

// applySchema creates the Record Type's table if needed, then applies field
// changes from the previous revision. Every statement is safe to repeat.
// Field columns are nullable; Go enforces required fields.
func applySchema(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, previous *recordTypeDefinition, opts saveOptions) error {
	table := recordTypeTableName(recordType.Name)
	columns := make([]store.Column, 0, len(recordType.Fields))
	for _, field := range recordType.Fields {
		columnType, err := dialect.ColumnType(field.Type)
		if err != nil {
			return err
		}
		columns = append(columns, store.Column{Name: field.Name, Type: columnType})
	}
	statements := []string{dialect.CreateRecordTypeTable(table, columns), dialect.AddRevisionColumn(table)}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return migrateFields(ctx, conn, recordType, previous, opts)
}

// ensureLink adds the foreign key for a Link field, or confirms that an
// existing constraint already points at the intended table.
func ensureLink(ctx context.Context, conn sqlConn, recordTypeName string, field recordTypeField) error {
	table := recordTypeTableName(recordTypeName)
	target := recordTypeTableName(field.TargetRecordType)
	constraint := recordTypeForeignKeyName(recordTypeName, field.Name)
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

func recordTypeForeignKeyName(recordTypeName, fieldName string) string {
	sum := sha256.Sum256([]byte(recordTypeName + ":" + fieldName))
	return fmt.Sprintf("fk_rtype_%x", sum[:8])
}

func recordTypeFromProto(recordType *athenav1.RecordType) recordTypeDefinition {
	definition := recordTypeDefinition{Name: recordType.GetKey(), Label: strings.TrimSpace(recordType.GetLabel()), Description: strings.TrimSpace(recordType.GetDescription())}
	for _, field := range recordType.GetFields() {
		definition.Fields = append(definition.Fields, recordTypeField{Name: field.GetKey(), Label: strings.TrimSpace(field.GetLabel()), Type: field.GetType(), Required: field.GetRequired(), TargetRecordType: field.GetTargetRecordTypeKey()})
	}
	return definition
}

func recordTypeToProto(recordType recordTypeDefinition, revision int64) *athenav1.RecordType {
	result := &athenav1.RecordType{Key: recordType.Name, Label: recordType.Label, Description: recordType.Description, Revision: revision}
	for _, field := range recordType.Fields {
		result.Fields = append(result.Fields, &athenav1.RecordTypeField{Key: field.Name, Label: field.Label, Type: field.Type, Required: field.Required, TargetRecordTypeKey: field.TargetRecordType})
	}
	return result
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
