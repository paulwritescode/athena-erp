package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"athena-erp/internal/ledger"
	"athena-erp/internal/types"
)

func mustFamily(t *testing.T, s *ledger.Service, name, base string) ledger.UoMFamily {
	t.Helper()
	family, err := s.SaveUoMFamily(context.Background(), ledger.UoMFamily{Name: name, Label: name, BaseUnit: base}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return family
}

func mustConversion(t *testing.T, s *ledger.Service, family, unit, item, factor string, scale int) ledger.UoMConversion {
	t.Helper()
	row, err := s.SaveUoMConversion(context.Background(), ledger.UoMConversion{
		Family: family, Unit: unit, Item: item, Factor: mustDecimal(t, factor), Scale: scale,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func mustQuantity(t *testing.T, value, uom string) types.Quantity {
	t.Helper()
	qty, err := types.NewQuantity(mustDecimal(t, value), uom)
	if err != nil {
		t.Fatal(err)
	}
	return qty
}

func TestUoMFamilyLifecycle(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	family, err := s.SaveUoMFamily(ctx, ledger.UoMFamily{Name: "mass", Label: "Mass", BaseUnit: "kg"}, 0)
	if err != nil || family.Revision != 1 {
		t.Fatalf("create: %+v %v", family, err)
	}
	if _, err := s.SaveUoMFamily(ctx, ledger.UoMFamily{Name: "mass", Label: "Again", BaseUnit: "kg"}, 0); !errors.Is(err, ledger.ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := s.SaveUoMFamily(ctx, ledger.UoMFamily{Name: "Bad!", Label: "x", BaseUnit: "kg"}, 0); err == nil {
		t.Fatal("bad name accepted")
	}
	if _, err := s.SaveUoMFamily(ctx, ledger.UoMFamily{Name: "mass2", Label: "x", BaseUnit: "KG"}, 0); err == nil {
		t.Fatal("bad base unit accepted")
	}
	family.Label = "Mass units"
	family, err = s.SaveUoMFamily(ctx, family, 1)
	if err != nil || family.Revision != 2 {
		t.Fatalf("update: %+v %v", family, err)
	}
	if _, err := s.SaveUoMFamily(ctx, family, 1); !errors.Is(err, ledger.ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	got, err := s.GetUoMFamily(ctx, "mass")
	if err != nil || got.Label != "Mass units" || got.BaseUnit != "kg" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.GetUoMFamily(ctx, "missing"); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	families, err := s.ListUoMFamilies(ctx)
	if err != nil || len(families) != 1 {
		t.Fatalf("list: %v %v", families, err)
	}
}

func TestUoMConversionRules(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustFamily(t, s, "mass", "kg")
	if _, err := s.SaveUoMConversion(ctx, ledger.UoMConversion{Family: "missing", Unit: "g", Factor: mustDecimal(t, "0.001")}, 0); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("unknown family: %v", err)
	}
	bad := []ledger.UoMConversion{
		{Family: "mass", Unit: "G!", Factor: mustDecimal(t, "0.001")},
		{Family: "mass", Unit: "kg", Factor: mustDecimal(t, "1")},
		{Family: "mass", Unit: "g", Factor: mustDecimal(t, "0")},
		{Family: "mass", Unit: "g", Factor: mustDecimal(t, "-2")},
		{Family: "mass", Unit: "g", Factor: mustDecimal(t, "0.001"), Scale: -1},
		{Family: "mass", Unit: "g", Factor: mustDecimal(t, "0.001"), Scale: 13},
		{Family: "mass", Unit: "g", Item: "Bad Item!", Factor: mustDecimal(t, "0.001")},
	}
	for i, row := range bad {
		if _, err := s.SaveUoMConversion(ctx, row, 0); err == nil {
			t.Fatalf("bad row %d accepted", i)
		}
	}
	row := mustConversion(t, s, "mass", "g", "", "0.001", 6)
	if _, err := s.SaveUoMConversion(ctx, ledger.UoMConversion{Family: "mass", Unit: "g", Factor: mustDecimal(t, "0.002")}, 0); !errors.Is(err, ledger.ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	row.Factor = mustDecimal(t, "0.002")
	row, err := s.SaveUoMConversion(ctx, row, 1)
	if err != nil || row.Revision != 2 {
		t.Fatalf("update: %+v %v", row, err)
	}
	if _, err := s.SaveUoMConversion(ctx, row, 1); !errors.Is(err, ledger.ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	ghost := row
	ghost.Unit = "mg"
	if _, err := s.SaveUoMConversion(ctx, ghost, 1); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing row update: %v", err)
	}
	got, err := s.GetUoMConversion(ctx, "mass", "g", "")
	if err != nil || !got.Factor.Equal(mustDecimal(t, "0.002")) || got.Scale != 6 || got.Revision != 2 {
		t.Fatalf("get: %+v %v", got, err)
	}
	rows, err := s.ListUoMConversions(ctx, "mass")
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v %v", rows, err)
	}
}

func TestUoMToBaseAndBack(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustFamily(t, s, "mass", "kg")
	mustFamily(t, s, "count", "each")
	mustConversion(t, s, "mass", "g", "", "0.001", 6)
	mustConversion(t, s, "count", "box", "", "12", 2)
	mustConversion(t, s, "count", "box", "eggs", "6", 2)

	base, factor, err := s.ToBaseQuantity(ctx, mustQuantity(t, "1500", "g"), "", types.HalfUp)
	if err != nil || base.UoM != "kg" || !base.Value.Equal(mustDecimal(t, "1.5")) || !factor.Equal(mustDecimal(t, "0.001")) {
		t.Fatalf("to base: %v %v %v", base, factor, err)
	}
	// Base units come back unchanged with factor 1.
	base, factor, err = s.ToBaseQuantity(ctx, mustQuantity(t, "2.5", "kg"), "", types.HalfUp)
	if err != nil || base.String() != "2.5 kg" || factor.String() != "1" {
		t.Fatalf("base identity: %v %v %v", base, factor, err)
	}
	// The item override wins over the default; other items use the default.
	base, _, err = s.ToBaseQuantity(ctx, mustQuantity(t, "2", "box"), "eggs", types.HalfUp)
	if err != nil || base.UoM != "each" || !base.Value.Equal(mustDecimal(t, "12")) {
		t.Fatalf("override: %v %v", base, err)
	}
	base, _, err = s.ToBaseQuantity(ctx, mustQuantity(t, "2", "box"), "nails", types.HalfUp)
	if err != nil || !base.Value.Equal(mustDecimal(t, "24")) {
		t.Fatalf("default: %v %v", base, err)
	}
	// Disabling the override falls back to the default.
	override, err := s.GetUoMConversion(ctx, "count", "box", "eggs")
	if err != nil {
		t.Fatal(err)
	}
	override.Disabled = true
	if _, err := s.SaveUoMConversion(ctx, override, override.Revision); err != nil {
		t.Fatal(err)
	}
	base, _, err = s.ToBaseQuantity(ctx, mustQuantity(t, "2", "box"), "eggs", types.HalfUp)
	if err != nil || !base.Value.Equal(mustDecimal(t, "24")) {
		t.Fatalf("disabled override: %v %v", base, err)
	}
	// Base units convert back into the entered unit.
	back, err := s.FromBaseQuantity(ctx, mustQuantity(t, "1.5", "kg"), "g", "", types.HalfUp)
	if err != nil || back.UoM != "g" || !back.Value.Equal(mustDecimal(t, "1500")) {
		t.Fatalf("from base: %v %v", back, err)
	}
	if _, err := s.FromBaseQuantity(ctx, mustQuantity(t, "1.5", "kg"), "box", "", types.HalfUp); !errors.Is(err, ledger.ErrCrossFamily) {
		t.Fatalf("cross family from base: %v", err)
	}
	if _, err := s.FromBaseQuantity(ctx, mustQuantity(t, "500", "g"), "kg", "", types.HalfUp); err == nil || !strings.Contains(err.Error(), "base units") {
		t.Fatalf("non-base input: %v", err)
	}
	if _, _, err := s.ToBaseQuantity(ctx, mustQuantity(t, "1", "blargh"), "", types.HalfUp); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("unknown unit: %v", err)
	}
	if _, _, err := s.ToBaseQuantity(ctx, mustQuantity(t, "1", "g"), "", "sideways"); !errors.Is(err, types.ErrRoundingMode) {
		t.Fatalf("bad mode: %v", err)
	}
}

func TestUoMConvertRejectsCrossFamily(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	mustFamily(t, s, "mass", "kg")
	mustFamily(t, s, "count", "each")
	mustConversion(t, s, "mass", "g", "", "0.001", 6)
	mustConversion(t, s, "count", "box", "", "12", 2)

	got, err := s.ConvertQuantity(ctx, mustQuantity(t, "1500", "g"), "kg", "", types.HalfUp)
	if err != nil || !got.Value.Equal(mustDecimal(t, "1.5")) {
		t.Fatalf("convert: %v %v", got, err)
	}
	// Same unit is identity, never rounded.
	got, err = s.ConvertQuantity(ctx, mustQuantity(t, "1.23456789", "g"), "g", "", types.HalfUp)
	if err != nil || got.String() != "1.23456789 g" {
		t.Fatalf("identity: %v %v", got, err)
	}
	if _, err := s.ConvertQuantity(ctx, mustQuantity(t, "1500", "g"), "box", "", types.HalfUp); !errors.Is(err, ledger.ErrCrossFamily) {
		t.Fatalf("cross family: %v", err)
	}
	// A unit registered in two families is ambiguous.
	mustConversion(t, s, "mass", "pack", "", "2", 2)
	mustConversion(t, s, "count", "pack", "", "4", 2)
	if _, _, err := s.ToBaseQuantity(ctx, mustQuantity(t, "1", "pack"), "", types.HalfUp); !errors.Is(err, ledger.ErrAmbiguous) {
		t.Fatalf("ambiguous unit: %v", err)
	}
	// A disabled family refuses conversions.
	family, err := s.GetUoMFamily(ctx, "mass")
	if err != nil {
		t.Fatal(err)
	}
	family.Disabled = true
	if _, err := s.SaveUoMFamily(ctx, family, family.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConvertQuantity(ctx, mustQuantity(t, "1500", "g"), "kg", "", types.HalfUp); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled family: %v", err)
	}
}
