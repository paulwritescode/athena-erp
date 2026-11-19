package expr

import (
	"fmt"
	"reflect"
	"time"

	"athena-erp/internal/types"

	"cel.dev/cel-go/common/types/ref"

	celtypes "cel.dev/cel-go/common/types"
)

// The two exact value types CEL lacks (spec 23.6). Both wrap the values of
// internal/types, so an expression never sees a binary fraction.
var (
	decimalType = celtypes.NewOpaqueType("decimal")
	moneyType   = celtypes.NewOpaqueType("money")
)

type decimalVal struct{ d types.Decimal }

func (v decimalVal) ConvertToNative(typeDesc reflect.Type) (any, error) {
	if typeDesc == reflect.TypeOf(types.Decimal{}) {
		return v.d, nil
	}
	return nil, fmt.Errorf("a decimal does not convert to %v", typeDesc)
}

func (v decimalVal) ConvertToType(t ref.Type) ref.Val {
	switch t {
	case decimalType:
		return v
	case celtypes.TypeType:
		return decimalType
	case celtypes.StringType:
		return celtypes.String(v.d.String())
	}
	return celtypes.NewErr("a decimal does not convert to %s", t.TypeName())
}

// Equal compares exactly and ignores scale: 1.50 equals 1.5. Null and other
// types are never equal.
func (v decimalVal) Equal(other ref.Val) ref.Val {
	switch o := other.(type) {
	case decimalVal:
		return celtypes.Bool(v.d.Cmp(o.d) == 0)
	case celtypes.Int:
		return celtypes.Bool(v.d.Cmp(intDecimal(int64(o))) == 0)
	}
	return celtypes.False
}
func (v decimalVal) Type() ref.Type { return decimalType }
func (v decimalVal) Value() any     { return v.d }

type moneyVal struct{ m types.Money }

func (v moneyVal) ConvertToNative(typeDesc reflect.Type) (any, error) {
	if typeDesc == reflect.TypeOf(types.Money{}) {
		return v.m, nil
	}
	return nil, fmt.Errorf("money does not convert to %v", typeDesc)
}

func (v moneyVal) ConvertToType(t ref.Type) ref.Val {
	switch t {
	case moneyType:
		return v
	case celtypes.TypeType:
		return moneyType
	case celtypes.StringType:
		return celtypes.String(v.m.String())
	}
	return celtypes.NewErr("money does not convert to %s", t.TypeName())
}

// Equal needs the same currency and the same amount.
func (v moneyVal) Equal(other ref.Val) ref.Val {
	if o, ok := other.(moneyVal); ok {
		return celtypes.Bool(v.m.Currency.Code == o.m.Currency.Code && v.m.Currency.Exponent == o.m.Currency.Exponent && v.m.Minor == o.m.Minor)
	}
	return celtypes.False
}
func (v moneyVal) Type() ref.Type { return moneyType }
func (v moneyVal) Value() any     { return v.m }

func intDecimal(n int64) types.Decimal {
	d, _ := types.NewDecimal(n, 0)
	return d
}

// toRef converts a Go value of the model (nil, bool, int64, string,
// types.Decimal, types.Money, time.Time, []any) to a CEL value.
func toRef(v any) ref.Val {
	switch x := v.(type) {
	case nil:
		return celtypes.NullValue
	case bool:
		return celtypes.Bool(x)
	case int64:
		return celtypes.Int(x)
	case string:
		return celtypes.String(x)
	case types.Decimal:
		return decimalVal{x}
	case types.Money:
		return moneyVal{x}
	case time.Time:
		return celtypes.Timestamp{Time: x.UTC()}
	case []any:
		items := make([]ref.Val, len(x))
		for i, item := range x {
			items[i] = toRef(item)
		}
		return celtypes.NewRefValList(celtypes.DefaultTypeAdapter, items)
	}
	return celtypes.NewErr("unsupported value %T", v)
}

// fromRef converts an evaluation result back to the Go model.
func fromRef(v ref.Val) (any, error) {
	switch x := v.(type) {
	case celtypes.Null:
		return nil, nil
	case celtypes.Bool:
		return bool(x), nil
	case celtypes.Int:
		return int64(x), nil
	case celtypes.String:
		return string(x), nil
	case decimalVal:
		return x.d, nil
	case moneyVal:
		return x.m, nil
	case celtypes.Timestamp:
		return x.Time.UTC(), nil
	case *celtypes.Err:
		return nil, x
	}
	return nil, fmt.Errorf("unsupported result type %s", v.Type().TypeName())
}
