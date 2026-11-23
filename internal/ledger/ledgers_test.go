// Package ledger_test holds the database-backed ledger master tests. This
// file pins Property 25: effective ranges reject overlaps, as-of lookups
// return one row, and UoM conversions round-trip within scale.
package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"athena-erp/internal/ledger"
	"athena-erp/internal/types"

	"pgregory.net/rapid"
)

// TestLedgerEffectiveRangesProperty checks the ranges half of Property 25
// over drawn period sets in fresh companies: a save overlapping an accepted
// range fails naming the overlap, disjoint saves succeed, and PeriodFor
// returns the unique covering period or nothing, matching the oracle.
// Periods are the ledger's effective-dated rows; the same reject-overlap
// plus single-as-of-row contract binds effective-dated Record Types (R13.2).
//
// Feature: athena-batch-2, Property 25: Effective ranges and UoM round trip.
func TestLedgerEffectiveRangesProperty(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day := func(offset int) string { return base.AddDate(0, 0, offset).Format("2006-01-02") }
	share := func(s1, e1, s2, e2 string) bool { return s1 <= e2 && s2 <= e1 }
	n := 0
	rapid.Check(t, func(t *rapid.T) {
		n++
		company := fmt.Sprintf("c%d", n)
		if _, err := s.SaveCompany(ctx, ledger.Company{Name: company, Label: company}, 0); err != nil {
			t.Fatalf("company: %v", err)
		}
		type keptRange struct{ name, start, end string }
		var kept []keptRange
		for j := 0; j < rapid.IntRange(1, 6).Draw(t, "periods"); j++ {
			start := rapid.IntRange(0, 60).Draw(t, "start")
			end := start + rapid.IntRange(0, 20).Draw(t, "length")
			name := fmt.Sprintf("p%d_%d", n, j)
			_, err := s.SavePeriod(ctx, ledger.Period{Company: company, Name: name, Label: name, StartDate: day(start), EndDate: day(end)}, 0)
			clash := ""
			for _, k := range kept {
				if share(day(start), day(end), k.start, k.end) {
					clash = k.name
					break
				}
			}
			if clash != "" {
				if err == nil || !strings.Contains(err.Error(), "overlaps") {
					t.Fatalf("overlap with %s accepted: %v", clash, err)
				}
			} else {
				if err != nil {
					t.Fatalf("disjoint save: %v", err)
				}
				kept = append(kept, keptRange{name, day(start), day(end)})
			}
		}
		for range rapid.IntRange(1, 3).Draw(t, "queries") {
			at := day(rapid.IntRange(-5, 85).Draw(t, "at"))
			want := ""
			for _, k := range kept {
				if k.start <= at && at <= k.end {
					if want != "" {
						t.Fatalf("oracle holds two covers for %s", at)
					}
					want = k.name
				}
			}
			got, err := s.PeriodFor(ctx, company, "main", at)
			if want == "" {
				if !errors.Is(err, ledger.ErrNotFound) {
					t.Fatalf("gap %s: %+v %v", at, got, err)
				}
			} else if err != nil || got.Name != want {
				t.Fatalf("as-of %s: %+v %v, want %s", at, got, err, want)
			}
		}
	})
}

// TestUoMRoundTripProperty checks the UoM half of Property 25 over drawn
// families, factors and quantities: converting to base units and back
// returns the entered quantity within the declared scale, and converting
// across families is rejected. The scale bound is exact integer math: each
// rounding errs at most half a last-place unit, so with factor f at scale
// s, f·|back − entered| ≤ 0.5·10^−s·(1+f).
//
// Feature: athena-batch-2, Property 25: Effective ranges and UoM round trip.
func TestUoMRoundTripProperty(t *testing.T) {
	ctx := context.Background()
	s := service(t)
	one, err := types.NewDecimal(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	rapid.Check(t, func(t *rapid.T) {
		n++
		family := fmt.Sprintf("f%d", n)
		baseUnit := fmt.Sprintf("base%d", n)
		unit := fmt.Sprintf("u%d", n)
		other := fmt.Sprintf("g%d", n)
		otherBase := fmt.Sprintf("c%d", n)
		factor, err := types.NewDecimal(int64(rapid.IntRange(1, 999999).Draw(t, "factor unscaled")), rapid.IntRange(0, 6).Draw(t, "factor scale"))
		if err != nil {
			t.Fatalf("factor: %v", err)
		}
		scale := rapid.IntRange(0, 6).Draw(t, "scale")
		mode := types.HalfUp
		if rapid.IntRange(0, 1).Draw(t, "mode") == 1 {
			mode = types.HalfEven
		}
		if _, err := s.SaveUoMFamily(ctx, ledger.UoMFamily{Name: family, Label: family, BaseUnit: baseUnit}, 0); err != nil {
			t.Fatalf("family: %v", err)
		}
		if _, err := s.SaveUoMFamily(ctx, ledger.UoMFamily{Name: other, Label: other, BaseUnit: otherBase}, 0); err != nil {
			t.Fatalf("other family: %v", err)
		}
		if _, err := s.SaveUoMConversion(ctx, ledger.UoMConversion{Family: family, Unit: unit, Factor: factor, Scale: scale}, 0); err != nil {
			t.Fatalf("conversion: %v", err)
		}
		q, err := types.NewDecimal(int64(rapid.IntRange(-99999999, 99999999).Draw(t, "entered unscaled")), rapid.IntRange(0, 8).Draw(t, "entered scale"))
		if err != nil {
			t.Fatalf("entered: %v", err)
		}
		entered, err := types.NewQuantity(q, unit)
		if err != nil {
			t.Fatalf("quantity: %v", err)
		}
		base, stored, err := s.ToBaseQuantity(ctx, entered, "", mode)
		if err != nil {
			t.Fatalf("to base: %v", err)
		}
		if !stored.Equal(factor) {
			t.Fatalf("stored factor %v, want %v", stored, factor)
		}
		back, err := s.FromBaseQuantity(ctx, base, unit, "", mode)
		if err != nil {
			t.Fatalf("from base: %v", err)
		}
		lhs := factor.Mul(back.Value.Sub(q).Abs())
		halfUnit, err := types.NewDecimal(5, scale+1)
		if err != nil {
			t.Fatalf("half unit: %v", err)
		}
		if rhs := halfUnit.Mul(one.Add(factor)); lhs.Cmp(rhs) > 0 {
			t.Fatalf("round trip %v -> %v exceeds scale %d", q, back.Value, scale)
		}
		if _, err := s.ConvertQuantity(ctx, entered, otherBase, "", mode); !errors.Is(err, ledger.ErrCrossFamily) {
			t.Fatalf("cross family: %v", err)
		}
	})
}
