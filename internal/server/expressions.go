package server

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/expr"
	"athena-erp/internal/types"
)

// Expressions on fields (spec 3.3, 23.6, 24.4; ADR-0013). A formula is stored
// by the kernel on every write; required_if, read_only_if and validate are
// checked by the kernel; depends_on is a hint for the desk. Every expression
// is compiled when the Record Type is saved, so a bad one never reaches a
// write.

const maxExpressionLength = 1000

// today is the date the language's today() returns. Tests replace it.
var today = func() time.Time { return time.Now().UTC().Truncate(24 * time.Hour) }

// exprKind is the language kind of a field's values.
func exprKind(field recordTypeField) expr.Kind {
	switch field.Type {
	case "Number", "Currency", "Quantity":
		return expr.Decimal
	case "Money":
		return expr.Money
	case "Date":
		return expr.Date
	case "Checkbox":
		return expr.Bool
	}
	return expr.String
}

func (f recordTypeField) hasRules() bool {
	return f.RequiredIf != "" || f.ReadOnlyIf != "" || f.Validate != "" || f.DependsOn != ""
}

// hasExpressions reports whether any field of d uses an expression.
func (d recordTypeDefinition) hasExpressions() bool {
	for _, f := range d.Fields {
		if f.Formula != "" || f.hasRules() {
			return true
		}
	}
	return false
}

// formulaResultFits says whether a formula of kind result can fill field.
func formulaResultFits(field recordTypeField, result expr.Kind) bool {
	switch field.Type {
	case "Number", "Currency", "Quantity":
		return result == expr.Decimal || result == expr.Int
	case "Link":
		return false
	}
	return exprKind(field) == result
}

// exprEnvironment is the typed scope of one Record Type: its variables, the
// columns they map to, and the child Record Types that feed rollups.
type exprEnvironment struct {
	env      *expr.Env
	vars     []expr.Variable
	columns  map[string]string
	children []recordTypeDefinition
	byField  map[string]recordTypeField
}

// newExprEnvironment builds the scope of d. override replaces (or adds) one
// child Record Type, so a save can be checked before it is stored.
func newExprEnvironment(ctx context.Context, q querier, d recordTypeDefinition, override *recordTypeDefinition) (*exprEnvironment, error) {
	children, err := childRecordTypes(ctx, q, d.Name)
	if err != nil {
		return nil, err
	}
	if override != nil && override.ParentRecordType == d.Name {
		replaced := false
		for i := range children {
			if children[i].Name == override.Name {
				children[i], replaced = *override, true
			}
		}
		if !replaced {
			children = append(children, *override)
		}
	}
	e := &exprEnvironment{columns: map[string]string{}, children: children, byField: fieldsByKey(d)}
	add := func(v expr.Variable, column string) {
		e.vars = append(e.vars, v)
		if column != "" {
			e.columns[v.Name] = dialect.QuoteIdent(column)
		}
	}
	for _, field := range d.Fields {
		v := expr.Variable{Name: "doc." + field.Name, Kind: exprKind(field)}
		if field.Type == "Money" {
			v.Currency = field.Currency
			if field.Scale != nil {
				v.Scale = *field.Scale
			}
		}
		add(v, field.Name)
	}
	system := map[string]expr.Kind{"revision": expr.Int}
	for _, column := range d.systemColumns() {
		switch column.Name {
		case "owner", "created_by", "modified_by", "name":
			system[column.Name] = expr.String
		case "docstate":
			system[column.Name] = expr.Int
		}
	}
	names := make([]string, 0, len(system))
	for name := range system {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, own := e.byField[name]; !own {
			add(expr.Variable{Name: "doc." + name, Kind: system[name]}, name)
		}
	}
	for _, child := range children {
		if _, clash := e.byField[child.Name]; clash {
			return nil, invalid(fmt.Sprintf("%s has a field with the key %q, the same as its child Record Type %s; rename one of them", d.Label, child.Name, child.Label))
		}
		for _, field := range child.Fields {
			kind := exprKind(field)
			if kind != expr.Decimal && kind != expr.Money {
				continue
			}
			v := expr.Variable{Name: "doc." + child.Name + "." + field.Name, Kind: kind, List: true}
			if field.Type == "Money" {
				v.Currency = field.Currency
				if field.Scale != nil {
					v.Scale = *field.Scale
				}
			}
			add(v, "")
		}
	}
	if e.env, err = expr.NewEnv(e.vars...); err != nil {
		return nil, internalError("build expression environment", err)
	}
	return e, nil
}

type formulaField struct {
	field   recordTypeField
	program *expr.Program
}

type fieldRules struct {
	requiredIf, readOnlyIf, validate, dependsOn *expr.Program
}

// exprPlan holds the compiled expressions of one Record Type.
type exprPlan struct {
	*exprEnvironment
	def      recordTypeDefinition
	formulas []formulaField // in dependency order
	rules    map[string]fieldRules
	// usesChildren is true when an expression reads a rollup list.
	usesChildren bool
	// rollupOf lists the child Record Types whose lists a formula reads.
	rollupOf map[string]bool
}

// compilePlan compiles every expression of d. Errors name the field and the
// problem and are safe to show.
func compilePlan(ctx context.Context, q querier, d recordTypeDefinition, override *recordTypeDefinition) (*exprPlan, error) {
	environment, err := newExprEnvironment(ctx, q, d, override)
	if err != nil {
		return nil, err
	}
	plan := &exprPlan{exprEnvironment: environment, def: d, rules: map[string]fieldRules{}, rollupOf: map[string]bool{}}
	compile := func(field recordTypeField, attribute, source string) (*expr.Program, error) {
		if source == "" {
			return nil, nil
		}
		if utf8.RuneCountInString(source) > maxExpressionLength {
			return nil, invalid(fmt.Sprintf("%s: %s is longer than %d characters", field.Label, attribute, maxExpressionLength))
		}
		program, err := environment.env.Compile(source)
		if err != nil {
			return nil, invalid(fmt.Sprintf("%s: %s: %v", field.Label, attribute, err))
		}
		for _, dep := range program.Deps {
			if parts := strings.Split(dep, "."); len(parts) == 3 {
				plan.usesChildren = true
				plan.rollupOf[parts[1]] = true
			}
		}
		return program, nil
	}
	condition := func(field recordTypeField, attribute, source string) (*expr.Program, error) {
		program, err := compile(field, attribute, source)
		if err == nil && program != nil && program.Result != expr.Bool {
			err = invalid(fmt.Sprintf("%s: %s must return true or false, not %s", field.Label, attribute, program.Result))
		}
		return program, err
	}
	var formulas []formulaField
	for _, field := range d.Fields {
		var rules fieldRules
		if rules.requiredIf, err = condition(field, "required_if", field.RequiredIf); err != nil {
			return nil, err
		}
		if rules.readOnlyIf, err = condition(field, "read_only_if", field.ReadOnlyIf); err != nil {
			return nil, err
		}
		if rules.validate, err = condition(field, "validate", field.Validate); err != nil {
			return nil, err
		}
		if rules.dependsOn, err = condition(field, "depends_on", field.DependsOn); err != nil {
			return nil, err
		}
		if field.hasRules() {
			plan.rules[field.Name] = rules
		}
		program, err := compile(field, "formula", field.Formula)
		if err != nil {
			return nil, err
		}
		if program == nil {
			continue
		}
		if !formulaResultFits(field, program.Result) {
			return nil, invalid(fmt.Sprintf("%s: the formula returns %s, which a %s field cannot hold", field.Label, program.Result, field.Type))
		}
		if program.UsesToday {
			return nil, invalid(fmt.Sprintf("%s: a formula cannot use today(), because its result is stored; use it in validate or required_if instead", field.Label))
		}
		formulas = append(formulas, formulaField{field: field, program: program})
	}
	if plan.formulas, err = orderFormulas(formulas); err != nil {
		return nil, err
	}
	return plan, nil
}

// orderFormulas sorts formulas so each one comes after the formulas it reads,
// keeping the declared order otherwise. A cycle is an error.
func orderFormulas(formulas []formulaField) ([]formulaField, error) {
	byKey := map[string]int{}
	for i, f := range formulas {
		byKey[f.field.Name] = i
	}
	waiting := make([]map[int]bool, len(formulas))
	for i, f := range formulas {
		waiting[i] = map[int]bool{}
		for _, dep := range f.program.Deps {
			if j, ok := byKey[strings.TrimPrefix(dep, "doc.")]; ok && strings.HasPrefix(dep, "doc.") && !strings.Contains(dep[4:], ".") {
				waiting[i][j] = true
			}
		}
	}
	done := make([]bool, len(formulas))
	var ordered []formulaField
	for len(ordered) < len(formulas) {
		progress := false
		for i := range formulas {
			if done[i] {
				continue
			}
			ready := true
			for j := range waiting[i] {
				if !done[j] {
					ready = false
					break
				}
			}
			if ready {
				done[i], progress = true, true
				ordered = append(ordered, formulas[i])
			}
		}
		if !progress {
			var stuck []string
			for i := range formulas {
				if !done[i] {
					stuck = append(stuck, formulas[i].field.Label)
				}
			}
			return nil, invalid(fmt.Sprintf("these fields are calculated from each other, so no order works: %s", strings.Join(stuck, ", ")))
		}
	}
	return ordered, nil
}

// validateExpressions checks the expression attributes of a definition at
// save time, and that the saved parent of a child Record Type still compiles
// with the child as defined.
func validateExpressions(ctx context.Context, conn sqlConn, d recordTypeDefinition) error {
	for _, field := range d.Fields {
		if field.Formula != "" {
			switch {
			case field.Default != "":
				return invalid(fmt.Sprintf("%s: a calculated field cannot have a default", field.Label))
			case field.SetOnce, field.NoCopy:
				return invalid(fmt.Sprintf("%s: a calculated field is always recalculated, so it cannot be set once or skip copying", field.Label))
			case field.Type == "Link":
				return invalid(fmt.Sprintf("%s: a Link field cannot be calculated", field.Label))
			}
		}
	}
	if _, err := compilePlan(ctx, conn, d, nil); err != nil {
		return err
	}
	if d.kind() != kindChild {
		return nil
	}
	parent, _, err := loadRecordType(ctx, conn, d.ParentRecordType)
	if err != nil || parent == nil || !parent.hasExpressions() {
		return nil
	}
	if _, err := compilePlan(ctx, conn, *parent, &d); err != nil {
		return invalid(fmt.Sprintf("this change breaks %s, which reads %s in an expression: %s", parent.Label, d.Label, connectMessage(err)))
	}
	return nil
}

// ---- values ---------------------------------------------------------------

// textValue reads a stored text value of a field as a language value.
func textValue(field recordTypeField, text string) (any, error) {
	switch exprKind(field) {
	case expr.Decimal:
		return types.ParseDecimal(text)
	case expr.Money:
		minor, err := moneyMinor(field, text)
		if err != nil {
			return nil, err
		}
		cur, err := moneyCurrency(field)
		if err != nil {
			return nil, err
		}
		return types.NewMoney(minor, cur), nil
	case expr.Date:
		return types.ParseDate(text)
	}
	return text, nil
}

// argValue reads a database argument (nil, bool, int64 minor units, string).
func argValue(field recordTypeField, arg any) (any, error) {
	switch v := arg.(type) {
	case nil:
		if field.Type == "Checkbox" {
			return false, nil
		}
		return nil, nil
	case bool:
		return v, nil
	case int64:
		cur, err := moneyCurrency(field)
		if err != nil {
			return nil, err
		}
		return types.NewMoney(v, cur), nil
	case string:
		return textValue(field, v)
	}
	return nil, fmt.Errorf("unexpected stored value %T", arg)
}

// storedArg converts a stored FieldValue to the database argument form.
func storedArg(field recordTypeField, value *athenav1.FieldValue) (any, error) {
	switch v := value.GetKind().(type) {
	case *athenav1.FieldValue_BoolValue:
		return v.BoolValue, nil
	case *athenav1.FieldValue_StringValue:
		if field.Type == "Money" {
			return moneyMinor(field, v.StringValue)
		}
		return v.StringValue, nil
	}
	return nil, nil
}

// evaluation is one run over a Record's values.
type evaluation struct {
	plan     *exprPlan
	values   map[string]any // field key -> database argument
	system   map[string]any // revision, owner, ... -> language value
	children map[string][]*athenav1.Record
	today    time.Time
	scope    expr.Scope
}

// newEvaluation starts from the arguments of a Record. system holds the
// kernel columns by key.
func newEvaluation(plan *exprPlan, values map[string]any, system map[string]any, children map[string][]*athenav1.Record) (*evaluation, error) {
	e := &evaluation{plan: plan, values: values, system: system, children: children, today: today()}
	e.scope = expr.Scope{Today: e.today, Values: map[string]any{}}
	for _, field := range plan.def.Fields {
		if err := e.set(field); err != nil {
			return nil, err
		}
	}
	for key, value := range system {
		e.scope.Values["doc."+key] = value
	}
	for _, child := range plan.children {
		if !plan.rollupOf[child.Name] {
			continue
		}
		records := children[child.Name]
		for _, field := range child.Fields {
			kind := exprKind(field)
			if kind != expr.Decimal && kind != expr.Money {
				continue
			}
			list := make([]any, 0, len(records))
			for _, record := range records {
				arg, err := storedArg(field, record.GetValues()[field.Name])
				if err != nil {
					return nil, internalError("read child Records", err)
				}
				value, err := argValue(field, arg)
				if err != nil {
					return nil, internalError("read child Records", err)
				}
				list = append(list, value)
			}
			e.scope.Values["doc."+child.Name+"."+field.Name] = list
		}
	}
	return e, nil
}

func (e *evaluation) set(field recordTypeField) error {
	value, err := argValue(field, e.values[field.Name])
	if err != nil {
		return internalError("read "+field.Label, err)
	}
	e.scope.Values["doc."+field.Name] = value
	return nil
}

// runBool evaluates a condition; an unset program is "not true".
func (e *evaluation) runBool(p *expr.Program) (bool, error) {
	if p == nil {
		return false, nil
	}
	v, err := p.Eval(e.scope)
	if err != nil {
		return false, err
	}
	b, _ := v.(bool)
	return b, nil
}

// calculate evaluates the formulas in order and stores each result in e.values.
// With a non-nil onError, a formula that fails leaves its field empty and is
// reported there instead of stopping.
func (e *evaluation) calculate(onError func(field recordTypeField, err error)) (map[string]any, error) {
	computed := map[string]any{}
	for _, f := range e.plan.formulas {
		var arg any
		value, err := f.program.Eval(e.scope)
		if err == nil {
			arg, err = formulaArg(f.field, value)
		}
		if err != nil {
			if onError == nil {
				return nil, invalid(fmt.Sprintf("%s could not be calculated: %s", f.field.Label, connectMessage(err)))
			}
			onError(f.field, err)
			arg = nil
		}
		e.values[f.field.Name], computed[f.field.Name] = arg, arg
		if err := e.set(f.field); err != nil {
			return nil, err
		}
	}
	return computed, nil
}

// formulaArg turns a result into the field's database argument, applying the
// field's own rules (type, range, length, choices). A result that needs
// rounding is refused, like any other value.
func formulaArg(field recordTypeField, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	var text string
	switch v := value.(type) {
	case bool:
		return v, nil
	case int64:
		text = strconv.FormatInt(v, 10)
	case string:
		text = v
	case types.Decimal:
		text = trimDecimal(v.String())
	case types.Money:
		if field.Scale == nil || v.Currency.Code != field.Currency || v.Currency.Exponent != *field.Scale {
			return nil, fmt.Errorf("the result is in %s, not %s", v.Currency.Code, field.Currency)
		}
		text = v.Decimal().String()
	case time.Time:
		text = types.FormatDate(v)
	default:
		return nil, fmt.Errorf("unsupported result %T", value)
	}
	if text == "" {
		return nil, nil
	}
	arg, err := fieldArgument(field, &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: text}})
	if err != nil {
		return nil, fmt.Errorf("the result does not fit the field (%s)", connectMessage(err))
	}
	return arg, nil
}

// checkRules enforces required_if and validate on the final values.
func (e *evaluation) checkRules() error {
	for _, field := range e.plan.def.Fields {
		rules, ok := e.plan.rules[field.Name]
		if !ok {
			continue
		}
		empty := e.values[field.Name] == nil
		if rules.requiredIf != nil && empty {
			required, err := e.runBool(rules.requiredIf)
			if err != nil {
				return invalid(fmt.Sprintf("%s: required_if could not run: %s", field.Label, err))
			}
			if required {
				return invalid(fmt.Sprintf("%s is required", field.Label))
			}
		}
		if rules.validate != nil && !empty {
			valid, err := e.runBool(rules.validate)
			if err != nil {
				return invalid(fmt.Sprintf("%s: validate could not run: %s", field.Label, err))
			}
			if !valid {
				return invalid(fmt.Sprintf("%s is not valid: it must satisfy %s", field.Label, field.Validate))
			}
		}
	}
	return nil
}

// requireCalculated refuses an empty result for a required formula field.
func (e *evaluation) requireCalculated() error {
	for _, f := range e.plan.formulas {
		if f.field.Required && e.values[f.field.Name] == nil {
			return invalid(fmt.Sprintf("%s is required but its formula gave no value", f.field.Label))
		}
	}
	return nil
}

// systemValues returns the kernel columns readable as doc.<name>.
func systemValues(record *athenav1.Record, d recordTypeDefinition) map[string]any {
	values := map[string]any{"revision": record.GetRevision()}
	optional := func(key, text string) {
		if text != "" {
			values[key] = text
		}
	}
	optional("owner", record.GetOwner())
	optional("created_by", record.GetCreatedBy())
	optional("modified_by", record.GetModifiedBy())
	if d.Submittable {
		values["docstate"] = int64(record.GetDocstate())
		optional("name", record.GetName())
	}
	return values
}

// newRecordSystemValues are the kernel values a Record about to be created has.
func newRecordSystemValues(ctx context.Context, d recordTypeDefinition) map[string]any {
	values := map[string]any{"revision": int64(0), "owner": actorName(ctx), "created_by": actorName(ctx), "modified_by": actorName(ctx)}
	if d.Submittable {
		values["docstate"] = int64(0)
	}
	return values
}

// recordArgs reads a stored Record's values as database arguments.
func recordArgs(d recordTypeDefinition, record *athenav1.Record) (map[string]any, error) {
	values := make(map[string]any, len(d.Fields))
	for _, field := range d.Fields {
		arg, err := storedArg(field, record.GetValues()[field.Name])
		if err != nil {
			return nil, internalError("read "+field.Label, err)
		}
		values[field.Name] = arg
	}
	return values, nil
}

// loadRollupChildren reads the child Records an evaluation needs.
func loadRollupChildren(ctx context.Context, q querier, plan *exprPlan, parentID string) (map[string][]*athenav1.Record, error) {
	if !plan.usesChildren || parentID == "" {
		return nil, nil
	}
	out := map[string][]*athenav1.Record{}
	for _, child := range plan.children {
		if !plan.rollupOf[child.Name] {
			continue
		}
		records, err := listChildren(ctx, q, child, parentID)
		if err != nil {
			return nil, err
		}
		out[child.Name] = records
	}
	return out, nil
}

func columnKey(column string) string { return strings.Trim(column, "`\"") }

// applyExpressions runs the formulas and rules for a Record about to be
// written. columns and args are what bindValues returned (the full set on
// create, the changed fields on update); current is the stored Record on
// update. It returns the columns and arguments with the calculated fields
// that need writing added, and the keys of those fields.
func applyExpressions(ctx context.Context, q querier, d recordTypeDefinition, current *athenav1.Record, id string, columns []string, args []any) ([]string, []any, []string, error) {
	if !d.hasExpressions() {
		return columns, args, nil, nil
	}
	plan, err := compilePlan(ctx, q, d, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	values := map[string]any{}
	system := newRecordSystemValues(ctx, d)
	if current != nil {
		if values, err = recordArgs(d, current); err != nil {
			return nil, nil, nil, err
		}
		system = systemValues(current, d)
	}
	changed := map[string]bool{}
	for i, column := range columns {
		values[columnKey(column)], changed[columnKey(column)] = args[i], true
	}
	children, err := loadRollupChildren(ctx, q, plan, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if current != nil {
		before, err := newEvaluation(plan, storedOnly(current, d), system, children)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, field := range d.Fields {
			rules, ok := plan.rules[field.Name]
			if !ok || rules.readOnlyIf == nil || !changed[field.Name] {
				continue
			}
			locked, err := before.runBool(rules.readOnlyIf)
			if err != nil {
				return nil, nil, nil, invalid(fmt.Sprintf("%s: read_only_if could not run: %s", field.Label, err))
			}
			if locked {
				return nil, nil, nil, invalid(fmt.Sprintf("%s is read only right now", field.Label))
			}
		}
	}
	e, err := newEvaluation(plan, values, system, children)
	if err != nil {
		return nil, nil, nil, err
	}
	computed, err := e.calculate(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := e.requireCalculated(); err != nil {
		return nil, nil, nil, err
	}
	if err := e.checkRules(); err != nil {
		return nil, nil, nil, err
	}
	var keys []string
	for _, f := range plan.formulas {
		arg := computed[f.field.Name]
		if current != nil && sameValue(f.field, current.GetValues()[f.field.Name], arg) {
			continue
		}
		if current == nil && arg == nil {
			continue
		}
		columns = append(columns, dialect.QuoteIdent(f.field.Name))
		args = append(args, arg)
		keys = append(keys, f.field.Name)
	}
	return columns, args, keys, nil
}

// storedOnly returns the stored Record's arguments without the pending
// changes, for the read_only_if check of a write.
func storedOnly(current *athenav1.Record, d recordTypeDefinition) map[string]any {
	stored, err := recordArgs(d, current)
	if err != nil {
		return map[string]any{}
	}
	return stored
}

// refreshParent recalculates the formulas of a parent Record after one of its
// child Records changed, in the child's transaction. The parent's revision
// does not change, because the person editing it did not; the new values are
// recorded in the Audit Log as "recalculated".
func refreshParent(ctx context.Context, tx *sql.Tx, child recordTypeDefinition, parentID string) error {
	parentType, _, err := loadRecordType(ctx, tx, child.ParentRecordType)
	if err != nil || parentType == nil {
		return internalError("load parent Record Type", fmt.Errorf("parent %q: %v", child.ParentRecordType, err))
	}
	if !parentType.hasExpressions() {
		return nil
	}
	plan, err := compilePlan(ctx, tx, *parentType, nil)
	if err != nil {
		return err
	}
	if !plan.rollupOf[child.Name] {
		return nil
	}
	_, err = recalculate(ctx, tx, plan, parentID)
	return err
}

// recalculate recomputes every formula of one Record and stores the ones that
// changed. It reports whether anything changed.
func recalculate(ctx context.Context, tx *sql.Tx, plan *exprPlan, id string) (bool, error) {
	record, err := loadRecord(ctx, tx, plan.def, id, dialect.ForUpdate())
	if err != nil {
		return false, err
	}
	values, err := recordArgs(plan.def, record)
	if err != nil {
		return false, err
	}
	children, err := loadRollupChildren(ctx, tx, plan, id)
	if err != nil {
		return false, err
	}
	e, err := newEvaluation(plan, values, systemValues(record, plan.def), children)
	if err != nil {
		return false, err
	}
	computed, err := e.calculate(nil)
	if err != nil {
		return false, err
	}
	var assignments []string
	var params []any
	var keys []string
	for _, f := range plan.formulas {
		arg := computed[f.field.Name]
		if sameValue(f.field, record.GetValues()[f.field.Name], arg) {
			continue
		}
		assignments = append(assignments, dialect.QuoteIdent(f.field.Name)+" = ?")
		params = append(params, arg)
		keys = append(keys, f.field.Name)
	}
	if len(keys) == 0 {
		return false, nil
	}
	statement := fmt.Sprintf("UPDATE %s SET %s WHERE `id` = ?", dialect.QuoteIdent(recordTypeTableName(plan.def.Name)), strings.Join(assignments, ", "))
	if _, err := tx.ExecContext(ctx, statement, append(params, id)...); err != nil {
		return false, writeError("recalculate record", err)
	}
	updated, err := loadRecord(ctx, tx, plan.def, id, "")
	if err != nil {
		return false, err
	}
	if err := writeAudit(ctx, tx, plan.def.Name, id, record.GetRevision(), "recalculated", fieldChanges(keys, record, updated)); err != nil {
		return false, err
	}
	return true, nil
}

// recalculateAll recomputes the stored formulas of every Record of d after a
// save changed them. Each Record is its own transaction, so an interrupted
// save resumes safely: recalculating is idempotent.
func recalculateAll(ctx context.Context, conn sqlConn, d recordTypeDefinition, override *recordTypeDefinition) error {
	plan, err := compilePlan(ctx, conn, d, override)
	if err != nil {
		return err
	}
	if len(plan.formulas) == 0 {
		return nil
	}
	table := dialect.QuoteIdent(recordTypeTableName(d.Name))
	after := ""
	for {
		rows, err := conn.QueryContext(ctx, "SELECT `id` FROM "+table+" WHERE `id` > ? ORDER BY `id` LIMIT 200", after)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if _, err := recalculate(ctx, tx, plan, id); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("recalculate %s %s: %s", d.Name, id, connectMessage(err))
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		after = ids[len(ids)-1]
	}
}

// formulasChanged reports whether saving d changes what any stored formula
// should hold: a new or edited formula, or a changed field type.
func formulasChanged(d recordTypeDefinition, previous *recordTypeDefinition) bool {
	if previous == nil {
		return false
	}
	old := fieldsByKey(*previous)
	for _, field := range d.Fields {
		if field.Formula == "" {
			continue
		}
		before, existed := old[field.Name]
		if !existed || before.Formula != field.Formula || before.Type != field.Type || before.Currency != field.Currency {
			return true
		}
	}
	return false
}

// recalculateAfterSave recomputes the Record Type's stored formulas when the
// save changed them, then the parent's, whose rollups read this Record Type.
func recalculateAfterSave(ctx context.Context, conn sqlConn, d recordTypeDefinition, previous *recordTypeDefinition) error {
	if !formulasChanged(d, previous) {
		return nil
	}
	if err := recalculateAll(ctx, conn, d, nil); err != nil {
		return err
	}
	if d.kind() != kindChild {
		return nil
	}
	parent, _, err := loadRecordType(ctx, conn, d.ParentRecordType)
	if err != nil || parent == nil || !parent.hasExpressions() {
		return err
	}
	return recalculateAll(ctx, conn, *parent, &d)
}
