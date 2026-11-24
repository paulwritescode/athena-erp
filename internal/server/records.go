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
	"athena-erp/internal/files"
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
	// workspace names the Workspace slug on durable event envelopes; empty
	// leaves it blank.
	workspace string
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
	if req.GetParentId() != "" {
		if _, err := uuid.Parse(req.GetParentId()); err != nil {
			return nil, invalid("parent_id must be a record ID")
		}
		switch {
		case recordType.kind() == kindChild:
			where += " AND `parent_id` = ?"
		case recordType.Tree:
			where += " AND `tree_parent_id` = ?"
		default:
			return nil, invalid("only child and tree Record Types have a parent")
		}
		args = append(args, req.GetParentId())
	}
	if req.GetAsOf() != "" {
		if !recordType.EffectiveDated {
			return nil, invalid(fmt.Sprintf("%s does not track effective dates", recordType.Label))
		}
		asOf, err := parseAsOf(req.GetAsOf())
		if err != nil {
			return nil, err
		}
		where += " AND " + asOfPredicate
		args = append(args, asOf, asOf)
	}
	if live := liveClause(dialect, recordType); live != "" {
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
	if q := strings.TrimSpace(req.GetQuery()); q != "" {
		// The match runs in the database inside the paged query, so a search
		// never reads more rows than a plain list.
		predicate, pargs := matchPredicate(visibleDefinition(recordType, hidden), q)
		where += " AND (" + predicate + ")"
		args = append(args, pargs...)
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
	if err := refuseKernelKind(recordType.kind()); err != nil {
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
	if req.GetParentId() != "" && recordType.kind() != kindChild && !recordType.Tree {
		return nil, invalid("only child and tree Record Types have a parent")
	}
	if !recordType.EffectiveDated && (req.GetEffectiveFrom() != "" || req.GetEffectiveTo() != "") {
		return nil, invalid(fmt.Sprintf("%s does not track effective dates", recordType.Label))
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("create record", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := serializeTypeWrites(ctx, tx, recordType); err != nil {
		return nil, err
	}
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
	if recordType.Tree {
		if req.GetParentId() == "" {
			system["tree_path"], system["tree_depth"] = treeRootPath(id.String()), 0
		} else {
			parentPath, parentDepth, err := loadTreeParent(ctx, tx, recordType, req.GetParentId())
			if err != nil {
				return nil, err
			}
			system["tree_parent_id"] = req.GetParentId()
			system["tree_path"], system["tree_depth"] = parentPath+id.String()+"/", parentDepth+1
		}
	}
	var fromArg, toArg any
	if recordType.EffectiveDated {
		if fromArg, toArg, err = parseEffectiveRange(req.GetEffectiveFrom(), req.GetEffectiveTo()); err != nil {
			return nil, err
		}
		system["effective_from"], system["effective_to"] = fromArg, toArg
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
	if recordType.EffectiveDated {
		if err := checkEffectiveOverlap(ctx, tx, recordType, columns, args, nil, id.String(), req.GetParentId(), fromArg, toArg); err != nil {
			return nil, err
		}
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
	for _, column := range kernelColumns(recordType) {
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
	if live := liveClause(dialect, *parent); live != "" {
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
	// On an effective-dated type unique means unique-at-a-time: history rows
	// share values across disjoint ranges, and checkEffectiveOverlap owns
	// that check with its own message.
	if recordType.EffectiveDated {
		return nil
	}
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
		if live := liveClause(dialect, recordType); live != "" {
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

// serializeTypeWrites locks the Record Type's definition row, so writes to
// tree and effective-dated tables run one at a time per type. The parent,
// cycle and overlap checks read rows another write could move, and no index
// guards them the way a unique index guards checkUnique; the lock closes
// that race. It is always taken before any row lock, so writers cannot
// deadlock on it.
func serializeTypeWrites(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition) error {
	if !recordType.Tree && !recordType.EffectiveDated {
		return nil
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM athena_record_types WHERE name = ? `+dialect.ForUpdate(), recordType.Name).Scan(&revision); err != nil {
		return internalError("lock Record Type", err)
	}
	return nil
}

// treeRootPath is the path of a top-level row. IDs hold no slashes, so the
// join is unambiguous and a path prefix always names a subtree.
func treeRootPath(id string) string { return "/" + id + "/" }

// loadTreeParent locks the parent row, refuses a missing or deleted parent,
// repairs a missing path and returns the parent's path and depth.
func loadTreeParent(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, parentID string) (string, int64, error) {
	if _, err := uuid.Parse(parentID); err != nil {
		return "", 0, invalid("the parent must be a record ID")
	}
	table := dialect.QuoteIdent(recordTypeTableName(recordType.Name))
	query := "SELECT `tree_parent_id`, `tree_path`, `tree_depth` FROM " + table + " WHERE `id` = ?"
	if live := liveClause(dialect, recordType); live != "" {
		query += " AND " + live
	}
	var grandparent, path sql.NullString
	var depth sql.NullInt64
	switch err := tx.QueryRowContext(ctx, query+" "+dialect.ForUpdate(), parentID).Scan(&grandparent, &path, &depth); {
	case errors.Is(err, sql.ErrNoRows):
		return "", 0, invalid(fmt.Sprintf("the %s that should be the parent does not exist", recordType.Label))
	case err != nil:
		return "", 0, internalError("check parent", err)
	}
	if path.String != "" {
		return path.String, depth.Int64, nil
	}
	return repairTreePath(ctx, tx, recordType, parentID, grandparent.String)
}

// repairTreePath recomputes the path and depth of id from the stored parent
// links, healing rows written before paths existed. It walks up to the
// first ancestor with a path (or a top-level row) and writes back down;
// the caller holds the type lock, so the chain cannot move underneath it.
func repairTreePath(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id, parentID string) (string, int64, error) {
	table := dialect.QuoteIdent(recordTypeTableName(recordType.Name))
	chain := []string{id}
	next := parentID
	base, level := "", int64(0)
	for range 100 {
		if next == "" {
			base, level = "/", -1
			break
		}
		var grandparent, path sql.NullString
		var depth sql.NullInt64
		switch err := tx.QueryRowContext(ctx, "SELECT `tree_parent_id`, `tree_path`, `tree_depth` FROM "+table+" WHERE `id` = ? "+dialect.ForUpdate(), next).Scan(&grandparent, &path, &depth); {
		case errors.Is(err, sql.ErrNoRows):
			return "", 0, internalError("repair tree path", fmt.Errorf("parent %s is gone", next))
		case err != nil:
			return "", 0, internalError("repair tree path", err)
		}
		for _, have := range chain {
			if have == next {
				return "", 0, internalError("repair tree path", fmt.Errorf("cycle through %s", next))
			}
		}
		if path.String != "" {
			base, level = path.String, depth.Int64
			break
		}
		chain = append(chain, next)
		next = grandparent.String
	}
	if base == "" {
		return "", 0, internalError("repair tree path", errors.New("the parent chain is deeper than 100"))
	}
	var got string
	var gotDepth int64
	for i := len(chain) - 1; i >= 0; i-- {
		level++
		got, gotDepth = base+chain[i]+"/", level
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET `tree_path` = ?, `tree_depth` = ? WHERE `id` = ?", got, gotDepth, chain[i]); err != nil {
			return "", 0, internalError("repair tree path", err)
		}
	}
	return got, gotDepth, nil
}

// moveTreeSubtree rewrites the paths of the descendants of id after a move.
// Paths hold UUIDs only, so the LIKE prefix matches exactly the subtree.
func moveTreeSubtree(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id, oldPrefix, newPrefix string, delta int64) error {
	table := dialect.QuoteIdent(recordTypeTableName(recordType.Name))
	_, err := tx.ExecContext(ctx, "UPDATE "+table+" SET `tree_path` = CONCAT(?, SUBSTRING(`tree_path`, ?)), `tree_depth` = `tree_depth` + ? WHERE `tree_path` LIKE ? AND `id` <> ?",
		newPrefix, len(oldPrefix)+1, delta, oldPrefix+"%", id)
	if err != nil {
		return internalError("move record", err)
	}
	return nil
}

// kernelAssign is one kernel column written by an update alongside fields.
type kernelAssign struct {
	column string
	value  any
}

// treeMove is the moved row's own tree columns; descendants are rewritten
// inside planTreeMove.
type treeMove struct {
	parent any
	path   string
	depth  int64
}

// planTreeMove validates a move, rewrites the descendants' paths and returns
// the moved row's own columns. Moving under itself or its own descendant
// would close a cycle: the new parent's path would start with the row's own
// path prefix.
func planTreeMove(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id string, current *athenav1.Record, newParent string) (treeMove, error) {
	if newParent == id {
		return treeMove{}, invalid("a record cannot be its own parent")
	}
	oldPrefix := current.GetTreePath()
	if oldPrefix == "" {
		oldPrefix = treeRootPath(id)
	}
	var moved treeMove
	if newParent == "" {
		moved = treeMove{path: treeRootPath(id)}
	} else {
		parentPath, parentDepth, err := loadTreeParent(ctx, tx, recordType, newParent)
		if err != nil {
			return treeMove{}, err
		}
		if strings.HasPrefix(parentPath, oldPrefix) {
			return treeMove{}, invalid(fmt.Sprintf("%s is already under this record, so the move would close a cycle", recordType.Label))
		}
		moved = treeMove{parent: newParent, path: parentPath + id + "/", depth: parentDepth + 1}
	}
	if err := moveTreeSubtree(ctx, tx, recordType, id, oldPrefix, moved.path, moved.depth-current.GetTreeDepth()); err != nil {
		return treeMove{}, err
	}
	return moved, nil
}

// auditText renders a tree or range value for the Audit Log; empty is nil.
func auditText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// checkEffectiveOverlap refuses a range that overlaps another live row of
// the same entity. Rows are one entity when no unique field tells them
// apart (a NULL side never tells); a type without unique fields holds one
// entity, and child rows are further scoped by their parent, so one line
// code can repeat across documents. current is nil on create; unchanged
// unique fields are read from the stored row through a self-join.
func checkEffectiveOverlap(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, columns []string, args []any, current *athenav1.Record, selfID, parentID string, fromArg, toArg any) error {
	bound := map[string]any{}
	for i, column := range columns {
		bound[strings.Trim(column, "`")] = args[i]
	}
	table := dialect.QuoteIdent(recordTypeTableName(recordType.Name))
	params := []any{}
	from := table + " AS `o`"
	needCurrent := false
	if current != nil {
		for _, field := range recordType.Fields {
			if field.Unique {
				if _, ok := bound[field.Name]; !ok {
					needCurrent = true
					break
				}
			}
		}
	}
	if needCurrent {
		from += " JOIN " + table + " AS `c` ON `c`.`id` = ?"
		params = append(params, selfID)
	}
	conditions := []string{
		"(? IS NULL OR `o`.`effective_to` IS NULL OR ? < `o`.`effective_to`)",
		"(`o`.`effective_from` IS NULL OR ? IS NULL OR `o`.`effective_from` < ?)",
		"`o`.`id` <> ?",
	}
	params = append(params, fromArg, fromArg, toArg, toArg, selfID)
	if recordType.kind() == kindMaster {
		conditions = append(conditions, "`o`.`deleted_at` IS NULL")
	}
	if recordType.kind() == kindChild {
		conditions = append(conditions, "`o`.`parent_id` = ?")
		params = append(params, parentID)
	}
	for _, field := range recordType.Fields {
		if !field.Unique {
			continue
		}
		column := dialect.QuoteIdent(field.Name)
		if value, ok := bound[field.Name]; ok {
			conditions = append(conditions, fmt.Sprintf("(`o`.%[1]s IS NULL OR ? IS NULL OR `o`.%[1]s = ?)", column))
			params = append(params, value, value)
		} else if needCurrent {
			conditions = append(conditions, fmt.Sprintf("(`o`.%[1]s IS NULL OR `c`.%[1]s IS NULL OR `o`.%[1]s = `c`.%[1]s)", column))
		}
	}
	var clashes bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+from+" WHERE "+strings.Join(conditions, " AND ")+")", params...).Scan(&clashes); err != nil {
		return internalError("check effective range", err)
	}
	if !clashes {
		return nil
	}
	var keys []string
	for _, field := range recordType.Fields {
		if field.Unique {
			keys = append(keys, field.Label)
		}
	}
	if len(keys) == 0 {
		return invalid(fmt.Sprintf("these effective dates overlap another %s; %s holds one effective row at a time", recordType.Label, recordType.Label))
	}
	return invalid(fmt.Sprintf("these effective dates overlap another %s with the same %s", recordType.Label, strings.Join(keys, ", ")))
}

// asOfPredicate keeps the rows effective on the given date, end-exclusive.
const asOfPredicate = "(`effective_from` IS NULL OR `effective_from` <= ?) AND (`effective_to` IS NULL OR ? < `effective_to`)"

func (s recordService) UpdateRecord(ctx context.Context, req *athenav1.UpdateRecordRequest) (res *athenav1.UpdateRecordResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "update_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	if err := refuseKernelKind(recordType.kind()); err != nil {
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
	if req.TreeParentId != nil && !recordType.Tree {
		return nil, invalid(fmt.Sprintf("%s is not a tree", recordType.Label))
	}
	if (req.EffectiveFrom != nil || req.EffectiveTo != nil) && !recordType.EffectiveDated {
		return nil, invalid(fmt.Sprintf("%s does not track effective dates", recordType.Label))
	}
	// A move or range change may carry no field values.
	var columns []string
	var args []any
	if req.TreeParentId != nil || req.EffectiveFrom != nil || req.EffectiveTo != nil {
		columns, args, err = bindUpdateValues(recordType, req.GetValues())
	} else {
		columns, args, err = bindValues(recordType, req.GetValues(), false)
	}
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
	if err := serializeTypeWrites(ctx, tx, recordType); err != nil {
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
	// A move or range change counts as a change even with no field values.
	move := req.TreeParentId != nil && *req.TreeParentId != current.GetTreeParentId()
	newFrom, newTo := current.GetEffectiveFrom(), current.GetEffectiveTo()
	if req.EffectiveFrom != nil {
		newFrom = *req.EffectiveFrom
	}
	if req.EffectiveTo != nil {
		newTo = *req.EffectiveTo
	}
	rangeChange := newFrom != current.GetEffectiveFrom() || newTo != current.GetEffectiveTo()
	if len(changed) == 0 && !move && !rangeChange {
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
	// Tree moves and range changes run after the field checks, against the
	// final unique values.
	var extraAssigns []kernelAssign
	var extraChanges []audit.Change
	var extraKeys []string
	if move {
		moved, err := planTreeMove(ctx, tx, recordType, req.GetId(), current, *req.TreeParentId)
		if err != nil {
			return nil, err
		}
		extraAssigns = append(extraAssigns,
			kernelAssign{column: "tree_parent_id", value: moved.parent},
			kernelAssign{column: "tree_path", value: moved.path},
			kernelAssign{column: "tree_depth", value: moved.depth})
		extraChanges = append(extraChanges, audit.Change{Field: "tree_parent_id", Old: auditText(current.GetTreeParentId()), New: auditText(*req.TreeParentId)})
		extraKeys = append(extraKeys, "tree_parent_id")
	}
	if rangeChange {
		fromArg, toArg, err := parseEffectiveRange(newFrom, newTo)
		if err != nil {
			return nil, err
		}
		if err := checkEffectiveOverlap(ctx, tx, recordType, changedColumns, changedArgs, current, req.GetId(), current.GetParentId(), fromArg, toArg); err != nil {
			return nil, err
		}
		extraAssigns = append(extraAssigns,
			kernelAssign{column: "effective_from", value: fromArg},
			kernelAssign{column: "effective_to", value: toArg})
		if newFrom != current.GetEffectiveFrom() {
			extraChanges = append(extraChanges, audit.Change{Field: "effective_from", Old: auditText(current.GetEffectiveFrom()), New: auditText(newFrom)})
			extraKeys = append(extraKeys, "effective_from")
		}
		if newTo != current.GetEffectiveTo() {
			extraChanges = append(extraChanges, audit.Change{Field: "effective_to", Old: auditText(current.GetEffectiveTo()), New: auditText(newTo)})
			extraKeys = append(extraKeys, "effective_to")
		}
	}
	assignments := []string{dialect.QuoteIdent("revision") + " = " + dialect.QuoteIdent("revision") + " + 1"}
	for _, column := range changedColumns {
		assignments = append(assignments, column+" = ?")
	}
	for _, extra := range extraAssigns {
		assignments = append(assignments, dialect.QuoteIdent(extra.column)+" = ?")
		changedArgs = append(changedArgs, extra.value)
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
	// The proposed row must satisfy the Assignments too: the old row was
	// checked above, and moving a Record out of scope is refused here.
	if err := authorizeProposed(ctx, tx, p, recordType, req.GetId(), auth.ActionWrite); err != nil {
		return nil, err
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
	changes := fieldChanges(changed, current, updated)
	changes = append(changes, extraChanges...)
	changed = append(changed, extraKeys...)
	if err := writeAudit(ctx, tx, recordType.Name, req.GetId(), current.GetRevision()+1, "updated", changes); err != nil {
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
func liveClause(d store.Dialect, recordType recordTypeDefinition) string {
	if recordType.kind() == kindMaster {
		return d.QuoteIdent("deleted_at") + " IS NULL"
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
	if err := refuseKernelKind(recordType.kind()); err != nil {
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
	if err := serializeTypeWrites(ctx, tx, recordType); err != nil {
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
	if recordType.Tree {
		if err := ensureNoTreeChildren(ctx, tx, recordType, req.GetId()); err != nil {
			return nil, err
		}
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

// ensureNoTreeChildren returns FailedPrecondition when a live row sits under
// the doomed row. The type lock serializes this against creates and moves.
func ensureNoTreeChildren(ctx context.Context, tx *sql.Tx, recordType recordTypeDefinition, id string) error {
	query := "SELECT EXISTS(SELECT 1 FROM " + dialect.QuoteIdent(recordTypeTableName(recordType.Name)) + " WHERE `tree_parent_id` = ?"
	if live := liveClause(dialect, recordType); live != "" {
		query += " AND " + live
	}
	var children bool
	if err := tx.QueryRowContext(ctx, query+")", id).Scan(&children); err != nil {
		return internalError("check children", err)
	}
	if children {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("this %s still has records under it; move or delete them first", recordType.Label))
	}
	return nil
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
	if live := liveClause(dialect, recordType); live != "" {
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
	if live := liveClause(dialect, recordType); live != "" {
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
	for _, column := range kernelColumns(recordType) {
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
	var treeParentID, treePath sql.NullString
	var treeDepth sql.NullInt64
	var effectiveFrom, effectiveTo sql.NullTime
	system := map[string]any{"created_by": &createdBy, "modified_by": &modifiedBy, "request_id": &requestID, "owner": &owner, "parent_id": &parentID, "def_version": &defVersion, "idx": &idx, "deleted_at": &deletedAt, "docstate": &docstate, "name": &name, "amended_from": &amendedFrom,
		"tree_parent_id": &treeParentID, "tree_path": &treePath, "tree_depth": &treeDepth, "effective_from": &effectiveFrom, "effective_to": &effectiveTo}
	for _, column := range kernelColumns(recordType) {
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
	record.TreeParentId, record.TreePath, record.TreeDepth = treeParentID.String, treePath.String, treeDepth.Int64
	if recordType.Tree && record.TreePath == "" && record.TreeParentId == "" {
		// A parentless row is a top-level entry even when its path was
		// never written; the next write under it heals the stored row.
		record.TreePath = treeRootPath(record.Id)
	}
	if effectiveFrom.Valid {
		record.EffectiveFrom = effectiveFrom.Time.Format(time.DateOnly)
	}
	if effectiveTo.Valid {
		record.EffectiveTo = effectiveTo.Time.Format(time.DateOnly)
	}
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
				if field.Type == "Image" {
					var ref imageReference
					if err := json.Unmarshal([]byte(v.String), &ref); err != nil {
						return nil, internalError("read image reference", err)
					}
					value.Kind = &athenav1.FieldValue_ImageValue{ImageValue: &athenav1.ImageValue{Path: ref.Path, Hash: ref.Hash, AltText: ref.AltText}}
					break
				}
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
	return bindValuesInner(recordType, values, creating, false)
}

// bindUpdateValues binds an update that may change only tree or effective
// columns, with no field values.
func bindUpdateValues(recordType recordTypeDefinition, values map[string]*athenav1.FieldValue) ([]string, []any, error) {
	return bindValuesInner(recordType, values, false, true)
}

func bindValuesInner(recordType recordTypeDefinition, values map[string]*athenav1.FieldValue, creating, allowEmpty bool) ([]string, []any, error) {
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
	if !creating && len(columns) == 0 && !allowEmpty {
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
// maxAltText is the longest alt text an image value may carry: a sentence
// for screen readers, not a caption.
const maxAltText = 1000

var shaHexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// imageReference is the stored form of an Image value: a Workspace file
// path with the hash the writer saw and the alt text readers hear.
type imageReference struct {
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	AltText string `json:"alt_text"`
}

// imageArgument validates an Image value and renders it for the JSON column.
// The reference shape is checked here; the file itself is verified when the
// layout that shows it saves, and through GetFile when it is read.
func imageArgument(field recordTypeField, value *athenav1.FieldValue) (any, error) {
	v, ok := value.GetKind().(*athenav1.FieldValue_ImageValue)
	if !ok {
		return nil, invalid(fmt.Sprintf("%s must be an image reference", field.Label))
	}
	ref := v.ImageValue
	path := strings.TrimSpace(ref.GetPath())
	if path == "" || !files.IsImagePath(path) {
		return nil, invalid(fmt.Sprintf("%s must reference an image under images/", field.Label))
	}
	if !shaHexPattern.MatchString(ref.GetHash()) {
		return nil, invalid(fmt.Sprintf("%s must carry the 64-character hash of its file", field.Label))
	}
	alt := strings.TrimSpace(ref.GetAltText())
	if alt == "" {
		return nil, invalid(fmt.Sprintf("%s needs alt text describing the image", field.Label))
	}
	if utf8.RuneCountInString(alt) > maxAltText {
		return nil, invalid(fmt.Sprintf("%s: the alt text is longer than %d characters", field.Label, maxAltText))
	}
	payload, err := json.Marshal(imageReference{Path: path, Hash: ref.GetHash(), AltText: alt})
	if err != nil {
		return nil, internalError("write image reference", err)
	}
	return string(payload), nil
}

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
	if field.Type == "Image" {
		return imageArgument(field, value)
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
