package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/expr"
	"athena-erp/internal/types"
)

// filterPredicate compiles a ListRecords filter and translates it to SQL.
func filterPredicate(ctx context.Context, q querier, d recordTypeDefinition, source string) (expr.SQL, error) {
	environment, err := newExprEnvironment(ctx, q, d, nil)
	if err != nil {
		return expr.SQL{}, err
	}
	program, err := environment.env.Compile(source)
	if err != nil {
		return expr.SQL{}, invalid("filter: " + err.Error())
	}
	if program.Result != expr.Bool {
		return expr.SQL{}, invalid("filter must return true or false")
	}
	predicate, err := program.Translate(dialect, environment.columns, today())
	if err != nil {
		return expr.SQL{}, invalid("filter: " + err.Error())
	}
	return predicate, nil
}

// fieldValueOf renders a database argument as a FieldValue.
func fieldValueOf(field recordTypeField, arg any) *athenav1.FieldValue {
	switch v := arg.(type) {
	case bool:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_BoolValue{BoolValue: v}}
	case int64:
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: moneyText(field, v)}}
	case string:
		if exprKind(field) == expr.Decimal {
			v = trimDecimal(v)
		}
		return &athenav1.FieldValue{Kind: &athenav1.FieldValue_StringValue{StringValue: v}}
	}
	return &athenav1.FieldValue{Kind: &athenav1.FieldValue_NullValue{NullValue: true}}
}

func (s recordService) PreviewRecord(ctx context.Context, req *athenav1.PreviewRecordRequest) (res *athenav1.PreviewRecordResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "preview_record", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
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
	if req.GetId() != "" {
		if err := authorizeRecord(ctx, s.db, p, recordType, req.GetId(), auth.ActionRead); err != nil {
			return nil, err
		}
	}
	res = &athenav1.PreviewRecordResponse{Computed: map[string]*athenav1.FieldValue{}}
	if !recordType.hasExpressions() {
		return res, nil
	}
	// Hidden fields still take part in the calculation, as they do on save,
	// but their own results and states are left out of the response.
	defer func() {
		if res == nil {
			return
		}
		for key := range hidden {
			delete(res.Computed, key)
		}
		states := res.States[:0]
		for _, state := range res.States {
			if !hidden[state.GetKey()] {
				states = append(states, state)
			}
		}
		res.States = states
	}()
	plan, err := compilePlan(ctx, s.db, recordType, nil)
	if err != nil {
		return nil, err
	}
	values, system := map[string]any{}, newRecordSystemValues(ctx, recordType)
	var current *athenav1.Record
	if req.GetId() != "" {
		if current, err = s.load(ctx, recordType, req.GetId()); err != nil {
			return nil, err
		}
		if values, err = recordArgs(recordType, current); err != nil {
			return nil, err
		}
		system = systemValues(current, recordType)
	}
	problems := map[string]string{}
	fields := fieldsByKey(recordType)
	for key, entered := range req.GetValues() {
		field, ok := fields[key]
		if !ok || field.Formula != "" {
			continue
		}
		arg, err := fieldArgument(field, entered)
		if err != nil {
			problems[key], arg = connectMessage(err), nil
		}
		values[key] = arg
	}
	if current == nil {
		for _, field := range recordType.Fields {
			if _, set := values[field.Name]; !set {
				if fallback, err := createDefault(field); err == nil {
					values[field.Name] = fallback
				}
			}
		}
	}
	children, err := loadRollupChildren(ctx, s.db, plan, req.GetId())
	if err != nil {
		return nil, err
	}
	e, err := newEvaluation(plan, values, system, children)
	if err != nil {
		return nil, err
	}
	computed, err := e.calculate(func(field recordTypeField, err error) { problems[field.Name] = err.Error() })
	if err != nil {
		return nil, err
	}
	for _, f := range plan.formulas {
		res.Computed[f.field.Name] = fieldValueOf(f.field, computed[f.field.Name])
	}
	for _, field := range recordType.Fields {
		rules := plan.rules[field.Name]
		if field.Formula == "" && !field.hasRules() {
			continue
		}
		state := &athenav1.FieldState{Key: field.Name, Required: field.Required, ReadOnly: field.ReadOnly || field.Formula != "", Error: problems[field.Name]}
		check := func(program *expr.Program, apply func()) {
			if program == nil {
				return
			}
			ok, err := e.runBool(program)
			if err != nil {
				state.Error = err.Error()
				return
			}
			if ok {
				apply()
			}
		}
		check(rules.requiredIf, func() { state.Required = true })
		if current != nil {
			check(rules.readOnlyIf, func() { state.ReadOnly = true })
		}
		if rules.dependsOn != nil {
			state.Hidden = true
			check(rules.dependsOn, func() { state.Hidden = false })
		}
		res.States = append(res.States, state)
	}
	return res, nil
}

func formatValue(v any) string {
	switch x := v.(type) {
	case types.Decimal:
		return x.String()
	case types.Money:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case string:
		return x
	case time.Time:
		return types.FormatDate(x)
	}
	return fmt.Sprint(v)
}

func (s recordService) EvaluateExpression(ctx context.Context, req *athenav1.EvaluateExpressionRequest) (res *athenav1.EvaluateExpressionResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "evaluate_expression", req.GetRecordTypeKey())
	defer func() { end(err) }()
	recordType, err := s.recordType(ctx, req.GetRecordTypeKey())
	if err != nil {
		return nil, err
	}
	p, err := accessFor(ctx)
	if err != nil {
		return nil, err
	}
	// Fields above the caller's level are not in scope, so an expression
	// cannot read them.
	recordType = visibleDefinition(recordType, hiddenFields(p, recordType))
	if req.GetId() != "" {
		if err := authorizeRecord(ctx, s.db, p, recordType, req.GetId(), auth.ActionRead); err != nil {
			return nil, err
		}
	}
	environment, err := newExprEnvironment(ctx, s.db, recordType, nil)
	if err != nil {
		return nil, err
	}
	program, err := environment.env.Compile(req.GetExpression())
	if err != nil {
		return nil, invalid(err.Error())
	}
	res = &athenav1.EvaluateExpressionResponse{ResultType: string(program.Result), Dependencies: program.Deps, UsesToday: program.UsesToday}
	if program.Result != expr.Bool {
		res.SqlReason = "only a true or false expression can filter"
	} else if _, err := program.Translate(dialect, environment.columns, today()); err != nil {
		res.SqlReason = err.Error()
	} else {
		res.SqlTranslatable = true
	}
	if req.GetId() == "" {
		return res, nil
	}
	record, err := s.load(ctx, recordType, req.GetId())
	if err != nil {
		return nil, err
	}
	plan := &exprPlan{exprEnvironment: environment, def: recordType, rollupOf: map[string]bool{}}
	for _, dep := range program.Deps {
		for _, child := range environment.children {
			if len(dep) > len(child.Name)+5 && dep[:len(child.Name)+5] == "doc."+child.Name+"." {
				plan.usesChildren, plan.rollupOf[child.Name] = true, true
			}
		}
	}
	values, err := recordArgs(recordType, record)
	if err != nil {
		return nil, err
	}
	children, err := loadRollupChildren(ctx, s.db, plan, req.GetId())
	if err != nil {
		return nil, err
	}
	e, err := newEvaluation(plan, values, systemValues(record, recordType), children)
	if err != nil {
		return nil, err
	}
	value, err := program.Eval(e.scope)
	if err != nil {
		return nil, invalid("the expression could not be evaluated: " + err.Error())
	}
	res.Evaluated, res.IsNull = true, value == nil
	if value != nil {
		res.Value = formatValue(value)
	}
	return res, nil
}
