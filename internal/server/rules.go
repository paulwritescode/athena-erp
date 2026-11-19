package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/rules"
	"athena-erp/internal/types"

	"connectrpc.com/connect/v2"
)

// ruleKeyPattern limits a Rule key to the width of athena_rules.name.
var ruleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

const (
	maxRuleLabel    = 255
	maxRulePriority = 1000
	defaultRuns     = 50
	maxRuns         = 200
	// ruleAuditType is the Audit Log record type of Rule changes, next to
	// athena:file. The row holds hashes and attributes, never the source.
	ruleAuditType = "athena:rule"
)

// ruleService implements athena.v1.RuleService. The gate checks rules:read or
// rules:write; the handlers check again so a call that bypasses the gate is
// still refused.
type ruleService struct {
	db     *sql.DB
	engine ruleEngine
}

func newRuleService(db *sql.DB) ruleService { return ruleService{db: db, engine: ruleEngine{db: db}} }

func (s ruleService) ready() error {
	if s.db == nil {
		return connect.NewError(connect.CodeUnavailable, "the database is not configured")
	}
	return nil
}

func definitionMessage(r storedRuleRow) *athenav1.RuleDefinition {
	return &athenav1.RuleDefinition{Key: r.key, Label: r.label, RuleType: r.ruleType, RecordTypeKey: r.recordType, Event: r.event, Priority: int32(r.priority), Enabled: r.enabled, Source: r.source}
}

// storedRuleRow is a whole athena_rules or athena_rule_versions row.
type storedRuleRow struct {
	key, label, ruleType, recordType, event string
	priority                                int
	enabled                                 bool
	version                                 int64
	source, hash, actor                     string
	at                                      time.Time
}

func (r storedRuleRow) message() *athenav1.Rule {
	return &athenav1.Rule{Definition: definitionMessage(r), Version: r.version, SourceHash: r.hash, ModifiedBy: r.actor, ModifiedAt: r.at.UTC().Format(time.RFC3339)}
}

const ruleColumns = "name, label, rule_type, COALESCE(record_type, ''), COALESCE(event, ''), priority, enabled, version, source, source_hash, modified_by, updated_at"

type scanner interface{ Scan(...any) error }

func scanRule(row scanner) (storedRuleRow, error) {
	var r storedRuleRow
	err := row.Scan(&r.key, &r.label, &r.ruleType, &r.recordType, &r.event, &r.priority, &r.enabled, &r.version, &r.source, &r.hash, &r.actor, &r.at)
	return r, err
}

func (s ruleService) ListRules(ctx context.Context, _ *athenav1.ListRulesRequest) (*athenav1.ListRulesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesRead); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+ruleColumns+" FROM athena_rules ORDER BY name")
	if err != nil {
		return nil, internalError("list rules", err)
	}
	defer rows.Close()
	res := &athenav1.ListRulesResponse{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, internalError("read rule", err)
		}
		res.Rules = append(res.Rules, r.message())
	}
	return res, rows.Err()
}

func (s ruleService) GetRule(ctx context.Context, req *athenav1.GetRuleRequest) (*athenav1.GetRuleResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesRead); err != nil {
		return nil, err
	}
	r, err := scanRule(s.db.QueryRowContext(ctx, "SELECT "+ruleColumns+" FROM athena_rules WHERE name = ?", req.GetKey()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Sprintf("the Rule %q does not exist", req.GetKey()))
	}
	if err != nil {
		return nil, internalError("read rule", err)
	}
	return &athenav1.GetRuleResponse{Rule: r.message()}, nil
}

// checkedRule is a definition that passed validation, with its Record Type.
type checkedRule struct {
	def *athenav1.RuleDefinition
	rt  *recordTypeDefinition
}

// checkDefinition validates a Rule definition and compiles its source. Every
// failure is an invalid-argument error; a source problem carries its line.
func checkDefinition(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, def *athenav1.RuleDefinition) (checkedRule, error) {
	if !ruleKeyPattern.MatchString(def.GetKey()) {
		return checkedRule{}, invalid("a Rule key starts with a lowercase letter and holds lowercase letters, digits and underscores, up to 63 characters")
	}
	if def.GetLabel() == "" || len([]rune(def.GetLabel())) > maxRuleLabel {
		return checkedRule{}, invalid("a Rule needs a label of up to 255 characters")
	}
	kind := rules.Type(def.GetRuleType())
	if !kind.Valid() {
		return checkedRule{}, invalid(fmt.Sprintf("the Rule type must be one of %v; posting, reaction and scheduled Rules arrive with later phases", rules.Types))
	}
	if def.GetPriority() < 0 || def.GetPriority() > maxRulePriority {
		return checkedRule{}, invalid("the priority is between 0 and 1000")
	}
	if def.GetSource() == "" {
		return checkedRule{}, invalid("the Rule has no source")
	}
	out := checkedRule{def: def}
	var refs rules.Refs
	switch {
	case kind == rules.Transform && def.GetEvent() != "":
		return out, invalid("a transform runs when called and has no event")
	case kind != rules.Transform && !contains(kind.Events(), def.GetEvent()):
		return out, invalid(fmt.Sprintf("a %s Rule runs at %v, not %q", kind, kind.Events(), def.GetEvent()))
	}
	if def.GetRecordTypeKey() == "" && kind != rules.Transform {
		return out, invalid("a Rule needs the Record Type it belongs to")
	}
	if def.GetRecordTypeKey() != "" {
		rt, err := loadRecordTypeFrom(ctx, q, def.GetRecordTypeKey())
		if err != nil {
			return out, err
		}
		if rt.kind() != kindDocument {
			return out, invalid(fmt.Sprintf("%s is not a document; Rules run on document events", rt.Label))
		}
		out.rt, refs = &rt, ruleRefs(rt)
	}
	if _, err := rules.Compile(kind, def.GetSource(), refs); err != nil {
		return out, invalid(err.Error())
	}
	return out, nil
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func (s ruleService) SaveRule(ctx context.Context, req *athenav1.SaveRuleRequest) (res *athenav1.SaveRuleResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "save_rule", req.GetDefinition().GetKey())
	defer func() { end(err) }()
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesWrite); err != nil {
		return nil, err
	}
	def := req.GetDefinition()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("save rule", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := checkDefinition(ctx, tx, def); err != nil {
		return nil, err
	}
	current, err := scanRule(tx.QueryRowContext(ctx, "SELECT "+ruleColumns+" FROM athena_rules WHERE name = ? FOR UPDATE", def.GetKey()))
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, internalError("read rule", err)
	}
	switch {
	case exists && current.version != req.GetExpectedVersion():
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this Rule; reload it and try again")
	case !exists && req.GetExpectedVersion() != 0:
		return nil, connect.NewError(connect.CodeAborted, "this Rule no longer exists; reload and try again")
	}
	hash := rules.SourceHash(def.GetSource())
	next := storedRuleRow{key: def.GetKey(), label: def.GetLabel(), ruleType: def.GetRuleType(), recordType: def.GetRecordTypeKey(), event: def.GetEvent(), priority: int(def.GetPriority()), enabled: def.GetEnabled(), version: 1, source: def.GetSource(), hash: hash, actor: actorName(ctx)}
	if exists {
		// A save that changes nothing is not a new version.
		same := current.label == next.label && current.ruleType == next.ruleType && current.recordType == next.recordType && current.event == next.event && current.priority == next.priority && current.enabled == next.enabled && current.hash == hash
		if same {
			return &athenav1.SaveRuleResponse{Rule: current.message()}, nil
		}
		next.version = current.version + 1
		_, err = tx.ExecContext(ctx, "UPDATE athena_rules SET label = ?, rule_type = ?, record_type = ?, event = ?, priority = ?, enabled = ?, version = ?, source = ?, source_hash = ?, modified_by = ?, updated_at = CURRENT_TIMESTAMP(3) WHERE name = ?",
			next.label, next.ruleType, nullText(next.recordType), nullText(next.event), next.priority, next.enabled, next.version, next.source, hash, next.actor, next.key)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO athena_rules (name, label, rule_type, record_type, event, priority, enabled, version, source, source_hash, modified_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			next.key, next.label, next.ruleType, nullText(next.recordType), nullText(next.event), next.priority, next.enabled, next.version, next.source, hash, next.actor)
	}
	if err != nil {
		return nil, writeError("save rule", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO athena_rule_versions (rule_name, version, label, rule_type, record_type, event, priority, enabled, source, source_hash, actor, request_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		next.key, next.version, next.label, next.ruleType, nullText(next.recordType), nullText(next.event), next.priority, next.enabled, next.source, hash, next.actor, requestID); err != nil {
		return nil, writeError("save rule", err)
	}
	action := "rule_created"
	changes := []audit.Change{{Field: "source_hash", New: &hash}, {Field: "rule_type", New: ptr(next.ruleType)}, {Field: "record_type", New: ptr(next.recordType)}, {Field: "event", New: ptr(next.event)}, {Field: "enabled", New: ptr(fmt.Sprint(next.enabled))}}
	if exists {
		action = "rule_saved"
		changes[0].Old = ptr(current.hash)
	}
	if err := writeAudit(ctx, tx, ruleAuditType, next.key, next.version, action, changes); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("save rule", err)
	}
	saved, err := s.GetRule(ctx, &athenav1.GetRuleRequest{Key: next.key})
	if err != nil {
		return nil, err
	}
	return &athenav1.SaveRuleResponse{Rule: saved.GetRule()}, nil
}

func (s ruleService) DeleteRule(ctx context.Context, req *athenav1.DeleteRuleRequest) (res *athenav1.DeleteRuleResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "delete_rule", req.GetKey())
	defer func() { end(err) }()
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesWrite); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("delete rule", err)
	}
	defer func() { _ = tx.Rollback() }()
	var version int64
	if err := tx.QueryRowContext(ctx, "SELECT version FROM athena_rules WHERE name = ? FOR UPDATE", req.GetKey()).Scan(&version); errors.Is(err, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Sprintf("the Rule %q does not exist", req.GetKey()))
	} else if err != nil {
		return nil, internalError("read rule", err)
	}
	if version != req.GetExpectedVersion() {
		return nil, connect.NewError(connect.CodeAborted, "someone else changed this Rule; reload it and try again")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM athena_rules WHERE name = ?", req.GetKey()); err != nil {
		return nil, writeError("delete rule", err)
	}
	if err := writeAudit(ctx, tx, ruleAuditType, req.GetKey(), version+1, "rule_deleted", nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, internalError("delete rule", err)
	}
	return &athenav1.DeleteRuleResponse{}, nil
}

func (s ruleService) ListRuleVersions(ctx context.Context, req *athenav1.ListRuleVersionsRequest) (*athenav1.ListRuleVersionsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesRead); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT rule_name, label, rule_type, COALESCE(record_type, ''), COALESCE(event, ''), priority, enabled, version, source, source_hash, actor, created_at FROM athena_rule_versions WHERE rule_name = ? ORDER BY version DESC", req.GetKey())
	if err != nil {
		return nil, internalError("list rule versions", err)
	}
	defer rows.Close()
	res := &athenav1.ListRuleVersionsResponse{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, internalError("read rule version", err)
		}
		res.Versions = append(res.Versions, &athenav1.RuleVersion{Version: r.version, Definition: definitionMessage(r), SourceHash: r.hash, Actor: r.actor, CreatedAt: r.at.UTC().Format(time.RFC3339)})
	}
	return res, rows.Err()
}

// SimulateRule runs a Rule against a document in a transaction that is always
// rolled back. Reads are made as the acting user, so a simulation shows only
// what that user may see.
func (s ruleService) SimulateRule(ctx context.Context, req *athenav1.SimulateRuleRequest) (res *athenav1.SimulateRuleResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "simulate_rule", req.GetDefinition().GetKey())
	defer func() { end(err) }()
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesWrite); err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internalError("simulate rule", err)
	}
	defer func() { _ = tx.Rollback() }()
	checked, err := checkDefinition(ctx, tx, req.GetDefinition())
	if err != nil {
		return nil, err
	}
	def := req.GetDefinition()
	version := int64(0)
	if err := tx.QueryRowContext(ctx, "SELECT version FROM athena_rules WHERE name = ?", def.GetKey()).Scan(&version); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, internalError("read rule", err)
	}
	run := ruleRun{
		engine: s.engine, rule: storedRule{Key: def.GetKey(), Version: version, Type: rules.Type(def.GetRuleType()), Event: def.GetEvent(), Priority: int(def.GetPriority())},
		def: checked.rt, tx: tx, event: def.GetEvent(), dryRun: true, source: def.GetSource(), recordID: req.GetRecordId(),
	}
	doc, err := simulationDocument(ctx, tx, p, checked.rt, req)
	if err != nil {
		return nil, err
	}
	before := map[string]any{}
	for key, value := range doc.Values {
		before[key] = value
	}
	run.doc = doc
	out, err := run.execute(ctx)
	if err != nil {
		return nil, err
	}
	res = outcomeMessage(out)
	for _, key := range doc.Written() {
		field := fieldsByKey(*checked.rt)[key]
		res.Changes = append(res.Changes, &athenav1.RuleChange{Field: key, OldValue: argText(field, before[key]), NewValue: argText(field, doc.Writes[key])})
	}
	for _, w := range out.Result {
		if res.Result == nil {
			res.Result = map[string]*athenav1.FieldValue{}
		}
		res.Result[w.Field] = resultValue(w.Value)
	}
	return res, nil
}

// simulationDocument builds the document a simulation runs on: the stored
// Record the acting user may read (when record_id is set) with the supplied
// values laid over it. A transform with no Record Type gets the values as
// plain text.
func simulationDocument(ctx context.Context, tx *sql.Tx, p *auth.Principal, d *recordTypeDefinition, req *athenav1.SimulateRuleRequest) (*lifecycle.Document, error) {
	doc := &lifecycle.Document{Values: map[string]any{}, System: map[string]any{}}
	if d == nil {
		for key, value := range req.GetValues() {
			switch v := value.GetKind().(type) {
			case *athenav1.FieldValue_StringValue:
				doc.Values[key] = v.StringValue
			case *athenav1.FieldValue_BoolValue:
				doc.Values[key] = v.BoolValue
			default:
				doc.Values[key] = nil
			}
		}
		return doc, nil
	}
	var current *athenav1.Record
	if id := req.GetRecordId(); id != "" {
		if err := authorizeRecord(ctx, tx, p, *d, id, auth.ActionRead); err != nil {
			return nil, err
		}
		var err error
		if current, err = loadRecord(ctx, tx, *d, id, ""); err != nil {
			return nil, err
		}
	}
	hidden := hiddenFields(p, *d)
	if err := checkWritable(hidden, *d, req.GetValues()); err != nil {
		return nil, err
	}
	// Simulated values need not form a complete document, so required fields
	// are not enforced: bind them as an update.
	var columns []string
	var args []any
	if len(req.GetValues()) > 0 {
		var err error
		if columns, args, err = bindValues(*d, req.GetValues(), false); err != nil {
			return nil, err
		}
	}
	return pendingDocument(ctx, *d, req.GetRecordId(), current, columns, args)
}

// argText renders a stored argument for the diff, or nil when empty.
func argText(field recordTypeField, arg any) *string {
	switch v := arg.(type) {
	case nil:
		return nil
	case int64:
		text := moneyText(field, v)
		return &text
	case bool:
		text := fmt.Sprint(v)
		return &text
	case string:
		return &v
	}
	text := fmt.Sprint(arg)
	return &text
}

// resultValue renders what a transform returned.
func resultValue(value any) *athenav1.FieldValue {
	switch v := value.(type) {
	case nil:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_NullValue{NullValue: true}}
	case bool:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_BoolValue{BoolValue: v}}
	case types.Decimal:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: v.String()}}
	case types.Money:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: v.Decimal().String()}}
	}
	return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: fmt.Sprint(value)}}
}

func outcomeMessage(out rules.Outcome) *athenav1.SimulateRuleResponse {
	res := &athenav1.SimulateRuleResponse{
		Outcome: string(out.Status), Code: out.Code, Message: out.Message, Steps: int64(out.Steps), Reads: int64(out.Reads),
		DurationMicros: out.Duration.Microseconds(), InputHash: out.InputHash,
	}
	for _, m := range out.Warnings {
		res.Warnings = append(res.Warnings, &athenav1.RuleMessage{Code: m.Code, Text: m.Text})
	}
	for _, m := range out.Logs {
		res.Logs = append(res.Logs, &athenav1.RuleMessage{Code: m.Code, Text: m.Text})
	}
	return res
}

func (s ruleService) ListRuleRuns(ctx context.Context, req *athenav1.ListRuleRunsRequest) (*athenav1.ListRuleRunsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionRulesRead); err != nil {
		return nil, err
	}
	size := int(req.GetPageSize())
	if size <= 0 {
		size = defaultRuns
	}
	size = min(size, maxRuns)
	where, args := []string{"1 = 1"}, []any{}
	if req.GetPageToken() != "" {
		after, err := decodeCursor(req.GetPageToken(), "rule_runs")
		if err != nil {
			return nil, err
		}
		where, args = append(where, "id < ?"), append(args, after)
	}
	for _, f := range []struct{ column, value string }{{"rule_name", req.GetRuleKey()}, {"record_type", req.GetRecordTypeKey()}, {"record_id", req.GetRecordId()}, {"request_id", req.GetRequestId()}, {"outcome", req.GetOutcome()}} {
		if f.value != "" {
			where, args = append(where, f.column+" = ?"), append(args, f.value)
		}
	}
	query := "SELECT id, rule_name, rule_version, rule_type, COALESCE(record_type, ''), COALESCE(record_id, ''), COALESCE(event, ''), input_hash, outcome, code, message, steps, rows_read, rows_written, duration_micros, dry_run, actor, COALESCE(request_id, ''), messages, created_at FROM athena_rule_runs WHERE "
	for i, w := range where {
		if i > 0 {
			query += " AND "
		}
		query += w
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY id DESC LIMIT ?", append(args, size+1)...)
	if err != nil {
		return nil, internalError("list rule runs", err)
	}
	defer rows.Close()
	res := &athenav1.ListRuleRunsResponse{}
	for rows.Next() {
		var r athenav1.RuleRun
		var messages []byte
		var at time.Time
		if err := rows.Scan(&r.Id, &r.RuleKey, &r.RuleVersion, &r.RuleType, &r.RecordTypeKey, &r.RecordId, &r.Event, &r.InputHash, &r.Outcome, &r.Code, &r.Message, &r.Steps, &r.Reads, &r.Writes, &r.DurationMicros, &r.DryRun, &r.Actor, &r.RequestId, &messages, &at); err != nil {
			return nil, internalError("read rule run", err)
		}
		r.CreatedAt = at.UTC().Format(time.RFC3339Nano)
		var parsed map[string][]ruleMessageJSON
		if err := json.Unmarshal(messages, &parsed); err == nil {
			for _, m := range parsed["warnings"] {
				r.Warnings = append(r.Warnings, &athenav1.RuleMessage{Code: m.Code, Text: m.Text})
			}
			for _, m := range parsed["logs"] {
				r.Logs = append(r.Logs, &athenav1.RuleMessage{Code: m.Code, Text: m.Text})
			}
		}
		res.Runs = append(res.Runs, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, internalError("list rule runs", err)
	}
	if len(res.Runs) > size {
		res.Runs = res.Runs[:size]
		res.NextPageToken = encodeCursor("rule_runs", res.Runs[size-1].Id)
	}
	return res, nil
}
