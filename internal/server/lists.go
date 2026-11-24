package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/events"
	"athena-erp/internal/expr"
	"athena-erp/internal/files"
	"athena-erp/internal/ledger"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/lists"
	"athena-erp/internal/store"
	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

// listService implements athena.v1.ListService over athena_lists
// (migration 23, ADR-0034). It embeds recordService so running a list
// reuses record listing — permissions, Scopes and paging behave exactly
// like Records. The Desk sends the condition tree; the server compiles and
// validates it and stores both forms, and only the compiled expression is
// ever executed.
type listService struct {
	recordService
	files *files.Store
}

func newListService(workspace *Workspace, db *sql.DB, hooks *lifecycle.Registry, slug string) listService {
	s := listService{recordService: recordService{db: db, hooks: hooks, workspace: slug}}
	if workspace == nil || workspace.Root == "" || db == nil {
		return s
	}
	if store, err := files.Open(workspace.Root, db, dialect); err == nil {
		s.files = store
	}
	return s
}

// maxListName is the longest list name in characters (VARCHAR(140)).
const maxListName = 140

func (s listService) SaveList(ctx context.Context, req *v1.SaveListRequest) (_ *v1.SaveListResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "save_list", "")
	defer func() { end(err) }()
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.GetName())
	if n := utf8.RuneCountInString(name); n < 1 || n > maxListName {
		return nil, connect.NewError(connect.CodeInvalidArgument, "the list name holds 1 to 140 characters")
	}
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	var tree expr.Tree
	if err := json.Unmarshal([]byte(req.GetConditionTree()), &tree); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, "the condition tree is not valid JSON")
	}
	env, err := newExprEnvironment(ctx, s.db, recordType, nil)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]expr.Variable, len(env.vars))
	for _, v := range env.vars {
		vars[v.Name] = v
	}
	compiled, err := expr.CompileTree(tree, vars)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	columns := req.GetColumns()
	for _, col := range columns {
		if _, ok := env.byField[col]; !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, "unknown column "+col)
		}
	}
	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return s.createList(ctx, name, recordType.Name, req.GetConditionTree(), compiled, columnsJSON)
	}
	return s.updateList(ctx, req.GetId(), name, recordType.Name, req.GetConditionTree(), compiled, columnsJSON, req.GetExpectedRevision())
}

func (s listService) createList(ctx context.Context, name, recordTypeKey, tree, compiled string, columnsJSON []byte) (*v1.SaveListResponse, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO athena_lists (id, name, record_type_key, condition_tree, condition_expr, columns, owner, revision) VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		id.String(), name, recordTypeKey, tree, compiled, nullableColumns(columnsJSON), actorName(ctx))
	if err != nil {
		if dialect.Classify(err) == store.ErrDuplicateKey {
			return nil, connect.NewError(connect.CodeAlreadyExists, "a list named "+name+" already exists")
		}
		return nil, internalError("save list", err)
	}
	return &v1.SaveListResponse{List: &v1.SavedList{Id: id.String(), Name: name, RecordTypeKey: recordTypeKey, ConditionTree: tree, ConditionExpr: compiled, Columns: columnNames(columnsJSON), Owner: actorName(ctx), Revision: 1}}, nil
}

func (s listService) updateList(ctx context.Context, id, name, recordTypeKey, tree, compiled string, columnsJSON []byte, expected int64) (*v1.SaveListResponse, error) {
	doc, err := s.loadList(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc.Revision != expected {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this list; reload it and try again")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE athena_lists SET name = ?, record_type_key = ?, condition_tree = ?, condition_expr = ?, columns = ?, revision = revision + 1 WHERE id = ? AND revision = ?`,
		name, recordTypeKey, tree, compiled, nullableColumns(columnsJSON), id, expected)
	if err != nil {
		if dialect.Classify(err) == store.ErrDuplicateKey {
			return nil, connect.NewError(connect.CodeAlreadyExists, "a list named "+name+" already exists")
		}
		return nil, internalError("save list", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 0 {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this list; reload it and try again")
	}
	doc.Name, doc.RecordTypeKey, doc.ConditionTree, doc.ConditionExpr = name, recordTypeKey, tree, compiled
	doc.Columns, doc.Revision = columnNames(columnsJSON), doc.Revision+1
	return &v1.SaveListResponse{List: doc}, nil
}

func nullableColumns(raw []byte) any {
	if string(raw) == "null" || string(raw) == "[]" {
		return nil
	}
	return string(raw)
}

func columnNames(raw []byte) []string {
	var out []string
	_ = json.Unmarshal(raw, &out)
	return out
}

func (s listService) loadList(ctx context.Context, id string) (*v1.SavedList, error) {
	var doc v1.SavedList
	var tree, compiled, columns string
	var columnsNull sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, name, record_type_key, condition_tree, condition_expr, columns, owner, revision FROM athena_lists WHERE id = ?`, id).
		Scan(&doc.Id, &doc.Name, &doc.RecordTypeKey, &tree, &compiled, &columnsNull, &doc.Owner, &doc.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, "no such list")
	}
	if err != nil {
		return nil, err
	}
	doc.ConditionTree, doc.ConditionExpr = tree, compiled
	if columnsNull.Valid {
		columns = columnsNull.String
	}
	doc.Columns = columnNames([]byte(columns))
	return &doc, nil
}

// canRead denies when the caller holds no read Grant on the Record Type.
// The operator (no principal) passes, like everywhere else.
func canRead(ctx context.Context, recordTypeKey string) error {
	p, err := accessFor(ctx)
	if err != nil {
		return err
	}
	if p != nil && !p.Can(recordTypeKey, auth.ActionRead) {
		return connect.NewError(connect.CodePermissionDenied, "you do not have permission to do this")
	}
	return nil
}

func (s listService) GetList(ctx context.Context, req *v1.GetListRequest) (*v1.GetListResponse, error) {
	doc, err := s.loadList(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := canRead(ctx, doc.GetRecordTypeKey()); err != nil {
		return nil, err
	}
	return &v1.GetListResponse{List: doc}, nil
}

func (s listService) ListLists(ctx context.Context, req *v1.ListListsRequest) (*v1.ListListsResponse, error) {
	if _, err := s.recordType(ctx, req.GetRecordTypeKey()); err != nil {
		return nil, err
	}
	if err := canRead(ctx, req.GetRecordTypeKey()); err != nil {
		return nil, err
	}
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	pageSize = min(pageSize, maxPageSize)
	// UUIDv7 ids sort by creation time, so the key is the keyset cursor.
	where, args := "record_type_key = ?", []any{req.GetRecordTypeKey()}
	if req.GetPageToken() != "" {
		where += " AND id < ?"
		args = append(args, req.GetPageToken())
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, record_type_key, condition_tree, condition_expr, columns, owner, revision FROM athena_lists WHERE `+where+` ORDER BY id DESC LIMIT ?`, append(args, pageSize+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*v1.SavedList
	for rows.Next() {
		var doc v1.SavedList
		var tree, compiled, columns string
		var columnsNull sql.NullString
		if err := rows.Scan(&doc.Id, &doc.Name, &doc.RecordTypeKey, &tree, &compiled, &columnsNull, &doc.Owner, &doc.Revision); err != nil {
			return nil, err
		}
		doc.ConditionTree, doc.ConditionExpr = tree, compiled
		if columnsNull.Valid {
			columns = columnsNull.String
		}
		doc.Columns = columnNames([]byte(columns))
		out = append(out, &doc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	res := &v1.ListListsResponse{Lists: out}
	if len(out) > pageSize {
		res.Lists = out[:pageSize]
		res.NextPageToken = out[pageSize-1].GetId()
	}
	return res, nil
}

func (s listService) DeleteList(ctx context.Context, req *v1.DeleteListRequest) (*v1.DeleteListResponse, error) {
	if _, err := requireSystem(ctx, auth.PermissionBuild); err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM athena_lists WHERE id = ? AND revision = ?`, req.GetId(), req.GetExpectedRevision())
	if err != nil {
		return nil, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 0 {
		if _, err := s.loadList(ctx, req.GetId()); err != nil {
			return nil, err
		}
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this list; reload it and try again")
	}
	return &v1.DeleteListResponse{}, nil
}

func (s listService) RunList(ctx context.Context, req *v1.RunListRequest) (*v1.RunListResponse, error) {
	doc, err := s.loadList(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := canRead(ctx, doc.GetRecordTypeKey()); err != nil {
		return nil, err
	}
	listed, err := s.recordService.ListRecords(ctx, &v1.ListRecordsRequest{
		RecordTypeKey: doc.GetRecordTypeKey(),
		Filter:        doc.GetConditionExpr(),
		PageSize:      req.GetPageSize(),
		PageToken:     req.GetPageToken(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.RunListResponse{List: doc, Records: listed.GetRecords(), NextPageToken: listed.GetNextPageToken()}, nil
}

// exportDefaultRows and exportMaxRows bound one PDF export: enough for a
// real list, too small to time out a request.
const (
	exportDefaultRows = 200
	exportMaxRows     = 500
)

func (s listService) ExportListPdf(ctx context.Context, req *v1.ExportListPdfRequest) (*v1.ExportListPdfResponse, error) {
	doc, err := s.loadList(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := canRead(ctx, doc.GetRecordTypeKey()); err != nil {
		return nil, err
	}
	maxRows := int(req.GetMaxRows())
	if maxRows <= 0 {
		maxRows = exportDefaultRows
	}
	maxRows = min(maxRows, exportMaxRows)
	recordType, err := s.recordType(ctx, doc.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	columns := doc.GetColumns()
	if len(columns) == 0 {
		for _, field := range recordType.Fields {
			columns = append(columns, field.Name)
		}
	}
	labels := make([]string, len(columns))
	byKey := fieldsByKey(recordType)
	for i, key := range columns {
		labels[i] = key
		if field, ok := byKey[key]; ok {
			labels[i] = field.Label
		}
	}
	var rows [][]string
	cache := &linkCache{}
	token := ""
	for len(rows) < maxRows {
		listed, err := s.recordService.ListRecords(ctx, &v1.ListRecordsRequest{
			RecordTypeKey: doc.GetRecordTypeKey(),
			Filter:        doc.GetConditionExpr(),
			PageSize:      int32(min(maxRows-len(rows), maxPageSize)),
			PageToken:     token,
		})
		if err != nil {
			return nil, err
		}
		for _, record := range listed.GetRecords() {
			rows = append(rows, s.formatRow(ctx, recordType, columns, record, cache))
			if len(rows) >= maxRows {
				break
			}
		}
		if listed.GetNextPageToken() == "" {
			break
		}
		token = listed.GetNextPageToken()
	}
	name, logo := s.identityForExport(ctx)
	pdf, err := lists.RenderListPDF(lists.ListDocument{
		WorkspaceName: name,
		LogoPNG:       logo,
		Title:         doc.GetName(),
		FilterText:    doc.GetConditionExpr(),
		RenderedAt:    time.Now().UTC(),
		Columns:       labels,
		Rows:          rows,
	})
	if err != nil {
		return nil, err
	}
	return &v1.ExportListPdfResponse{Pdf: pdf, Filename: slugFilename(doc.GetName())}, nil
}

// formatRow renders one record's cells as display text, mirroring the
// Desk's recordFieldText: Yes/No for checkboxes, alt text for images,
// linked labels resolved through a per-export cache.
func (s listService) formatRow(ctx context.Context, recordType recordTypeDefinition, columns []string, record *v1.Record, cache *linkCache) []string {
	byKey := fieldsByKey(recordType)
	cells := make([]string, len(columns))
	for i, key := range columns {
		value := record.GetValues()[key]
		if field, ok := byKey[key]; ok && field.Type == "Link" && field.TargetRecordType != "" {
			if id, ok := value.GetKind().(*v1.FieldValue_StringValue); ok && id.StringValue != "" {
				cells[i] = cache.label(ctx, s.db, field.TargetRecordType, id.StringValue)
				continue
			}
			cells[i] = ""
			continue
		}
		cells[i] = listCellText(value)
	}
	return cells
}

// linkCache resolves linked record labels once per target record.
type linkCache struct {
	defs   map[string]recordTypeDefinition
	labels map[string]map[string]string
}

func (c *linkCache) label(ctx context.Context, db *sql.DB, target, id string) string {
	if c.labels == nil {
		c.labels = map[string]map[string]string{}
	}
	byID := c.labels[target]
	if label, ok := byID[id]; ok {
		return label
	}
	def, err := cachedDefinition(ctx, db, c, target)
	if err != nil || def == nil {
		return ""
	}
	record, err := loadRecord(ctx, db, *def, id, "")
	label := ""
	if err == nil && record != nil {
		label = listCellText(record.GetValues()[def.LabelField])
	}
	if byID == nil {
		byID = map[string]string{}
		c.labels[target] = byID
	}
	byID[id] = label
	return label
}

func cachedDefinition(ctx context.Context, db *sql.DB, c *linkCache, target string) (*recordTypeDefinition, error) {
	if c.defs == nil {
		c.defs = map[string]recordTypeDefinition{}
	}
	if def, ok := c.defs[target]; ok {
		return &def, nil
	}
	def, _, err := loadRecordType(ctx, db, target)
	if err != nil || def == nil {
		return nil, err
	}
	c.defs[target] = *def
	return def, nil
}

func listCellText(value *v1.FieldValue) string {
	switch v := value.GetKind().(type) {
	case *v1.FieldValue_StringValue:
		return v.StringValue
	case *v1.FieldValue_BoolValue:
		if v.BoolValue {
			return "Yes"
		}
		return "No"
	case *v1.FieldValue_ImageValue:
		return v.ImageValue.GetAltText()
	}
	return ""
}

func slugFilename(name string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			out.WriteRune('-')
		}
	}
	slug := strings.Trim(out.String(), "-")
	if slug == "" {
		slug = "list"
	}
	return slug + ".pdf"
}

// messageRequested is the outbox event type of one queued list message.
// Its payload carries ids and counts only, never Record values.
const messageRequested = "list.message.requested"

const (
	queueDefaultRecipients = 200
	queueMaxRecipients     = 500
)

func (s listService) QueueMessages(ctx context.Context, req *v1.QueueMessagesRequest) (*v1.QueueMessagesResponse, error) {
	if _, err := requireSystem(ctx, auth.PermissionListsSend); err != nil {
		return nil, err
	}
	doc, err := s.loadList(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := canRead(ctx, doc.GetRecordTypeKey()); err != nil {
		return nil, err
	}
	if req.GetChannel() != "email" && req.GetChannel() != "message" {
		return nil, connect.NewError(connect.CodeInvalidArgument, "channel is email or message")
	}
	maxRecipients := int(req.GetMaxRecipients())
	if maxRecipients <= 0 {
		maxRecipients = queueDefaultRecipients
	}
	maxRecipients = min(maxRecipients, queueMaxRecipients)
	query := doc.GetConditionExpr()
	if strings.TrimSpace(req.GetAudienceTree()) != "" {
		recordType, err := s.recordType(ctx, doc.GetRecordTypeKey())
		if err != nil {
			return nil, err
		}
		env, err := newExprEnvironment(ctx, s.db, recordType, nil)
		if err != nil {
			return nil, err
		}
		vars := make(map[string]expr.Variable, len(env.vars))
		for _, v := range env.vars {
			vars[v.Name] = v
		}
		var tree expr.Tree
		if err := json.Unmarshal([]byte(req.GetAudienceTree()), &tree); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, "the audience tree is not valid JSON")
		}
		audience, err := expr.CompileTree(tree, vars)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err.Error())
		}
		query = "(" + query + ") && (" + audience + ")"
	}
	recipients, err := s.collectIDs(ctx, doc.GetRecordTypeKey(), query, maxRecipients)
	if err != nil {
		return nil, err
	}
	total := int64(len(recipients))
	excluded := int64(0)
	if query != doc.GetConditionExpr() {
		base, err := s.collectIDs(ctx, doc.GetRecordTypeKey(), doc.GetConditionExpr(), maxRecipients)
		if err != nil {
			return nil, err
		}
		total, excluded = int64(len(base)), int64(len(base)-len(recipients))
	}
	if req.GetDryRun() {
		return &v1.QueueMessagesResponse{Queued: int64(len(recipients)), Excluded: excluded, Total: total, Query: query}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("queue messages", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, recipient := range recipients {
		if _, err := events.Emit(ctx, tx, events.Envelope{
			EventType:       messageRequested,
			Workspace:       s.workspace,
			Producer:        "lists",
			SubjectType:     doc.GetRecordTypeKey(),
			SubjectID:       recipient.id,
			SubjectRevision: recipient.revision,
			Actor:           actorName(ctx),
			Payload:         map[string]any{"list_id": doc.GetId(), "channel": req.GetChannel(), "query": query},
		}); err != nil {
			return nil, internalError("queue messages", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("queue messages", err)
	}
	return &v1.QueueMessagesResponse{Queued: int64(len(recipients)), Excluded: excluded, Total: total, Query: query}, nil
}

type recipient struct {
	id       string
	revision int64
}

func (s listService) collectIDs(ctx context.Context, recordTypeKey, filter string, max int) ([]recipient, error) {
	var out []recipient
	token := ""
	for len(out) < max {
		listed, err := s.recordService.ListRecords(ctx, &v1.ListRecordsRequest{
			RecordTypeKey: recordTypeKey,
			Filter:        filter,
			PageSize:      int32(min(max-len(out), maxPageSize)),
			PageToken:     token,
		})
		if err != nil {
			return nil, err
		}
		for _, record := range listed.GetRecords() {
			out = append(out, recipient{id: record.GetId(), revision: record.GetRevision()})
			if len(out) >= max {
				break
			}
		}
		if listed.GetNextPageToken() == "" {
			break
		}
		token = listed.GetNextPageToken()
	}
	return out, nil
}

// identityForExport reads the workspace display name and light PNG logo
// for the PDF header. Anything missing degrades to name-only; export never
// fails for branding.
func (s listService) identityForExport(ctx context.Context) (string, []byte) {
	svc := ledger.Service{DB: s.db, Dialect: dialect}
	doc, err := svc.GetIdentityDoc(ctx)
	name := doc.DisplayName
	if name == "" {
		name = s.workspace
	}
	if err != nil || doc.Light == nil || doc.Light.Ext != "png" || s.files == nil {
		return name, nil
	}
	_, content, err := s.files.Get(ctx, "brand/logo-light.png", 0)
	if err != nil || files.Hash(content) != doc.Light.Hash {
		return name, nil
	}
	return name, content
}
