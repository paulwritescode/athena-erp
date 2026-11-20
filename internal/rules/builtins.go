package rules

import (
	"errors"
	"fmt"

	"athena-erp/internal/types"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// predeclared builds the host functions of one run. Each closes over the run's
// state, so counters and limits are per run. Only the functions a type may
// use are present; calling another is a compile error, not a runtime check.
func (s *state) predeclared(t Type) starlark.StringDict {
	all := starlark.StringDict{
		"fail":     starlark.NewBuiltin("fail", s.fail),
		"warn":     starlark.NewBuiltin("warn", s.warn),
		"require":  starlark.NewBuiltin("require", s.require),
		"decimal":  starlark.NewBuiltin("decimal", s.decimal),
		"money":    starlark.NewBuiltin("money", s.money),
		"allocate": starlark.NewBuiltin("allocate", s.allocate),
		"round":    starlark.NewBuiltin("round", s.round),
		"div":      starlark.NewBuiltin("div", s.div),
		"log": starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"info":  starlark.NewBuiltin("log.info", s.logger("info")),
			"warn":  starlark.NewBuiltin("log.warn", s.logger("warn")),
			"trace": starlark.NewBuiltin("log.trace", s.logger("trace")),
		}),
	}
	if t.Reads() {
		all["get"] = starlark.NewBuiltin("get", s.get)
		all["find"] = starlark.NewBuiltin("find", s.find)
		all["exists"] = starlark.NewBuiltin("exists", s.exists)
	}
	if t.Writes() {
		all["set"] = starlark.NewBuiltin("set", s.set)
	}
	return all
}

func (s *state) fail(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var code, message string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "code", &code, "message", &message); err != nil {
		return nil, err
	}
	return nil, &Rejection{Code: clip(code, 40), Message: clip(message, maxMessageLen)}
}

func (s *state) warn(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var code, message string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "code", &code, "message", &message); err != nil {
		return nil, err
	}
	s.message(&s.outcome.Warnings, clip(code, 40), message)
	return starlark.None, nil
}

func (s *state) require(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var condition starlark.Value
	var message string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "condition", &condition, "message", &message); err != nil {
		return nil, err
	}
	ok, isBool := condition.(starlark.Bool)
	if !isBool {
		return nil, fmt.Errorf("require needs a True or False condition, not %s", condition.Type())
	}
	if !ok {
		return nil, &Rejection{Code: "required", Message: clip(message, maxMessageLen)}
	}
	return starlark.None, nil
}

func (s *state) logger(level string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(kwargs) > 0 {
			return nil, fmt.Errorf("%s takes no keyword arguments", b.Name())
		}
		text := ""
		for i, arg := range args {
			if i > 0 {
				text += " "
			}
			if str, ok := starlark.AsString(arg); ok {
				text += str
			} else {
				text += arg.String()
			}
		}
		s.message(&s.outcome.Logs, level, text)
		return starlark.None, nil
	}
}

func (s *state) decimal(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "value", &value); err != nil {
		return nil, err
	}
	switch v := value.(type) {
	case starlark.Float:
		return nil, ErrFloat
	case starlark.String:
		if err := s.checkLen("the decimal text", len(v)); err != nil {
			return nil, err
		}
		d, err := types.ParseDecimal(string(v))
		if err != nil {
			return nil, fmt.Errorf("%q is not a decimal number", string(v))
		}
		if err := s.countValues(1); err != nil {
			return nil, err
		}
		return decimalValue{d}, nil
	case starlark.Int:
		if intTooLarge(v) {
			return nil, &HostError{Code: CodeMemoryLimit, Err: errors.New("the integer is too large to use as a decimal")}
		}
	}
	if d, ok := asDecimal(value); ok {
		if err := s.countValues(1); err != nil {
			return nil, err
		}
		return decimalValue{d}, nil
	}
	return nil, fmt.Errorf("decimal needs a string or an integer, not %s", value.Type())
}

func (s *state) money(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var minor starlark.Int
	var code string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "minor", &minor, "currency", &code); err != nil {
		return nil, err
	}
	units, ok := minor.Int64()
	if !ok {
		return nil, errors.New("the amount does not fit 64 bits")
	}
	currency, known := s.host.Currency(code)
	if !known {
		return nil, fmt.Errorf("the currency %q is not used by this Record Type", code)
	}
	if err := s.countValues(1); err != nil {
		return nil, err
	}
	return moneyValue{types.NewMoney(units, currency)}, nil
}

func (s *state) allocate(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var total moneyValue
	var weights *starlark.List
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "total", &total, "weights", &weights); err != nil {
		return nil, err
	}
	if err := s.checkLen("the weights", weights.Len()); err != nil {
		return nil, err
	}
	parts := make([]int64, weights.Len())
	for i := range parts {
		n, ok := weights.Index(i).(starlark.Int)
		if !ok {
			return nil, errors.New("weights must be integers")
		}
		if parts[i], ok = n.Int64(); !ok {
			return nil, errors.New("a weight does not fit 64 bits")
		}
	}
	shares, err := total.m.Allocate(parts)
	if err != nil {
		return nil, err
	}
	if err := s.countValues(len(shares) + 1); err != nil {
		return nil, err
	}
	out := make([]starlark.Value, len(shares))
	for i, share := range shares {
		out[i] = moneyValue{share}
	}
	return starlark.NewList(out), nil
}

func roundingMode(name string) (types.RoundingMode, error) {
	mode := types.RoundingMode(name)
	if !mode.Valid() {
		return "", fmt.Errorf("the rounding mode must be %q or %q", types.HalfEven, types.HalfUp)
	}
	return mode, nil
}

func (s *state) round(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	var scale int
	mode := string(types.HalfEven)
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "value", &value, "scale", &scale, "mode?", &mode); err != nil {
		return nil, err
	}
	if i, ok := value.(starlark.Int); ok && intTooLarge(i) {
		return nil, &HostError{Code: CodeMemoryLimit, Err: errors.New("the integer is too large to use as a decimal")}
	}
	d, ok := asDecimal(value)
	if !ok {
		return nil, fmt.Errorf("round needs a decimal, not %s", value.Type())
	}
	m, err := roundingMode(mode)
	if err != nil {
		return nil, err
	}
	rounded, err := d.Round(scale, m)
	if err != nil {
		return nil, err
	}
	if err := s.countValues(1); err != nil {
		return nil, err
	}
	return decimalValue{rounded}, nil
}

func (s *state) div(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var x, y starlark.Value
	var scale int
	mode := string(types.HalfEven)
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "x", &x, "y", &y, "scale", &scale, "mode?", &mode); err != nil {
		return nil, err
	}
	for _, v := range []starlark.Value{x, y} {
		if i, ok := v.(starlark.Int); ok && intTooLarge(i) {
			return nil, &HostError{Code: CodeMemoryLimit, Err: errors.New("the integer is too large to use as a decimal")}
		}
	}
	a, ok1 := asDecimal(x)
	c, ok2 := asDecimal(y)
	if !ok1 || !ok2 {
		return nil, errors.New("div needs two decimals or integers")
	}
	m, err := roundingMode(mode)
	if err != nil {
		return nil, err
	}
	q, err := a.Div(c, scale, m)
	if err != nil {
		return nil, err
	}
	if err := s.countValues(1); err != nil {
		return nil, err
	}
	return decimalValue{q}, nil
}

// filtersFrom reads the equality filters of find and exists.
func (s *state) filtersFrom(value starlark.Value) (map[string]any, error) {
	filters := map[string]any{}
	if value == nil || value == starlark.None {
		return filters, nil
	}
	dict, ok := value.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("filters must be a dict, not %s", value.Type())
	}
	if err := s.checkLen("the filters", dict.Len()); err != nil {
		return nil, err
	}
	if err := s.countValues(dict.Len()); err != nil {
		return nil, err
	}
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, errors.New("filter keys must be field names")
		}
		v, err := s.toGo(item[1])
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", key, err)
		}
		filters[key] = v
	}
	return filters, nil
}

func (s *state) countReads(n int) error {
	s.outcome.Reads += n
	if s.outcome.Reads > s.limits.MaxReads {
		return &HostError{Code: CodeRowLimit, Err: fmt.Errorf("the Rule read more than %d rows", s.limits.MaxReads)}
	}
	return nil
}

func (s *state) get(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var recordType, id string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "record_type", &recordType, "id", &id); err != nil {
		return nil, err
	}
	if err := s.countReads(1); err != nil {
		return nil, err
	}
	row, found, err := s.host.Get(recordType, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return starlark.None, nil
	}
	if err := s.countValues(len(row.Values) + 1); err != nil {
		return nil, err
	}
	return newRecord(recordType, row)
}

func (s *state) find(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var recordType string
	var filter starlark.Value
	limit := 20
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "record_type", &recordType, "filters?", &filter, "limit?", &limit); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, errors.New("limit must be at least 1")
	}
	limit = min(limit, s.limits.MaxRows)
	filters, err := s.filtersFrom(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.host.Find(recordType, filters, limit)
	if err != nil {
		return nil, err
	}
	if len(rows) > s.limits.MaxRows {
		return nil, &HostError{Code: CodeRowLimit, Err: fmt.Errorf("find returned more than %d rows", s.limits.MaxRows)}
	}
	if err := s.countReads(max(len(rows), 1)); err != nil {
		return nil, err
	}
	values := 1
	for _, row := range rows {
		values += len(row.Values) + 1
	}
	if err := s.countValues(values); err != nil {
		return nil, err
	}
	out := make([]starlark.Value, len(rows))
	for i, row := range rows {
		if out[i], err = newRecord(recordType, row); err != nil {
			return nil, err
		}
	}
	return starlark.NewList(out), nil
}

func (s *state) exists(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var recordType string
	var filter starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "record_type", &recordType, "filters?", &filter); err != nil {
		return nil, err
	}
	filters, err := s.filtersFrom(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.host.Find(recordType, filters, 1)
	if err != nil {
		return nil, err
	}
	if err := s.countReads(1); err != nil {
		return nil, err
	}
	if err := s.countValues(1); err != nil {
		return nil, err
	}
	return starlark.Bool(len(rows) > 0), nil
}

func (s *state) set(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var field string
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "field", &field, "value", &value); err != nil {
		return nil, err
	}
	if len(s.outcome.Writes) >= s.limits.MaxWrites {
		return nil, &HostError{Code: CodeLimit, Err: fmt.Errorf("the Rule set more than %d fields", s.limits.MaxWrites)}
	}
	goValue, err := s.toGo(value)
	if err != nil {
		return nil, err
	}
	if err := s.countValues(1); err != nil {
		return nil, err
	}
	if err := s.host.Set(field, goValue); err != nil {
		return nil, err
	}
	s.outcome.Writes = append(s.outcome.Writes, Write{Field: field, Value: goValue})
	return starlark.None, nil
}
