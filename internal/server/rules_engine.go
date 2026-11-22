package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/notifications"
	"athena-erp/internal/posting"
	"athena-erp/internal/rules"
	"athena-erp/internal/types"

	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Starlark Rules in the kernel (spec 13, ADR-0016, Proposed). The engine is a
// lifecycle hook: it runs the enabled Rules of a document's Record Type for
// each in-transaction event, inside the transaction that saves the document,
// as the acting user. A Rule reaches data only through ruleHost, which applies
// the same Grants, field levels and field checks as the API. Rule changes to
// a document pass through lifecycle.Document and are applied by the record
// service with the checks of a user's save, so a Rule can never change a field
// the user could not.

// ruleSystemAttributes are the kernel columns a Rule can read as doc.<name>.
var ruleSystemAttributes = []string{"id", "revision", "docstate", "name", "owner", "created_by", "modified_by"}

// ruleRefs lists what a Rule on d may name, for compile-time checks.
func ruleRefs(d recordTypeDefinition) rules.Refs {
	refs := rules.Refs{Doc: map[string]bool{}, Writable: map[string]bool{}}
	for _, name := range ruleSystemAttributes {
		refs.Doc[name] = true
	}
	for _, field := range d.Fields {
		refs.Doc[field.Name] = true
		if field.Formula == "" {
			refs.Writable[field.Name] = true
		}
	}
	return refs
}

// storedRule is a Rule row as the engine loads it.
type storedRule struct {
	Key      string
	Version  int64
	Type     rules.Type
	Event    string
	Priority int
	Source   string
}

// ruleStackKey carries the Rules running in this call chain (rules.Stack).
type ruleStackKey struct{}

// registerRules attaches the Rule engine to every event a Rule can use.
func registerRules(hooks *lifecycle.Registry, db *sql.DB) {
	engine := ruleEngine{db: db}
	for _, name := range lifecycle.Order {
		hooks.On(name, "", engine.hook)
	}
}

type ruleEngine struct{ db *sql.DB }

// hook runs the Rules attached to one event. The first Rule that rejects or
// fails stops the operation; the caller rolls its transaction back.
func (e ruleEngine) hook(ctx context.Context, ev *lifecycle.Event) error {
	if ev.Tx == nil || ev.Doc == nil {
		return nil
	}
	// The Rules are read on the engine's own connection. A plain read inside
	// the document's transaction would start its REPEATABLE READ snapshot
	// early, and a save that has no Rule must not pay for that (it broke the
	// numbering counter insert, MariaDB error 1020).
	list, err := loadRulesFor(ctx, e.db, ev.RecordType, string(ev.Name))
	if err != nil {
		return err
	}
	// Posting Rules run through the posting path, which collects their plans
	// into the submit transaction; the generic engine would discard them, so
	// it skips them here.
	runnable := list[:0]
	for _, rule := range list {
		if rule.Type != rules.Posting {
			runnable = append(runnable, rule)
		}
	}
	if list = runnable; len(list) == 0 {
		return nil
	}
	def, err := loadRecordTypeFrom(ctx, ev.Tx, ev.RecordType)
	if err != nil {
		return err
	}
	for _, rule := range list {
		run := ruleRun{
			engine: e, rule: rule, def: &def, tx: ev.Tx, doc: ev.Doc, recordID: ev.RecordID, event: string(ev.Name),
		}
		out, err := run.execute(ctx)
		if err != nil {
			return err
		}
		if err := outcomeError(rule.Key, out); err != nil {
			return err
		}
	}
	return nil
}

// loadRulesFor returns the enabled Rules of one Record Type and event in run
// order: derivations, validations, guards, then priority, then key.
func loadRulesFor(ctx context.Context, q querier, recordType, event string) ([]storedRule, error) {
	rows, err := q.QueryContext(ctx, "SELECT name, version, rule_type, event, priority, source FROM athena_rules WHERE record_type = ? AND event = ? AND enabled = 1", recordType, event)
	if err != nil {
		return nil, internalError("load rules", err)
	}
	defer rows.Close()
	var list []storedRule
	for rows.Next() {
		var r storedRule
		var kind string
		if err := rows.Scan(&r.Key, &r.Version, &kind, &r.Event, &r.Priority, &r.Source); err != nil {
			return nil, internalError("read rule", err)
		}
		r.Type = rules.Type(kind)
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("load rules", err)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Type.Order() != b.Type.Order() {
			return a.Type.Order() < b.Type.Order()
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.Key < b.Key
	})
	return list, nil
}

// outcomeError turns a rejected or failed run into the error the caller of the
// save sees. A rejection shows the Rule's own message; any other failure
// names the Rule and the code, and keeps detail in the Rule Run log.
func outcomeError(key string, out rules.Outcome) error {
	switch out.Status {
	case rules.Rejected:
		return connect.NewError(connect.CodeFailedPrecondition, out.Message)
	case rules.Errored:
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("The Rule %s could not run (%s): %s", key, out.Code, out.Message))
	}
	return nil
}

// ruleRun is one execution of one Rule on one document.
type ruleRun struct {
	engine   ruleEngine
	rule     storedRule
	def      *recordTypeDefinition // nil for a transform with no Record Type
	tx       *sql.Tx
	doc      *lifecycle.Document
	recordID string
	event    string
	dryRun   bool
	// source overrides rule.Source for a draft being simulated.
	source string
}

// execute compiles and runs the Rule, then writes the Rule Run row on its own
// connection, so a rejected save still leaves its record. The error is only
// for failures of the kernel itself; a Rule that rejects or errors comes back
// in the Outcome.
func (r ruleRun) execute(ctx context.Context) (rules.Outcome, error) {
	ctx, span := otel.Tracer("athena/kernel").Start(ctx, "athena.rule_run")
	defer span.End()
	requestID, _ := requestFrom(ctx)
	stack, _ := ctx.Value(ruleStackKey{}).(rules.Stack)
	stack, stackErr := stack.Enter(r.rule.Key, rules.DefaultLimits.MaxCascadeDepth)
	ctx = context.WithValue(ctx, ruleStackKey{}, stack)

	source := r.rule.Source
	if r.source != "" {
		source = r.source
	}
	var refs rules.Refs
	recordType := ""
	if r.def != nil {
		refs, recordType = ruleRefs(*r.def), r.def.Name
	}
	var out rules.Outcome
	program, compileErr := rules.Compile(r.rule.Type, source, refs)
	p, accessErr := accessFor(ctx)
	switch {
	case stackErr != nil:
		out = rules.Outcome{Status: rules.Errored, Code: rules.CodeCascadeLimit, Message: stackErr.Error()}
	case compileErr != nil:
		out = rules.Outcome{Status: rules.Errored, Code: "compile", Message: compileErr.Error()}
	case accessErr != nil:
		return rules.Outcome{}, accessErr
	default:
		host := &ruleHost{ctx: ctx, tx: r.tx, principal: p, def: r.def, doc: r.doc}
		in := rules.Input{
			Rule: r.rule.Key, Version: r.rule.Version, Event: r.event, RecordType: recordType, RecordID: r.recordID,
			Doc: host.docRow(), Now: ruleNow().UTC(), User: actorName(ctx), RequestID: requestID,
		}
		out = program.Run(ctx, in, host, rules.DefaultLimits)
	}
	span.SetAttributes(
		attribute.String("athena.rule", r.rule.Key), attribute.String("athena.rule_type", string(r.rule.Type)),
		attribute.String("athena.event", r.event), attribute.String("athena.request_id", requestID),
		attribute.String("athena.outcome", string(out.Status)), attribute.String("athena.outcome_code", out.Code),
		attribute.Int64("athena.steps", int64(out.Steps)), attribute.Int("athena.reads", out.Reads),
		attribute.Int("athena.values", out.Values),
		attribute.Int("athena.writes", len(out.Writes)), attribute.Int64("athena.duration_micros", out.Duration.Microseconds()),
	)
	if out.Status != rules.OK {
		span.SetStatus(codes.Error, out.Code)
	}
	if err := r.engine.logRun(ctx, r, recordType, requestID, out); err != nil {
		return out, err
	}
	return out, nil
}

// ruleNow is the time handed to Rules as ctx.now. Tests replace it.
var ruleNow = time.Now

// logRun appends the Rule Run row. It uses the engine's own connection, not
// the document's transaction, so the row survives a rollback. It holds the
// input hash and counts, never Record values.
func (e ruleEngine) logRun(ctx context.Context, r ruleRun, recordType, requestID string, out rules.Outcome) error {
	id, err := uuid.NewV7()
	if err != nil {
		return internalError("write rule run", err)
	}
	messages, _ := json.Marshal(map[string][]ruleMessageJSON{"warnings": messagesJSON(out.Warnings), "logs": messagesJSON(out.Logs)})
	ctx = context.WithoutCancel(ctx)
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return internalError("write rule run", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO athena_rule_runs (id, rule_name, rule_version, rule_type, record_type, record_id, event, input_hash, outcome, code, message, steps, rows_read, rows_written, duration_micros, dry_run, actor, request_id, messages) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id.String(), r.rule.Key, r.rule.Version, string(r.rule.Type), nullText(recordType), nullText(r.recordID), nullText(r.event), out.InputHash, string(out.Status), out.Code, clipText(out.Message, 600),
		int64(out.Steps), out.Reads, len(out.Writes), out.Duration.Microseconds(), r.dryRun, actorName(ctx), requestID, string(messages))
	if err != nil {
		return internalError("write rule run", err)
	}
	if actor := auth.ActorID(ctx); actor != "" && !r.dryRun && out.Status != rules.OK && (r.rule.Type == rules.Validation || r.rule.Type == rules.Guard) {
		if err := notifications.Write(ctx, tx, actor, notifications.Event{Kind: notifications.SaveStopped, SubjectType: recordType, SubjectID: r.recordID, ActorID: actor}); err != nil {
			return internalError("notify about stopped save", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return internalError("write rule run", err)
	}
	return nil
}

type ruleMessageJSON struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

func messagesJSON(list []rules.Message) []ruleMessageJSON {
	out := make([]ruleMessageJSON, 0, len(list))
	for _, m := range list {
		out = append(out, ruleMessageJSON{Code: m.Code, Text: m.Text})
	}
	return out
}

func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func clipText(s string, limit int) string {
	if r := []rune(s); len(r) > limit {
		return string(r[:limit])
	}
	return s
}

// ---- posting --------------------------------------------------------------

// namedPlan is one posting Rule's plan with the Rule that built it, for
// errors that name the author.
type namedPlan struct {
	rule string
	plan posting.Plan
}

// postingPlans runs the enabled posting Rules of one Record Type and returns
// their plans in rule order. A Rule that rejects or fails refuses the submit
// with its outcome; a Rule that returns no plan is refused as authoring that
// cannot post. Documents without posting Rules post nothing. Two Rules may
// not name one book: one voucher revision, event and book owns one posting
// group, so a second plan for it could never post.
func (e ruleEngine) postingPlans(ctx context.Context, tx *sql.Tx, def recordTypeDefinition, recordID string, revision int64, doc *lifecycle.Document) ([]namedPlan, error) {
	list, err := loadRulesFor(ctx, tx, def.Name, string(lifecycle.OnSubmit))
	if err != nil {
		return nil, err
	}
	var plans []namedPlan
	books := map[string]string{}
	for _, rule := range list {
		if rule.Type != rules.Posting {
			continue
		}
		run := ruleRun{
			engine: e, rule: rule, def: &def, tx: tx, doc: doc, recordID: recordID, event: string(lifecycle.OnSubmit),
		}
		out, err := run.execute(ctx)
		if err != nil {
			return nil, err
		}
		if err := outcomeError(rule.Key, out); err != nil {
			return nil, err
		}
		if out.Plan == nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("The Rule %s returned no plan", rule.Key))
		}
		if first, dup := books[out.Plan.Book]; dup {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("The Rules %s and %s both post to book %q; merge them into one Rule", first, rule.Key, out.Plan.Book))
		}
		books[out.Plan.Book] = rule.Key
		plans = append(plans, namedPlan{rule: rule.Key, plan: voucherPlan(def.Name, recordID, revision, out.Plan)})
	}
	return plans, nil
}

// voucherPlan names the voucher, event and revision around a Rule's plan.
func voucherPlan(recordType, recordID string, revision int64, plan *rules.Plan) posting.Plan {
	out := posting.Plan{
		VoucherType: recordType, VoucherID: recordID, Revision: revision,
		Event: posting.EventSubmit, Book: plan.Book, PostingDate: plan.PostingDate,
		Lines: make([]posting.Line, len(plan.Lines)),
	}
	for i := range plan.Lines {
		line := &plan.Lines[i]
		out.Lines[i] = posting.Line{
			Company: line.Company, Account: line.Account,
			DebitBase: line.DebitBase, CreditBase: line.CreditBase,
			TxnCurrency: line.TxnCurrency, TxnDebit: line.TxnDebit, TxnCredit: line.TxnCredit,
			Quantity: line.Quantity, QuantityUOM: line.QuantityUOM,
			Dimensions: line.Dimensions, SourceType: line.SourceType, SourceID: line.SourceID,
		}
	}
	return out
}

// operationID derives the stable idempotency key of one post or reversal
// from its group identity. Retrying a submit or a cancel replays the first
// attempt instead of posting again; the hash keeps the key short whatever
// the voucher names.
func operationID(prefix, recordType, recordID string, revision int64, book string) string {
	joined := strings.Join([]string{recordType, recordID, fmt.Sprint(revision), book}, "\x00")
	sum := sha256.Sum256([]byte(joined))
	return prefix + ":" + hex.EncodeToString(sum[:])[:48]
}

// ---- host -----------------------------------------------------------------

// ruleHost is the Go side of a Rule's host functions. Every read is made as
// the acting user; every write is checked the way an API save is.
type ruleHost struct {
	ctx       context.Context
	tx        *sql.Tx
	principal *auth.Principal
	def       *recordTypeDefinition
	doc       *lifecycle.Document
}

// docRow converts the document to what the Rule sees.
func (h *ruleHost) docRow() rules.Row {
	row := rules.Row{Values: map[string]any{}, Hidden: map[string]bool{}}
	if h.doc == nil {
		return row
	}
	if h.def == nil {
		// A transform with no Record Type works on plain text and booleans.
		for key, value := range h.doc.Values {
			row.Values[key] = value
		}
		return row
	}
	row.Hidden = hiddenFields(h.principal, *h.def)
	for _, field := range h.def.Fields {
		if value, err := ruleValue(field, h.doc.Values[field.Name]); err == nil {
			row.Values[field.Name] = value
		}
	}
	for key, value := range h.doc.System {
		if _, own := row.Values[key]; own {
			continue
		}
		switch v := value.(type) {
		case int64, string, bool, nil:
			row.Values[key] = v
		case int32:
			row.Values[key] = int64(v)
		}
	}
	return row
}

// ruleValue converts a stored argument to a Rule value: exact decimals and
// money, ISO date text, never a float.
func ruleValue(field recordTypeField, arg any) (any, error) {
	value, err := argValue(field, arg)
	if err != nil {
		return nil, err
	}
	if t, ok := value.(time.Time); ok {
		return types.FormatDate(t), nil
	}
	return value, nil
}

// fieldFromRule converts a Rule value to the FieldValue the API would have
// received, so fieldArgument applies the same checks as for a user.
func fieldFromRule(field recordTypeField, value any) (*athenav1.FieldValue, error) {
	text := func(s string) *athenav1.FieldValue {
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: s}}
	}
	switch v := value.(type) {
	case nil:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_NullValue{NullValue: true}}, nil
	case bool:
		if field.Type != "Checkbox" {
			return nil, fmt.Errorf("%s is not a checkbox", field.Label)
		}
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_BoolValue{BoolValue: v}}, nil
	case string:
		switch field.Type {
		case "Checkbox", "Number", "Currency", "Quantity", "Money":
			return nil, fmt.Errorf("%s needs a %s value, not text", field.Label, field.Type)
		}
		return text(v), nil
	case types.Decimal:
		switch field.Type {
		case "Number", "Currency", "Quantity":
			return text(v.String()), nil
		}
		return nil, fmt.Errorf("%s does not take a decimal", field.Label)
	case types.Money:
		if field.Type != "Money" {
			return nil, fmt.Errorf("%s does not take money", field.Label)
		}
		if v.Currency.Code != field.Currency {
			return nil, fmt.Errorf("%s is in %s, not %s", field.Label, field.Currency, v.Currency.Code)
		}
		return text(v.Decimal().String()), nil
	}
	return nil, fmt.Errorf("%s: unsupported value", field.Label)
}

func hostError(code string, err error) error { return &rules.HostError{Code: code, Err: err} }

// Set changes a field of the document. State rules (a submitted document, a
// set-once field) are applied when the record service merges the writes.
func (h *ruleHost) Set(key string, value any) error {
	if h.def == nil || h.doc == nil {
		return hostError("invalid_field", errors.New("this Rule has no document to change"))
	}
	field, ok := fieldsByKey(*h.def)[key]
	if !ok {
		return hostError("invalid_field", fmt.Errorf("%s has no field %q", h.def.Label, key))
	}
	if hiddenFields(h.principal, *h.def)[key] {
		return hostError("permission_denied", fmt.Errorf("you do not have permission to change %s", field.Label))
	}
	if field.Formula != "" {
		return hostError("invalid_field", fmt.Errorf("%s is calculated by its formula", field.Label))
	}
	fv, err := fieldFromRule(field, value)
	if err != nil {
		return hostError("invalid", err)
	}
	arg, err := fieldArgument(field, fv)
	if err != nil {
		return hostError("invalid", errors.New(connectMessage(err)))
	}
	h.doc.Set(key, arg)
	return nil
}

func (h *ruleHost) Currency(code string) (types.Currency, bool) {
	if h.def == nil {
		return types.Currency{}, false
	}
	for _, field := range h.def.Fields {
		if field.Type == "Money" && field.Currency == code {
			if c, err := moneyCurrency(field); err == nil {
				return c, true
			}
		}
	}
	return types.Currency{}, false
}

// readable loads a Record Type the acting user may read.
func (h *ruleHost) readable(recordType string) (recordTypeDefinition, error) {
	if h.tx == nil {
		return recordTypeDefinition{}, hostError("invalid", errors.New("this Rule cannot read Records"))
	}
	if h.principal != nil && !h.principal.Can(recordType, auth.ActionRead) {
		return recordTypeDefinition{}, hostError("permission_denied", fmt.Errorf("you do not have permission to read %s", recordType))
	}
	def, err := loadRecordTypeFrom(h.ctx, h.tx, recordType)
	if err != nil {
		return recordTypeDefinition{}, hostError("not_found", errors.New(connectMessage(err)))
	}
	return def, nil
}

// recordRow converts a stored Record for a Rule, keeping the acting user's
// field levels.
func (h *ruleHost) recordRow(def recordTypeDefinition, record *athenav1.Record) (rules.Row, error) {
	row := rules.Row{Values: map[string]any{"id": record.GetId(), "revision": record.GetRevision()}, Hidden: hiddenFields(h.principal, def)}
	if def.kind() == kindChild {
		// A posting Rule reads its voucher's lines through find() filtered
		// on parent_id, so child rows carry their linkage as values.
		row.Values["parent_id"] = record.GetParentId()
		row.Values["idx"] = int64(record.GetIdx())
	}
	for _, field := range def.Fields {
		arg, err := storedArg(field, record.GetValues()[field.Name])
		if err != nil {
			return row, hostError("error", err)
		}
		if row.Values[field.Name], err = ruleValue(field, arg); err != nil {
			return row, hostError("error", err)
		}
	}
	for key, value := range systemValues(record, def) {
		if _, own := row.Values[key]; !own {
			row.Values[key] = value
		}
	}
	return row, nil
}

func (h *ruleHost) Get(recordType, id string) (rules.Row, bool, error) {
	def, err := h.readable(recordType)
	if err != nil {
		return rules.Row{}, false, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return rules.Row{}, false, nil
	}
	if err := authorizeRecord(h.ctx, h.tx, h.principal, def, id, auth.ActionRead); err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return rules.Row{}, false, nil
		}
		return rules.Row{}, false, hostError("permission_denied", errors.New(connectMessage(err)))
	}
	record, err := loadRecord(h.ctx, h.tx, def, id, "")
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return rules.Row{}, false, nil
		}
		return rules.Row{}, false, err
	}
	row, err := h.recordRow(def, record)
	return row, err == nil, err
}

// Find lists Records equal to the filters inside the acting user's read scope.
// It clamps limit to the row cap even though the Rule layer already does, so
// the SQL never asks for more than a run may hold.
func (h *ruleHost) Find(recordType string, filters map[string]any, limit int) ([]rules.Row, error) {
	if limit > rules.DefaultLimits.MaxRows {
		limit = rules.DefaultLimits.MaxRows
	}
	def, err := h.readable(recordType)
	if err != nil {
		return nil, err
	}
	scope, err := rowScopeFor(h.ctx, h.tx, h.principal, def, auth.ActionRead)
	if err != nil {
		return nil, err
	}
	hidden := hiddenFields(h.principal, def)
	where, args := []string{"1 = 1"}, []any{}
	if live := liveClause(dialect, def); live != "" {
		where = append(where, live)
	}
	if !scope.unrestricted {
		where = append(where, "("+scope.where+")")
		args = append(args, scope.args...)
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	byKey := fieldsByKey(def)
	for _, key := range keys {
		if key == "parent_id" {
			// Child rows filter on their linkage: find("bill_line",
			// {"parent_id": doc.id}) reads one voucher's lines. It is
			// structural, not a permission-leveled field.
			if def.kind() != kindChild {
				return nil, hostError("invalid_field", fmt.Errorf("%s has no parent to filter on", def.Label))
			}
			id, ok := filters[key].(string)
			if !ok {
				return nil, hostError("invalid", errors.New("parent_id filters on the document id text"))
			}
			where = append(where, "`parent_id` = ?")
			args = append(args, id)
			continue
		}
		field, ok := byKey[key]
		if !ok || hidden[key] {
			return nil, hostError("invalid_field", fmt.Errorf("%s has no field %q to filter on", def.Label, key))
		}
		fv, err := fieldFromRule(field, filters[key])
		if err != nil {
			return nil, hostError("invalid", err)
		}
		arg, err := fieldArgument(field, fv)
		if err != nil {
			return nil, hostError("invalid", errors.New(connectMessage(err)))
		}
		column := dialect.QuoteIdent(key)
		if arg == nil {
			where = append(where, column+" IS NULL")
			continue
		}
		where = append(where, column+" = ?")
		args = append(args, arg)
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY `id` LIMIT ?", selectColumns(def), dialect.QuoteIdent(recordTypeTableName(def.Name)), strings.Join(where, " AND "))
	rows, err := h.tx.QueryContext(h.ctx, query, append(args, limit)...)
	if err != nil {
		return nil, internalError("find records", err)
	}
	defer rows.Close()
	var out []rules.Row
	for rows.Next() {
		record, err := scanRecord(rows, def)
		if err != nil {
			return nil, internalError("read record", err)
		}
		row, err := h.recordRow(def, record)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ---- documents in flight --------------------------------------------------

// pendingDocument is the document as an event sees it: the stored values (none
// on create) with the pending changes laid over them.
func pendingDocument(ctx context.Context, d recordTypeDefinition, id string, current *athenav1.Record, columns []string, args []any) (*lifecycle.Document, error) {
	doc := &lifecycle.Document{Values: map[string]any{}, System: newRecordSystemValues(ctx, d)}
	if current != nil {
		stored, err := recordArgs(d, current)
		if err != nil {
			return nil, err
		}
		doc.Values, doc.System = stored, systemValues(current, d)
	}
	doc.System["id"] = id
	for i, column := range columns {
		doc.Values[columnKey(column)] = args[i]
	}
	return doc, nil
}

// mergeRuleWrites applies what hooks set to the columns about to be written.
// A field the user also changed takes the Rule's value. A field only the
// Rule changed is checked like a user's change: a submitted document keeps
// fields that do not allow edits after submit, and a set-once field that
// already has a value stays.
func mergeRuleWrites(d recordTypeDefinition, current *athenav1.Record, doc *lifecycle.Document, columns []string, args []any, changed []string) ([]string, []any, []string, error) {
	fields := fieldsByKey(d)
	for _, key := range doc.Written() {
		arg := doc.Writes[key]
		found := false
		for i, column := range columns {
			if columnKey(column) == key {
				args[i], found = arg, true
			}
		}
		if found {
			continue
		}
		field := fields[key]
		if current != nil {
			if sameValue(field, current.GetValues()[key], arg) {
				continue
			}
			if d.Submittable && lifecycle.State(current.GetDocstate()) == lifecycle.Submitted && !field.AllowOnSubmit {
				return nil, nil, nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("%s cannot be changed after the document is submitted", field.Label))
			}
			if stored := current.GetValues()[key]; field.SetOnce && !stored.GetNullValue() && stored.GetKind() != nil {
				return nil, nil, nil, invalid(fmt.Sprintf("%s can be set once and cannot change", field.Label))
			}
		}
		columns = append(columns, dialect.QuoteIdent(key))
		args = append(args, arg)
		changed = append(changed, key)
	}
	return columns, args, changed, nil
}
