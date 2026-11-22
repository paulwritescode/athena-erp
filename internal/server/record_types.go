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
	"time"
	"unicode/utf8"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/store"
	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

// recordTypeDefinition is the JSON form stored in athena_record_types.definition.
type recordTypeDefinition struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Kind is empty in definitions saved before kinds existed; kind() reads
	// that as master.
	Kind             string `json:"kind,omitempty"`
	Class            string `json:"class,omitempty"`
	ParentRecordType string `json:"parent_record_type,omitempty"`
	// Submittable and Amendable switch on the document lifecycle (spec 2.2, 5).
	Submittable bool `json:"submittable,omitempty"`
	Amendable   bool `json:"amendable,omitempty"`
	// NamingSeries, GapFree and AllowManualName configure document names
	// (spec 10.4, 24.3; ADR-0012). LabelField names the field that labels a
	// Record in pickers.
	NamingSeries    string `json:"naming_series,omitempty"`
	GapFree         bool   `json:"gap_free,omitempty"`
	AllowManualName bool   `json:"allow_manual_name,omitempty"`
	LabelField      string `json:"label_field,omitempty"`
	// Tree organises rows into a hierarchy through tree_parent_id; the
	// kernel stores tree_path and tree_depth. EffectiveDated gives rows an
	// end-exclusive effective range (ADR-0029).
	Tree           bool              `json:"tree,omitempty"`
	EffectiveDated bool              `json:"effective_dated,omitempty"`
	Fields         []recordTypeField `json:"fields"`
	// Layout arranges the form. Nil means definition order.
	Layout *formLayout `json:"layout,omitempty"`
	// Revision is the active revision. It is read from the revision column,
	// never stored in the definition.
	Revision int64 `json:"-"`
}

type recordTypeField struct {
	Name             string `json:"name"`
	Label            string `json:"label"`
	Type             string `json:"type"`
	Required         bool   `json:"required"`
	TargetRecordType string `json:"target_record_type,omitempty"`
	// Attributes enforced on every write (spec 3.2). Zero values mean unset.
	Unique    bool   `json:"unique,omitempty"`
	Default   string `json:"default,omitempty"`
	Min       string `json:"min,omitempty"`
	Max       string `json:"max,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SetOnce   bool   `json:"set_once,omitempty"`
	NoCopy    bool   `json:"no_copy,omitempty"`
	// AllowOnSubmit lets the field change after its document is submitted.
	AllowOnSubmit bool        `json:"allow_on_submit,omitempty"`
	Index         bool        `json:"index,omitempty"`
	Values        []enumValue `json:"values,omitempty"`
	Currency      string      `json:"currency,omitempty"`
	UoM           string      `json:"uom,omitempty"`
	Scale         *int        `json:"scale,omitempty"`
	// Expressions of the expression language (ADR-0013). Formula is stored
	// by the kernel on every write; the others are checked or hinted.
	Formula    string `json:"formula,omitempty"`
	RequiredIf string `json:"required_if,omitempty"`
	ReadOnlyIf string `json:"read_only_if,omitempty"`
	Validate   string `json:"validate,omitempty"`
	DependsOn  string `json:"depends_on,omitempty"`
	// PermissionLevel (0 to 9) hides the field from users whose Grants reach
	// a lower level (ADR-0014).
	PermissionLevel int `json:"permission_level,omitempty"`
}

// enumValue is one permanent choice of an Enum field.
type enumValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// recordTypeKeyPattern caps a key at 50 characters, so the table name
// p_workspace__<key> (13 characters of prefix) fits the 63-character limit
// without shortening (ADR-0009).
var (
	recordTypeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
	fieldKeyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

// reservedColumns are the system columns the kernel adds to Record Type tables
// (see systemColumns), reserved for every kind so a field never collides with
// one. revision is the spec's version field.
var reservedColumns = map[string]bool{"id": true, "revision": true, "created_at": true, "updated_at": true, "created_by": true, "modified_by": true, "owner": true, "deleted_at": true, "def_version": true, "request_id": true, "parent_id": true, "idx": true, "docstate": true, "amended_from": true, "tree_parent_id": true, "tree_path": true, "tree_depth": true, "effective_from": true, "effective_to": true}

// Operation actions and states in athena_record_type_operations.
const (
	actionSave         = "save"
	actionSaveGroup    = "save_group"
	actionDelete       = "delete"
	operationPending   = "pending"
	operationCompleted = "completed"
	operationFailed    = "failed"
	// operationCompensated marks a member save undone by pair compensation.
	// It completed and then was rolled back, so it is neither the last save
	// nor a failure of its own.
	operationCompensated = "compensated"
)

// saveGroup journals an atomic pair save: a child and its parent, applied both
// or neither. MariaDB commits each DDL statement on its own, so there is no
// single transaction; instead the group records everything compensation needs
// (the previous child definition, both revisions) and the member operation IDs
// so resume never runs a member as a lone save.
type saveGroup struct {
	Child          recordTypeDefinition  `json:"child"`
	ChildExpected  int64                 `json:"child_expected"`
	ChildPrevious  *recordTypeDefinition `json:"child_previous,omitempty"`
	ChildOp        string                `json:"child_op"`
	ParentName     string                `json:"parent_name"`
	ParentExpected int64                 `json:"parent_expected"`
	ParentOp       string                `json:"parent_op"`
}

// dialect generates every engine-specific statement. Each process sets it once
// from the endpoint driver when it opens the database (Serve, OpenKernel,
// openMigrationDB); tests must not flip it concurrently. PostgreSQL is
// implemented but untested and claims no parity.
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
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	builder := p == nil || p.HasSystem(auth.PermissionBuild)
	rows, err := s.db.QueryContext(ctx, `SELECT definition, revision FROM athena_record_types ORDER BY label`)
	if err != nil {
		return nil, internalError("list Record Types", err)
	}
	defer rows.Close()
	res := &athenav1.ListRecordTypesResponse{}
	var names []string
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
		// A builder sees every Record Type with every field. Anyone else sees
		// the Record Types a Grant lets them read, without the fields above
		// their permission level.
		if !builder {
			if !p.CanAny(recordType.Name) {
				continue
			}
			recordType = visibleDefinition(recordType, hiddenFields(p, recordType))
		}
		item := recordTypeToProto(recordType, revision)
		access := &athenav1.RecordTypeAccess{PermissionLevel: int32(levelOf(p, recordType.Name))}
		if p == nil {
			access.Actions = auth.Actions
		} else {
			access.Actions = p.Actions(recordType.Name)
		}
		item.Access = access
		res.RecordTypes = append(res.RecordTypes, item)
		names = append(names, recordType.Name)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("list Record Types", err)
	}
	if err := s.enrichRecordTypes(ctx, p, res.RecordTypes, names); err != nil {
		return nil, err
	}
	return res, nil
}

// enrichRecordTypes fills the list-only columns: last save time and actor
// from the journal, and Record counts for the Record Types the caller may
// read. PackKey stays empty until Packs install in C3.
func (s recordTypeService) enrichRecordTypes(ctx context.Context, p *auth.Principal, items []*athenav1.RecordType, names []string) error {
	if len(items) == 0 {
		return nil
	}
	saves, err := lastSaves(ctx, s.db)
	if err != nil {
		return internalError("read Record Type saves", err)
	}
	counts, err := recordCounts(ctx, s.db, p, items)
	if err != nil {
		return internalError("count Records", err)
	}
	for i, item := range items {
		if save, ok := saves[names[i]]; ok {
			item.UpdatedAt = save.at
			item.UpdatedBy = save.by
		}
		if count, ok := counts[names[i]]; ok {
			item.RecordCount = count
		} else {
			item.CountHidden = true
		}
	}
	return nil
}

// lastSave is when and by whom a Record Type was last saved.
type lastSave struct {
	at string
	by string
}

// lastSaves returns the latest completed save per Record Type. Member saves
// of a pair group that never completed are skipped, whatever their own
// status: their save was rolled back or never settled, so the surviving
// revision's actor and time win without any backfill. Ties on the journal's
// second-precision timestamp break on the time-ordered operation ID. Only
// the actor is read from the stored options; backfills may hold Record
// values and are never decoded here.
func lastSaves(ctx context.Context, db *sql.DB) (map[string]lastSave, error) {
	excluded, err := unsettledMemberOps(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT record_type_name, updated_at, options, id
		FROM athena_record_type_operations
		WHERE status = 'completed' AND action = 'save'
		ORDER BY record_type_name, updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	saves := map[string]lastSave{}
	for rows.Next() {
		var name string
		var at time.Time
		var options []byte
		var id string
		if err := rows.Scan(&name, &at, &options, &id); err != nil {
			return nil, err
		}
		if _, ok := saves[name]; ok || excluded[id] {
			continue
		}
		var actor struct {
			Actor string `json:"actor"`
		}
		if len(options) > 0 {
			if err := json.Unmarshal(options, &actor); err != nil {
				return nil, err
			}
		}
		saves[name] = lastSave{at: at.UTC().Format(time.RFC3339), by: actor.Actor}
	}
	return saves, rows.Err()
}

// unsettledMemberOps returns the operation IDs journaled as members of a
// pair group that never completed: pending mid-flight saves as well as
// failed and compensated groups, including groups written before member
// operations carried their own compensated mark.
func unsettledMemberOps(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT definition FROM athena_record_type_operations
		WHERE action = 'save_group' AND status != 'completed'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	excluded := map[string]bool{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var group saveGroup
		if err := json.Unmarshal(payload, &group); err != nil {
			return nil, err
		}
		excluded[group.ChildOp] = true
		excluded[group.ParentOp] = true
	}
	return excluded, rows.Err()
}

// recordCounts counts the live Records of the Record Types the caller may
// read without row conditions or Assignment scope, in one query. Names the
// caller may not read, or may read only in part, are absent from the map, so
// the caller sees 0 with count_hidden rather than a number that mixes rows
// they can and cannot open.
func recordCounts(ctx context.Context, db *sql.DB, p *auth.Principal, items []*athenav1.RecordType) (map[string]int64, error) {
	var targets []*athenav1.RecordType
	for _, item := range items {
		if p == nil {
			targets = append(targets, item)
			continue
		}
		if _, unrestricted := p.Scope(item.GetKey(), auth.ActionRead); unrestricted {
			targets = append(targets, item)
		}
	}
	if len(targets) == 0 {
		return map[string]int64{}, nil
	}
	return retryOnMissingTable(func(err error) bool {
		return dialect.Classify(err) == store.ErrMissingTable
	}, func() (map[string]int64, error) {
		existing := map[string]bool{}
		for _, item := range targets {
			ok, err := dialect.TableExists(ctx, db, recordTypeTableName(item.GetKey()))
			if err != nil {
				return nil, err
			}
			existing[item.GetKey()] = ok
		}
		return runCountQuery(ctx, db, targets, existing)
	})
}

// retryOnMissingTable runs op once more when it fails because a table
// vanished mid-flight, so a concurrent drop costs one retry instead of an
// error. Any other failure, or a second miss, is returned as is.
func retryOnMissingTable[T any](isMissing func(error) bool, op func() (T, error)) (T, error) {
	out, err := op()
	if err != nil && isMissing(err) {
		return op()
	}
	return out, err
}

// countMemberSQL builds one UNION member counting a Record Type's live
// Records. Placeholders stay ? here; the caller rebinds them through the
// dialect where the statement meets its arguments.
func countMemberSQL(d store.Dialect, key, kind string) string {
	member := "SELECT ? AS name, COUNT(*) AS n FROM " + d.QuoteIdent(recordTypeTableName(key))
	if clause := liveClause(d, recordTypeDefinition{Kind: kind}); clause != "" {
		member += " WHERE " + clause
	}
	return member
}

// runCountQuery counts each existing table once. Missing tables count 0:
// the definition outlived its table, which List still shows.
func runCountQuery(ctx context.Context, db *sql.DB, targets []*athenav1.RecordType, existing map[string]bool) (map[string]int64, error) {
	counts := map[string]int64{}
	var query strings.Builder
	var args []any
	for _, item := range targets {
		if !existing[item.GetKey()] {
			counts[item.GetKey()] = 0
			continue
		}
		if len(args) > 0 {
			query.WriteString(" UNION ALL ")
		}
		query.WriteString(countMemberSQL(dialect, item.GetKey(), item.GetKind()))
		args = append(args, item.GetKey())
	}
	if len(args) == 0 {
		return counts, nil
	}
	rows, err := db.QueryContext(ctx, dialect.Rebind(query.String()), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		counts[name] = count
	}
	return counts, rows.Err()
}

func (s recordTypeService) SaveRecordType(ctx context.Context, req *athenav1.SaveRecordTypeRequest) (*athenav1.SaveRecordTypeResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
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

	if req.GetParent() != nil {
		return s.savePair(ctx, conn, recordType, req)
	}
	saved, err := saveOne(ctx, conn, recordType, req.GetExpectedRevision(), req.GetConfirmDataLoss(), req.GetBackfills())
	if err != nil {
		return nil, err
	}
	return &athenav1.SaveRecordTypeResponse{RecordType: recordTypeToProto(recordType, saved)}, nil
}

// savePair saves a child and its parent both or neither. Both members are
// validated and planned before the group is journaled, so a bad parent never
// starts the child; if the parent then fails, the child is compensated.
func (s recordTypeService) savePair(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, req *athenav1.SaveRecordTypeRequest) (*athenav1.SaveRecordTypeResponse, error) {
	parent := recordTypeFromProto(req.GetParent())
	if recordType.kind() != kindChild || parent.Name != recordType.ParentRecordType {
		return nil, invalid("parent must be the Record Type that owns this child Record Type")
	}
	// Check the parent's expressions against the child as it will be,
	// before anything is saved.
	if _, err := compilePlan(ctx, conn, parent, &recordType); err != nil {
		return nil, invalid(fmt.Sprintf("%s: %s", parent.Label, connectMessage(err)))
	}
	child, err := prepareOne(ctx, conn, recordType, req.GetExpectedRevision(), req.GetConfirmDataLoss(), req.GetBackfills())
	if err != nil {
		return nil, err
	}
	child.opts.Actor = displayActor(ctx)
	// The parent prepares after the child commits: its validation reads the
	// child's saved definition. The revision is checked now so the journal
	// never records a parent the client did not see.
	if _, revision, err := loadRecordType(ctx, conn, parent.Name); err != nil {
		return nil, internalError("load Record Type", err)
	} else if revision != req.GetParent().GetRevision() {
		return nil, connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	}
	childOp, err := uuid.NewV7()
	if err != nil {
		return nil, internalError("record type save operation", err)
	}
	parentOp, err := uuid.NewV7()
	if err != nil {
		return nil, internalError("record type save operation", err)
	}
	group := saveGroup{
		Child: child.recordType, ChildExpected: child.revision, ChildPrevious: child.previous, ChildOp: childOp.String(),
		ParentName: parent.Name, ParentExpected: req.GetParent().GetRevision(), ParentOp: parentOp.String(),
	}
	groupID, err := startGroup(ctx, conn, group)
	if err != nil {
		return nil, internalError("record type save group", err)
	}
	saved, _, err := commitOne(ctx, conn, child, childOp.String())
	if err != nil {
		failGroup(conn, groupID, "child", err)
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_type_operations SET last_step = 'child_saved' WHERE id = ?`, groupID); err != nil {
		failGroup(conn, groupID, "child_saved", err)
		return nil, internalError("record type save group", err)
	}
	parentPrepared, err := prepareOne(ctx, conn, parent, req.GetParent().GetRevision(), req.GetConfirmDataLoss(), nil)
	if err != nil {
		return nil, s.abandonPair(ctx, conn, groupID, group, recordType.Label, parent.Label, err)
	}
	parentPrepared.opts.Actor = displayActor(ctx)
	parentSaved, _, err := commitOne(ctx, conn, parentPrepared, parentOp.String())
	if err != nil {
		return nil, s.abandonPair(ctx, conn, groupID, group, recordType.Label, parent.Label, err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_type_operations SET status = ?, last_step = 'completed' WHERE id = ?`, operationCompleted, groupID); err != nil {
		return nil, internalError("record type save group", err)
	}
	return &athenav1.SaveRecordTypeResponse{RecordType: recordTypeToProto(recordType, saved), Parent: recordTypeToProto(parent, parentSaved)}, nil
}

// abandonPair compensates the child after the parent failed to prepare or
// commit, and reports what happened to each member.
func (s recordTypeService) abandonPair(ctx context.Context, conn sqlConn, groupID string, group saveGroup, childLabel, parentLabel string, cause error) error {
	if cerr := compensateChildSave(ctx, conn, group); cerr != nil {
		failGroup(conn, groupID, "compensation_failed", cerr)
		return connect.NewError(connect.CodeOf(cause), fmt.Sprintf("%s was not saved, and rolling back %s failed: %s; the child save stands and needs an operator", parentLabel, childLabel, connectMessage(cerr)))
	}
	// The child save is undone: mark its member operation so the rolled-back
	// save never shows as anyone's last save.
	if _, err := conn.ExecContext(ctx, `UPDATE athena_record_type_operations SET status = ?, last_step = 'compensated' WHERE id = ?`, operationCompensated, group.ChildOp); err != nil {
		return internalError("record type save group", err)
	}
	failGroup(conn, groupID, "compensated", cause)
	return connect.NewError(connect.CodeOf(cause), fmt.Sprintf("%s was not saved, so %s was rolled back: %s", parentLabel, childLabel, connectMessage(cause)))
}

// preparedSave is one member validated and planned, ready to journal and run.
type preparedSave struct {
	recordType recordTypeDefinition
	previous   *recordTypeDefinition
	revision   int64
	opts       saveOptions
	steps      []migrationStep
}

// saveOne validates, plans and runs the save of one Record Type on the
// connection that holds the schema lock, and returns the new revision.
func saveOne(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, expected int64, confirm bool, backfills map[string]*athenav1.FieldValue) (int64, error) {
	prepared, err := prepareOne(ctx, conn, recordType, expected, confirm, backfills)
	if err != nil {
		return 0, err
	}
	prepared.opts.Actor = displayActor(ctx)
	saved, _, err := commitOne(ctx, conn, prepared, "")
	return saved, err
}

// prepareOne checks the revision, validates and plans without writing.
func prepareOne(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, expected int64, confirm bool, backfills map[string]*athenav1.FieldValue) (preparedSave, error) {
	previous, revision, err := loadRecordType(ctx, conn, recordType.Name)
	if err != nil {
		return preparedSave{}, internalError("load Record Type", err)
	}
	if revision != expected {
		if expected == 0 {
			return preparedSave{}, connect.NewError(connect.CodeAlreadyExists, fmt.Sprintf("a Record Type with the key %q already exists", recordType.Name))
		}
		return preparedSave{}, connect.NewError(connect.CodeAborted, "this Record Type was saved somewhere else; reload it and try again")
	}
	if err := validateRecordType(ctx, conn, recordType, previous); err != nil {
		return preparedSave{}, err
	}
	steps, err := planMigration(ctx, conn, recordType, previous)
	if err != nil {
		return preparedSave{}, err
	}
	opts, err := checkPlan(steps, confirm, backfills)
	if err != nil {
		return preparedSave{}, err
	}
	return preparedSave{recordType: recordType, previous: previous, revision: revision, opts: opts, steps: steps}, nil
}

// commitOne journals a save operation and runs it, returning the new revision
// and the operation ID. An empty operation ID generates one.
func commitOne(ctx context.Context, conn sqlConn, prepared preparedSave, operationID string) (int64, string, error) {
	if operationID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return 0, "", internalError("record type save operation", err)
		}
		operationID = id.String()
	}
	if err := startOperationWithID(ctx, conn, operationID, actionSave, prepared.recordType, prepared.revision, prepared.opts); err != nil {
		return 0, "", internalError("record type save operation", err)
	}
	log.Printf("record type save %s: %s", operationID, describeSteps(prepared.steps))
	saved, err := runOperation(ctx, conn, operationID, prepared.recordType, prepared.revision, prepared.opts)
	return saved, operationID, err
}

func (s recordTypeService) DeleteRecordType(ctx context.Context, req *athenav1.DeleteRecordTypeRequest) (*athenav1.DeleteRecordTypeResponse, error) {
	if s.db == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
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
	operationID, err := startOperation(ctx, conn, actionDelete, *recordType, revision, saveOptions{Actor: displayActor(ctx)})
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

// linkingRecordTypes returns the labels of other Record Types with a Link field to key, or that are children of it.
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
		if other.ParentRecordType == key {
			labels = append(labels, other.Label)
			continue
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
func loadRecordType(ctx context.Context, conn store.Querier, name string) (*recordTypeDefinition, int64, error) {
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
	recordType.Revision = revision
	return &recordType, revision, nil
}

// validateRecordType checks a definition on its own before any storage changes.
// Changes against the active revision are planned by planMigration.
func validateRecordType(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, previous *recordTypeDefinition) error {
	if !recordTypeKeyPattern.MatchString(recordType.Name) {
		return invalid("the Record Type key must start with a letter and use up to 50 lowercase letters, digits or underscores")
	}
	if recordType.Label == "" || utf8.RuneCountInString(recordType.Label) > 255 {
		return invalid("the Record Type name is required and must be at most 255 characters")
	}
	if utf8.RuneCountInString(recordType.Description) > 2000 {
		return invalid("the description must be at most 2000 characters")
	}
	if err := validateKind(ctx, conn, recordType, previous); err != nil {
		return err
	}
	if err := validateLifecycle(ctx, conn, recordType, previous); err != nil {
		return err
	}
	if err := validateTreeEffective(recordType); err != nil {
		return err
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
		if err := validateAttributes(field); err != nil {
			return err
		}
		if field.PermissionLevel < 0 || field.PermissionLevel > auth.MaxPermissionLevel {
			return invalid(fmt.Sprintf("%s: the permission level is 0 to %d", field.Label, auth.MaxPermissionLevel))
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
	if err := validateLayout(ctx, conn, recordType); err != nil {
		return err
	}
	return validateExpressions(ctx, conn, recordType)
}

// startOperation journals the operation and its definition before touching the schema.
func startOperation(ctx context.Context, conn sqlConn, action string, recordType recordTypeDefinition, expected int64, opts saveOptions) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), startOperationWithID(ctx, conn, id.String(), action, recordType, expected, opts)
}

// startOperationWithID journals under a caller-chosen ID, for group members
// whose IDs are recorded in the group before they run.
func startOperationWithID(ctx context.Context, conn sqlConn, id, action string, recordType recordTypeDefinition, expected int64, opts saveOptions) error {
	payload, err := json.Marshal(recordType)
	if err != nil {
		return err
	}
	options, err := json.Marshal(opts)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO athena_record_type_operations (id, record_type_name, action, expected_revision, definition, options, status, last_step) VALUES (?,?,?,?,?,?,?,'planned')`,
		id, recordType.Name, action, expected, payload, options, operationPending)
	return err
}

// startGroup journals a pair save before either member runs.
func startGroup(ctx context.Context, conn sqlConn, group saveGroup) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(group)
	if err != nil {
		return "", err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO athena_record_type_operations (id, record_type_name, action, expected_revision, definition, status, last_step) VALUES (?,?,?,?,?,?,'planned')`,
		id.String(), group.Child.Name, actionSaveGroup, group.ChildExpected, payload, operationPending)
	return id.String(), err
}

// failGroup marks a group failed with its cause.
func failGroup(conn sqlConn, id, step string, cause error) {
	failOperation(conn, id, step, cause)
}

// compensateChildSave undoes a committed child save after its parent failed.
// A new child loses its table and metadata row; an updated child gets its
// previous definition and revision back with its schema migrated back. Every
// statement repeats safely, so a crash during compensation resumes it. When
// the child never committed, or someone else saved past it, there is nothing
// safe to undo.
func compensateChildSave(ctx context.Context, conn sqlConn, group saveGroup) error {
	current, revision, err := loadRecordType(ctx, conn, group.Child.Name)
	if err != nil {
		return err
	}
	if revision == group.ChildExpected {
		return nil
	}
	if revision != group.ChildExpected+1 {
		return fmt.Errorf("%s is at revision %d, past the rolled-back save; leaving it for an operator", group.Child.Label, revision)
	}
	table := recordTypeTableName(group.Child.Name)
	if group.ChildPrevious == nil {
		var count int64
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+dialect.QuoteIdent(table)).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%s holds %d Records written after its save; leaving it for an operator", group.Child.Label, count)
		}
		if _, err := conn.ExecContext(ctx, dialect.DropTable(table)); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM athena_record_types WHERE name = ? AND revision = ?`, group.Child.Name, revision); err != nil {
			return err
		}
		return nil
	}
	prior := *group.ChildPrevious
	if err := applySchema(ctx, conn, prior, current, saveOptions{ConfirmDataLoss: true}); err != nil {
		return err
	}
	payload, err := json.Marshal(prior)
	if err != nil {
		return err
	}
	result, err := conn.ExecContext(ctx, `UPDATE athena_record_types SET label = ?, definition = ?, revision = ? WHERE name = ? AND revision = ?`,
		prior.Label, payload, group.ChildExpected, group.Child.Name, revision)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("%s changed during rollback; leaving it for an operator", group.Child.Label)
	}
	return nil
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
	group      *saveGroup
}

// resumeOperations finishes saves interrupted by a crash or restart. An
// operation whose Record Type has since moved to another revision is marked failed.
// A pending group rolls its pair back: members never resume as lone saves.
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
	members := map[string]bool{}
	for _, op := range operations {
		if op.group != nil {
			members[op.group.ChildOp] = true
			members[op.group.ParentOp] = true
		}
	}
	for _, op := range operations {
		if op.group == nil {
			if members[op.id] {
				continue
			}
			if err := resumeSingle(ctx, conn, op); err != nil {
				return err
			}
			continue
		}
		if err := resumeGroup(ctx, conn, op.id, *op.group); err != nil {
			return err
		}
	}
	return nil
}

func resumeSingle(ctx context.Context, conn sqlConn, op pendingOperation) error {
	_, revision, err := loadRecordType(ctx, conn, op.recordType.Name)
	if err != nil {
		return err
	}
	if revision != op.expected {
		failOperation(conn, op.id, "resume", fmt.Errorf("record type is at revision %d, operation expected %d", revision, op.expected))
		log.Printf("record type save %s: superseded, not resumed", op.id)
		return nil
	}
	run := runOperation
	if op.action == actionDelete {
		run = runDelete
	}
	if _, err := run(ctx, conn, op.id, op.recordType, op.expected, op.options); err != nil {
		log.Printf("record type save %s: resume failed: %v", op.id, err)
		return nil
	}
	log.Printf("record type %s %s: resumed and completed %q", op.action, op.id, op.recordType.Name)
	return nil
}

// resumeGroup settles a pair save interrupted by a crash. Both members landed
// means only the group mark is missing; anything else rolls the child back.
// A member that moved on without the group is left for an operator.
func resumeGroup(ctx context.Context, conn sqlConn, id string, group saveGroup) error {
	_, childRevision, err := loadRecordType(ctx, conn, group.Child.Name)
	if err != nil {
		return err
	}
	_, parentRevision, err := loadRecordType(ctx, conn, group.ParentName)
	if err != nil {
		return err
	}
	markMembers := func(step string, cause error) {
		for _, member := range []string{group.ChildOp, group.ParentOp} {
			failOperation(conn, member, step, cause)
		}
	}
	if childRevision == group.ChildExpected+1 && parentRevision == group.ParentExpected+1 {
		if _, err := conn.ExecContext(ctx, `UPDATE athena_record_type_operations SET status = ?, last_step = 'completed' WHERE id = ?`, operationCompleted, id); err != nil {
			return err
		}
		log.Printf("record type group %s: both members had landed; marked completed", id)
		return nil
	}
	if childRevision > group.ChildExpected+1 || parentRevision > group.ParentExpected+1 {
		cause := fmt.Errorf("a member moved on without the group; leaving it for an operator")
		failGroup(conn, id, "superseded", cause)
		markMembers("superseded", cause)
		log.Printf("record type group %s: superseded, not resumed", id)
		return nil
	}
	if parentRevision == group.ParentExpected+1 {
		cause := fmt.Errorf("the parent saved without the child; leaving it for an operator")
		failGroup(conn, id, "unexpected", cause)
		markMembers("unexpected", cause)
		log.Printf("record type group %s: parent without child, not resumed", id)
		return nil
	}
	if err := compensateChildSave(ctx, conn, group); err != nil {
		failGroup(conn, id, "compensation_failed", err)
		markMembers("compensation_failed", err)
		log.Printf("record type group %s: compensation failed: %v", id, err)
		return nil
	}
	cause := fmt.Errorf("rolled back after a restart before the pair completed")
	failGroup(conn, id, "compensated", cause)
	markMembers("compensated", cause)
	log.Printf("record type group %s: child rolled back", id)
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
		if op.action == actionSaveGroup {
			var group saveGroup
			if err := json.Unmarshal(payload, &group); err != nil {
				return nil, err
			}
			op.group = &group
			operations = append(operations, op)
			continue
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

// kernelColumns lists every kernel column of the Record Type's table: the
// base system columns plus the tree and effective-date columns when the
// flags are set. It wraps systemColumns (which lives with the lifecycle
// code) so the tree columns stay beside the tree logic.
func kernelColumns(recordType recordTypeDefinition) []store.Column {
	columns := recordType.systemColumns()
	if recordType.Tree {
		parent, _ := dialect.ColumnType("Link")
		columns = append(columns, store.Column{Name: "tree_parent_id", Type: parent},
			store.Column{Name: "tree_path", Type: store.TreePathColumnType},
			store.Column{Name: "tree_depth", Type: dialect.BigIntType()})
	}
	if recordType.EffectiveDated {
		date, _ := dialect.ColumnType("Date")
		columns = append(columns, store.Column{Name: "effective_from", Type: date},
			store.Column{Name: "effective_to", Type: date})
	}
	return columns
}

// validateTreeEffective checks the tree and effective-date flags against the
// kind. Child Record Types already nest under a parent and settings hold one
// row, so neither can be a tree; any kind can be effective-dated, but an
// amendable document cannot, because amend copies values into a new row
// without splitting ranges.
func validateTreeEffective(recordType recordTypeDefinition) error {
	if recordType.EffectiveDated && recordType.Amendable {
		return invalid("an amendable document cannot track effective dates: amend copies values without splitting their range")
	}
	if !recordType.Tree {
		return nil
	}
	switch recordType.kind() {
	case kindMaster, kindDocument:
		return nil
	case kindChild:
		return invalid("a child Record Type already nests under its parent and cannot be a tree")
	default:
		return invalid("a setting holds one record and cannot be a tree")
	}
}

// applySchema creates the Record Type's table if needed, then applies field
// changes from the previous revision. Every statement is safe to repeat.
// Field columns are nullable; Go enforces required fields.
func applySchema(ctx context.Context, conn sqlConn, recordType recordTypeDefinition, previous *recordTypeDefinition, opts saveOptions) error {
	table := recordTypeTableName(recordType.Name)
	columns := kernelColumns(recordType)
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
	if err := ensureSystemColumns(ctx, conn, table, kernelColumns(recordType)); err != nil {
		return err
	}
	if err := ensureLifecycle(ctx, conn, recordType); err != nil {
		return err
	}
	if recordType.kind() == kindChild {
		if err := ensureLink(ctx, conn, recordType.Name, recordTypeField{Name: "parent_id", Type: "Link", TargetRecordType: recordType.ParentRecordType}); err != nil {
			return err
		}
	}
	if recordType.Tree {
		if err := ensureLink(ctx, conn, recordType.Name, recordTypeField{Name: "tree_parent_id", Type: "Link", TargetRecordType: recordType.Name}); err != nil {
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
	definition := recordTypeDefinition{Name: recordType.GetKey(), Label: strings.TrimSpace(recordType.GetLabel()), Description: strings.TrimSpace(recordType.GetDescription()),
		Kind: normalizeKind(recordType.GetKind()), Class: recordType.GetClass(), ParentRecordType: recordType.GetParentRecordTypeKey(), Submittable: recordType.GetSubmittable(), Amendable: recordType.GetAmendable(),
		NamingSeries: strings.TrimSpace(recordType.GetNamingSeries()), GapFree: recordType.GetGapFree(), AllowManualName: recordType.GetAllowManualName(), LabelField: recordType.GetLabelField(),
		Tree: recordType.GetTree(), EffectiveDated: recordType.GetEffectiveDated()}
	for _, field := range recordType.GetFields() {
		out := recordTypeField{Name: field.GetKey(), Label: strings.TrimSpace(field.GetLabel()), Type: field.GetType(), Required: field.GetRequired(), TargetRecordType: field.GetTargetRecordTypeKey(),
			Unique: field.GetUnique(), Default: field.GetDefaultValue(), Min: strings.TrimSpace(field.GetMin()), Max: strings.TrimSpace(field.GetMax()), Pattern: field.GetPattern(), MaxLength: int(field.GetMaxLength()),
			ReadOnly: field.GetReadOnly(), SetOnce: field.GetSetOnce(), NoCopy: field.GetNoCopy(), AllowOnSubmit: field.GetAllowOnSubmit(), Index: field.GetIndex(), Currency: field.GetCurrency(), UoM: field.GetUom(),
			Formula: strings.TrimSpace(field.GetFormula()), RequiredIf: strings.TrimSpace(field.GetRequiredIf()), ReadOnlyIf: strings.TrimSpace(field.GetReadOnlyIf()),
			Validate: strings.TrimSpace(field.GetValidate()), DependsOn: strings.TrimSpace(field.GetDependsOn()), PermissionLevel: int(field.GetPermissionLevel())}
		if field.Scale != nil {
			scale := int(field.GetScale())
			out.Scale = &scale
		}
		for _, v := range field.GetEnumValues() {
			out.Values = append(out.Values, enumValue{Value: v.GetValue(), Label: strings.TrimSpace(v.GetLabel())})
		}
		definition.Fields = append(definition.Fields, out)
	}
	definition.Layout = layoutFromProto(recordType.GetLayout(), definition.Fields)
	return definition
}
func recordTypeToProto(recordType recordTypeDefinition, revision int64) *athenav1.RecordType {
	result := &athenav1.RecordType{Key: recordType.Name, Label: recordType.Label, Description: recordType.Description, Revision: revision,
		Kind: recordType.kind(), Class: recordType.Class, ParentRecordTypeKey: recordType.ParentRecordType, Submittable: recordType.Submittable, Amendable: recordType.Amendable,
		NamingSeries: recordType.NamingSeries, GapFree: recordType.GapFree, AllowManualName: recordType.AllowManualName, LabelField: recordType.LabelField,
		Tree: recordType.Tree, EffectiveDated: recordType.EffectiveDated}
	for _, field := range recordType.Fields {
		out := &athenav1.RecordTypeField{Key: field.Name, Label: field.Label, Type: field.Type, Required: field.Required, TargetRecordTypeKey: field.TargetRecordType,
			Unique: field.Unique, DefaultValue: field.Default, Min: field.Min, Max: field.Max, Pattern: field.Pattern, MaxLength: int32(field.MaxLength),
			ReadOnly: field.ReadOnly, SetOnce: field.SetOnce, NoCopy: field.NoCopy, AllowOnSubmit: field.AllowOnSubmit, Index: field.Index, Currency: field.Currency, Uom: field.UoM,
			Formula: field.Formula, RequiredIf: field.RequiredIf, ReadOnlyIf: field.ReadOnlyIf, Validate: field.Validate, DependsOn: field.DependsOn, PermissionLevel: int32(field.PermissionLevel)}
		if field.Scale != nil {
			scale := int32(*field.Scale)
			out.Scale = &scale
		}
		for _, v := range field.Values {
			out.EnumValues = append(out.EnumValues, &athenav1.EnumValue{Value: v.Value, Label: v.Label})
		}
		result.Fields = append(result.Fields, out)
	}
	result.Layout = layoutToProto(recordType.Layout)
	return result
}
func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
