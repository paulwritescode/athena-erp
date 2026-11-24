package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/audit"
	"athena-erp/internal/auth"
	"athena-erp/internal/expr"
	"athena-erp/internal/lifecycle"
	"athena-erp/internal/posting"
	"athena-erp/internal/rules"
	"athena-erp/internal/types"

	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

// ruleKeyPattern limits a Rule key to the width of athena_rules.name.
var ruleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

const (
	maxRuleLabel    = 255
	maxRulePriority = 1000
	maxRuleMessage  = 500
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
	sets := []*athenav1.DerivationSet{}
	if r.sets != "" {
		var parsed []builderSet
		if err := json.Unmarshal([]byte(r.sets), &parsed); err == nil {
			for _, s := range parsed {
				sets = append(sets, &athenav1.DerivationSet{Field: s.Field, ValueExpr: s.ValueExpr})
			}
		}
	}
	return &athenav1.RuleDefinition{Key: r.key, Label: r.label, RuleType: r.ruleType, RecordTypeKey: r.recordType, Event: r.event, Priority: int32(r.priority), Enabled: r.enabled, Source: r.source, ConditionTree: r.tree, ConditionExpr: r.expr, Message: r.notetext, Sets: sets}
}

// storedRuleRow is a whole athena_rules or athena_rule_versions row.
type storedRuleRow struct {
	key, label, ruleType, recordType, event string
	priority                                int
	enabled                                 bool
	version                                 int64
	source, hash, actor                     string
	tree, expr, notetext, sets              string
	at                                      time.Time
}

// builderSet is one derivation assignment in storage JSON.
type builderSet struct {
	Field     string `json:"field"`
	ValueExpr string `json:"value_expr"`
}

func (r storedRuleRow) message() *athenav1.Rule {
	return &athenav1.Rule{Definition: definitionMessage(r), Version: r.version, SourceHash: r.hash, ModifiedBy: r.actor, ModifiedAt: r.at.UTC().Format(time.RFC3339)}
}

const ruleColumns = "name, label, rule_type, COALESCE(record_type, ''), COALESCE(event, ''), priority, enabled, version, source, source_hash, modified_by, updated_at, COALESCE(condition_tree, ''), COALESCE(condition_expr, ''), COALESCE(message, ''), COALESCE(sets, '')"

type scanner interface{ Scan(...any) error }

func scanRule(row scanner) (storedRuleRow, error) {
	var r storedRuleRow
	err := row.Scan(&r.key, &r.label, &r.ruleType, &r.recordType, &r.event, &r.priority, &r.enabled, &r.version, &r.source, &r.hash, &r.actor, &r.at, &r.tree, &r.expr, &r.notetext, &r.sets)
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

// checkedRule is a definition that passed validation, with its Record Type
// and, for builder Rules, the compiled condition.
type checkedRule struct {
	def      *athenav1.RuleDefinition
	rt       *recordTypeDefinition
	compiled string
}

// checkDefinition validates a Rule definition and compiles its source or,
// for builder Rules, its condition tree. Every failure is an
// invalid-argument error; a source problem carries its line.
func checkDefinition(ctx context.Context, q *sql.Tx, def *athenav1.RuleDefinition) (checkedRule, error) {
	if !ruleKeyPattern.MatchString(def.GetKey()) {
		return checkedRule{}, invalid("a Rule key starts with a lowercase letter and holds lowercase letters, digits and underscores, up to 63 characters")
	}
	if def.GetLabel() == "" || len([]rune(def.GetLabel())) > maxRuleLabel {
		return checkedRule{}, invalid("a Rule needs a label of up to 255 characters")
	}
	kind := rules.Type(def.GetRuleType())
	if !kind.Valid() {
		return checkedRule{}, invalid(fmt.Sprintf("the Rule type must be one of %v; reaction and scheduled Rules arrive with later phases", rules.Types))
	}
	if def.GetPriority() < 0 || def.GetPriority() > maxRulePriority {
		return checkedRule{}, invalid("the priority is between 0 and 1000")
	}
	builder := strings.TrimSpace(def.GetConditionTree()) != ""
	if builder && def.GetSource() != "" {
		return checkedRule{}, invalid("write the Rule in Starlark or the builder, not both")
	}
	if !builder && def.GetSource() == "" {
		return checkedRule{}, invalid("the Rule has no source")
	}
	if !builder && (def.GetMessage() != "" || len(def.GetSets()) > 0) {
		return checkedRule{}, invalid("a message and set fields need a builder condition")
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
	if !builder {
		if _, err := rules.Compile(kind, def.GetSource(), refs); err != nil {
			return out, invalid(err.Error())
		}
		return out, nil
	}
	return checkBuilderDefinition(ctx, q, out, kind)
}

// checkBuilderDefinition validates a builder Rule: the tree compiles
// against the Record Type, validation and guard carry a message, and every
// derivation set names a field with a fitting expression.
func checkBuilderDefinition(ctx context.Context, q querier, out checkedRule, kind rules.Type) (checkedRule, error) {
	if kind != rules.Validation && kind != rules.Derivation && kind != rules.Guard {
		return out, invalid("only validation, derivation and guard Rules come from the builder")
	}
	if out.rt == nil {
		return out, invalid("a builder Rule needs the Record Type it belongs to")
	}
	def := out.def
	var tree expr.Tree
	if err := json.Unmarshal([]byte(def.GetConditionTree()), &tree); err != nil {
		return out, invalid("the condition tree is not valid JSON")
	}
	env, err := newExprEnvironment(ctx, q, *out.rt, nil)
	if err != nil {
		return out, err
	}
	vars := make(map[string]expr.Variable, len(env.vars))
	for _, v := range env.vars {
		vars[v.Name] = v
	}
	compiled, err := expr.CompileTree(tree, vars)
	if err != nil {
		return out, invalid(err.Error())
	}
	out.compiled = compiled
	message := strings.TrimSpace(def.GetMessage())
	if kind == rules.Derivation {
		if message != "" {
			return out, invalid("a derivation sets fields instead of messaging")
		}
		if len(def.GetSets()) == 0 {
			return out, invalid("a derivation sets at least one field")
		}
		for _, set := range def.GetSets() {
			field, ok := env.byField[set.GetField()]
			if !ok {
				return out, invalid(fmt.Sprintf("unknown field %q", set.GetField()))
			}
			program, err := env.env.Compile(set.GetValueExpr())
			if err != nil {
				return out, invalid(fmt.Sprintf("%s: %s", set.GetField(), err.Error()))
			}
			if !formulaResultFits(field, program.Result) {
				return out, invalid(fmt.Sprintf("%s: the value does not fit", set.GetField()))
			}
		}
		return out, nil
	}
	if message == "" {
		return out, invalid("say what the Rule reports when it fires")
	}
	if len([]rune(message)) > maxRuleMessage {
		return out, invalid("the message holds up to 500 characters")
	}
	if len(def.GetSets()) > 0 {
		return out, invalid("only a derivation sets fields")
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
	checked, err := checkDefinition(ctx, tx, def)
	if err != nil {
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
	executed := def.GetSource()
	setsJSON := ""
	if checked.compiled != "" {
		executed = checked.compiled
		sets := make([]builderSet, 0, len(def.GetSets()))
		for _, s := range def.GetSets() {
			sets = append(sets, builderSet{Field: s.GetField(), ValueExpr: s.GetValueExpr()})
		}
		raw, err := json.Marshal(sets)
		if err != nil {
			return nil, err
		}
		setsJSON = string(raw)
	}
	hash := rules.SourceHash(executed)
	enabled, diff := def.GetEnabled(), ""
	if rules.Type(def.GetRuleType()) == rules.Posting {
		// Activating a changed posting Rule replays its fixtures; a
		// changed plan lands disabled until a named user approves it by
		// saving again with enabled (R6.16).
		if enabled, diff, err = s.gatePostingSave(ctx, tx, requestID, def, checked.rt, current, exists, hash); err != nil {
			return nil, err
		}
	}
	next := storedRuleRow{key: def.GetKey(), label: def.GetLabel(), ruleType: def.GetRuleType(), recordType: def.GetRecordTypeKey(), event: def.GetEvent(), priority: int(def.GetPriority()), enabled: enabled, version: 1, source: def.GetSource(), hash: hash, actor: actorName(ctx), tree: def.GetConditionTree(), expr: checked.compiled, notetext: def.GetMessage(), sets: setsJSON}
	if exists {
		// A save that changes nothing is not a new version.
		same := current.label == next.label && current.ruleType == next.ruleType && current.recordType == next.recordType && current.event == next.event && current.priority == next.priority && current.enabled == next.enabled && current.hash == hash &&
			current.tree == next.tree && current.notetext == next.notetext && current.sets == next.sets
		if same {
			return &athenav1.SaveRuleResponse{Rule: current.message()}, nil
		}
		next.version = current.version + 1
		_, err = tx.ExecContext(ctx, "UPDATE athena_rules SET label = ?, rule_type = ?, record_type = ?, event = ?, priority = ?, enabled = ?, version = ?, source = ?, source_hash = ?, modified_by = ?, updated_at = CURRENT_TIMESTAMP(3), condition_tree = ?, condition_expr = ?, message = ?, sets = ? WHERE name = ?",
			next.label, next.ruleType, nullText(next.recordType), nullText(next.event), next.priority, next.enabled, next.version, next.source, hash, next.actor, nullText(next.tree), nullText(next.expr), nullText(next.notetext), nullText(next.sets), next.key)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO athena_rules (name, label, rule_type, record_type, event, priority, enabled, version, source, source_hash, modified_by, condition_tree, condition_expr, message, sets) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			next.key, next.label, next.ruleType, nullText(next.recordType), nullText(next.event), next.priority, next.enabled, next.version, next.source, hash, next.actor, nullText(next.tree), nullText(next.expr), nullText(next.notetext), nullText(next.sets))
	}
	if err != nil {
		return nil, writeError("save rule", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO athena_rule_versions (rule_name, version, label, rule_type, record_type, event, priority, enabled, source, source_hash, actor, request_id, condition_tree, condition_expr, message, sets) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		next.key, next.version, next.label, next.ruleType, nullText(next.recordType), nullText(next.event), next.priority, next.enabled, next.source, hash, next.actor, requestID, nullText(next.tree), nullText(next.expr), nullText(next.notetext), nullText(next.sets)); err != nil {
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
	if diff != "" {
		// The new source is stored disabled: the approver reads the diff,
		// then saves the same source with enabled, and that save records
		// their name as the approval.
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Sprintf("saved as disabled version %d: the plan changed since version %d, so review it and save again with enabled to approve:\n%s", next.version, current.version, diff))
	}
	saved, err := s.GetRule(ctx, &athenav1.GetRuleRequest{Key: next.key})
	if err != nil {
		return nil, err
	}
	return &athenav1.SaveRuleResponse{Rule: saved.GetRule()}, nil
}

// gatePostingSave enforces the activation rule of a posting Rule (R6.16). It
// returns the enabled value to store: activating (creating enabled,
// enabling, or changing the source of an enabled Rule) replays the Rule's
// fixtures against the current masters, and when the new source plans
// differently than the previous version the save lands disabled with the
// plan diff to review. Saving the same source with enabled afterwards is the
// named approval: the versions and the audit row record who approved it.
// Deactivating is always safe and replays nothing; a Rule that stays
// disabled can be drafted freely and replays when it is enabled.
func (s ruleService) gatePostingSave(ctx context.Context, tx *sql.Tx, requestID string, def *athenav1.RuleDefinition, rt *recordTypeDefinition, current storedRuleRow, exists bool, hash string) (bool, string, error) {
	if !def.GetEnabled() {
		return false, "", nil
	}
	sourceChanged := !exists || current.hash != hash
	if activating := !exists || !current.enabled || sourceChanged; !activating {
		return true, "", nil
	}
	replayed, err := s.replayPostingFixtures(ctx, tx, requestID, def, rt, def.GetSource(), current.version)
	if err != nil {
		return false, "", err
	}
	if !sourceChanged || !exists {
		return true, "", nil
	}
	previous, err := s.replayPostingFixtures(ctx, tx, requestID, def, rt, current.source, current.version)
	if err != nil {
		return false, fmt.Sprintf("the previous version no longer replays, so its plan cannot be compared: %s", connectMessage(err)), nil
	}
	if diff := diffFixturePlans(previous, replayed); diff != "" {
		return false, diff, nil
	}
	return true, "", nil
}

// fixturePlan is one fixture's plan after validation.
type fixturePlan struct {
	name string
	plan posting.Plan
}

// replayPostingFixtures runs source's fixtures through post() and validates
// every plan against the masters in tx, so an activated Rule proves it posts
// before any voucher depends on it. Fixture children are staged in tx under
// a synthetic document id and removed before returning; a refused save rolls
// back whatever is still staged. Reads run as the saving user, like the
// submit-time runs they stand in for.
func (s ruleService) replayPostingFixtures(ctx context.Context, tx *sql.Tx, requestID string, def *athenav1.RuleDefinition, rt *recordTypeDefinition, source string, version int64) ([]fixturePlan, error) {
	program, err := rules.Compile(rules.Posting, source, ruleRefs(*rt))
	if err != nil {
		return nil, invalid(err.Error())
	}
	fixtures, err := program.RunFixtures(ctx, rules.DefaultLimits, (&ruleHost{def: rt}).Currency)
	if err != nil {
		return nil, invalid("fixtures: " + err.Error())
	}
	children, err := childRecordTypes(ctx, tx, rt.Name)
	if err != nil {
		return nil, err
	}
	byChild := make(map[string]recordTypeDefinition, len(children))
	for _, child := range children {
		byChild[child.Name] = child
	}
	var plans []fixturePlan
	for i, fixture := range fixtures {
		docID := "fixture:" + def.GetKey() + ":" + fmt.Sprint(i)
		fieldValues, err := fixtureFieldValues(*rt, fixture.Name, fixture.Values)
		if err != nil {
			return nil, err
		}
		// An empty fixture is a blank document, not an empty update.
		var columns []string
		var args []any
		if len(fieldValues) > 0 {
			if columns, args, err = bindValues(*rt, fieldValues, false); err != nil {
				return nil, invalid(fmt.Sprintf("fixture %q: %s", fixture.Name, connectMessage(err)))
			}
		}
		doc, err := pendingDocument(ctx, *rt, docID, nil, columns, args)
		if err != nil {
			return nil, err
		}
		if len(fixture.Children) > 0 {
			// Child rows reference their parent, so the fixture's header is
			// staged with its values beside them; post() reads the header
			// from memory and the lines through find().
			system := map[string]any{"created_by": actorName(ctx), "modified_by": actorName(ctx), "def_version": rt.Revision, "request_id": requestID, "owner": actorName(ctx), "docstate": int32(lifecycle.Draft), "name": docID}
			if err := insertRow(ctx, tx, *rt, docID, system, columns, args); err != nil {
				return nil, err
			}
		}
		staged, err := stageFixtureChildren(ctx, tx, requestID, byChild, docID, fixture)
		if err != nil {
			return nil, err
		}
		run := ruleRun{
			engine: s.engine, rule: storedRule{Key: def.GetKey(), Version: version, Type: rules.Posting, Event: def.GetEvent()},
			def: rt, tx: tx, doc: doc, recordID: docID, event: def.GetEvent(), dryRun: true, source: source,
		}
		out, err := run.execute(ctx)
		if err != nil {
			return nil, err
		}
		if outcome := outcomeError(def.GetKey(), out); outcome != nil {
			return nil, invalid(fmt.Sprintf("fixture %q: %s", fixture.Name, connectMessage(outcome)))
		}
		if out.Plan == nil {
			return nil, invalid(fmt.Sprintf("fixture %q: the Rule returned no plan", fixture.Name))
		}
		plan := voucherPlan(rt.Name, docID, 1, out.Plan)
		if err := posting.ValidatePlan(ctx, tx, plan); err != nil {
			return nil, fixturePlanError(fixture.Name, err)
		}
		for _, child := range staged {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+dialect.QuoteIdent(recordTypeTableName(child.Name))+" WHERE `parent_id` = ?", docID); err != nil {
				return nil, internalError("clear staged fixture children", err)
			}
		}
		if len(fixture.Children) > 0 {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+dialect.QuoteIdent(recordTypeTableName(rt.Name))+" WHERE `id` = ?", docID); err != nil {
				return nil, internalError("clear staged fixture header", err)
			}
		}
		plans = append(plans, fixturePlan{name: fixture.Name, plan: plan})
	}
	return plans, nil
}

// fixtureFieldValues converts one fixture's Go values to the FieldValues its
// Record Type binds, naming the fixture and the field on any failure.
func fixtureFieldValues(d recordTypeDefinition, fixture string, values map[string]any) (map[string]*athenav1.FieldValue, error) {
	byKey := fieldsByKey(d)
	out := make(map[string]*athenav1.FieldValue, len(values))
	for key, value := range values {
		field, ok := byKey[key]
		if !ok {
			return nil, invalid(fmt.Sprintf("fixture %q names no field %q of %s", fixture, key, d.Label))
		}
		fv, err := fieldFromRule(field, value)
		if err != nil {
			return nil, invalid(fmt.Sprintf("fixture %q field %q: %s", fixture, key, err.Error()))
		}
		out[key] = fv
	}
	return out, nil
}

// stageFixtureChildren inserts one fixture's child rows under the fixture's
// document id so post() reads them through find(). It returns the child
// definitions it staged into for the caller to clear.
func stageFixtureChildren(ctx context.Context, tx *sql.Tx, requestID string, byChild map[string]recordTypeDefinition, docID string, fixture rules.Fixture) ([]recordTypeDefinition, error) {
	keys := make([]string, 0, len(fixture.Children))
	for key := range fixture.Children {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var staged []recordTypeDefinition
	for _, key := range keys {
		child, ok := byChild[key]
		if !ok {
			return nil, invalid(fmt.Sprintf("fixture %q names no child Record Type %q", fixture.Name, key))
		}
		for j, row := range fixture.Children[key] {
			where := fmt.Sprintf("%s children %s row %d", fixture.Name, key, j+1)
			fieldValues, err := fixtureFieldValues(child, where, row)
			if err != nil {
				return nil, err
			}
			var columns []string
			var args []any
			if len(fieldValues) > 0 {
				if columns, args, err = bindValues(child, fieldValues, false); err != nil {
					return nil, invalid(fmt.Sprintf("fixture %q: %s", where, connectMessage(err)))
				}
			}
			id, err := uuid.NewV7()
			if err != nil {
				return nil, internalError("stage fixture children", err)
			}
			system := map[string]any{"created_by": actorName(ctx), "modified_by": actorName(ctx), "def_version": child.Revision, "request_id": requestID, "parent_id": docID, "idx": j}
			if err := insertRow(ctx, tx, child, id.String(), system, columns, args); err != nil {
				return nil, err
			}
		}
		staged = append(staged, child)
	}
	return staged, nil
}

// fixturePlanError maps a fixture plan's validation failure to the save
// error: an undecided policy (Z2) keeps its sentinel, anything else is an
// authoring mistake to fix.
func fixturePlanError(fixture string, err error) error {
	if errors.Is(err, posting.ErrBookPolicyOpen) || errors.Is(err, posting.ErrCorrectionWindowOpen) {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("fixture %q: %s", fixture, err.Error()))
	}
	return invalid(fmt.Sprintf("fixture %q: %s", fixture, err.Error()))
}

// diffFixturePlans compares the previous and replayed plans fixture by
// fixture and describes every change in the author's own terms. Empty means
// the change plans identically and needs no approval. Fixtures are synthetic
// author data, so the diff may quote them.
func diffFixturePlans(previous, replayed []fixturePlan) string {
	byName := make(map[string]posting.Plan, len(previous))
	for _, p := range previous {
		byName[p.name] = p.plan
	}
	seen := map[string]bool{}
	var lines []string
	for _, p := range replayed {
		seen[p.name] = true
		before, ok := byName[p.name]
		if !ok {
			lines = append(lines, fmt.Sprintf("fixture %q is new", p.name))
			continue
		}
		lines = append(lines, diffPlans(fmt.Sprintf("fixture %q", p.name), before, p.plan)...)
	}
	for _, p := range previous {
		if !seen[p.name] {
			lines = append(lines, fmt.Sprintf("fixture %q was removed", p.name))
		}
	}
	const maxLines = 12
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("and %d more changes", len(lines)-maxLines))
	}
	return strings.Join(lines, "\n")
}

func diffPlans(where string, before, after posting.Plan) []string {
	var lines []string
	if before.Book != after.Book {
		lines = append(lines, fmt.Sprintf("%s: book %q became %q", where, before.Book, after.Book))
	}
	if before.PostingDate != after.PostingDate {
		lines = append(lines, fmt.Sprintf("%s: posting date %s became %s", where, before.PostingDate, after.PostingDate))
	}
	for i := 0; i < len(before.Lines) || i < len(after.Lines); i++ {
		leg := fmt.Sprintf("%s line %d", where, i+1)
		switch {
		case i >= len(before.Lines):
			lines = append(lines, leg+" was added")
		case i >= len(after.Lines):
			lines = append(lines, leg+" was removed")
		default:
			lines = append(lines, diffPlanLines(leg, before.Lines[i], after.Lines[i])...)
		}
	}
	return lines
}

func diffPlanLines(where string, before, after posting.Line) []string {
	var lines []string
	text := func(name, o, n string) {
		if o != n {
			lines = append(lines, fmt.Sprintf("%s: %s %q became %q", where, name, o, n))
		}
	}
	text("company", before.Company, after.Company)
	text("account", before.Account, after.Account)
	decimal := func(name string, o, n types.Decimal) {
		if o.Sub(n).Sign() != 0 {
			lines = append(lines, fmt.Sprintf("%s: %s %s became %s", where, name, o.String(), n.String()))
		}
	}
	decimal("debit", before.DebitBase, after.DebitBase)
	decimal("credit", before.CreditBase, after.CreditBase)
	text("transaction currency", before.TxnCurrency, after.TxnCurrency)
	decimal("transaction debit", before.TxnDebit, after.TxnDebit)
	decimal("transaction credit", before.TxnCredit, after.TxnCredit)
	var from, to types.Decimal
	if before.Quantity != nil {
		from = *before.Quantity
	}
	if after.Quantity != nil {
		to = *after.Quantity
	}
	decimal("quantity", from, to)
	text("unit", before.QuantityUOM, after.QuantityUOM)
	dimensions := map[string]bool{}
	for name, o := range before.Dimensions {
		dimensions[name] = true
		if n, ok := after.Dimensions[name]; !ok {
			lines = append(lines, fmt.Sprintf("%s: dimension %q %q was removed", where, name, o))
		} else if o != n {
			lines = append(lines, fmt.Sprintf("%s: dimension %q %q became %q", where, name, o, n))
		}
	}
	for name, n := range after.Dimensions {
		if !dimensions[name] {
			lines = append(lines, fmt.Sprintf("%s: dimension %q %q was added", where, name, n))
		}
	}
	text("source type", before.SourceType, after.SourceType)
	text("source id", before.SourceID, after.SourceID)
	return lines
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
	rows, err := s.db.QueryContext(ctx, "SELECT rule_name, label, rule_type, COALESCE(record_type, ''), COALESCE(event, ''), priority, enabled, version, source, source_hash, actor, created_at, COALESCE(condition_tree, ''), COALESCE(condition_expr, ''), COALESCE(message, ''), COALESCE(sets, '') FROM athena_rule_versions WHERE rule_name = ? ORDER BY version DESC", req.GetKey())
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
	setsJSON := ""
	if len(def.GetSets()) > 0 {
		sets := make([]builderSet, 0, len(def.GetSets()))
		for _, s := range def.GetSets() {
			sets = append(sets, builderSet{Field: s.GetField(), ValueExpr: s.GetValueExpr()})
		}
		if raw, err := json.Marshal(sets); err == nil {
			setsJSON = string(raw)
		}
	}
	run := ruleRun{
		engine: s.engine, rule: storedRule{Key: def.GetKey(), Version: version, Type: rules.Type(def.GetRuleType()), Event: def.GetEvent(), Priority: int(def.GetPriority()), ConditionExpr: checked.compiled, Message: def.GetMessage(), SetsJSON: setsJSON},
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
	if rules.Type(def.GetRuleType()) == rules.Posting && out.Status == rules.OK && out.Plan != nil {
		// A simulation names the plan it would post; the full preview with
		// resolved accounts and policy versions arrives with PreviewPosting.
		res.Message = fmt.Sprintf("the plan posts %d lines to book %q for %s", len(out.Plan.Lines), out.Plan.Book, out.Plan.PostingDate)
	}
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
