package stock

import (
	"testing"

	"athena-erp/internal/types"
)

func TestWeightedAverageReceiptBlends(t *testing.T) {
	method := WeightedAverage{}
	average, err := method.ReceiptAverage(types.MustDecimal("100"), types.MustDecimal("1000"), types.MustDecimal("50"), types.MustDecimal("16"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := average.String(), "12.000000"; got != want {
		t.Fatalf("average is %s, want %s", got, want)
	}
}

func TestWeightedAverageFirstReceiptSetsCost(t *testing.T) {
	method := WeightedAverage{}
	average, err := method.ReceiptAverage(types.Decimal{}, types.Decimal{}, types.MustDecimal("25"), types.MustDecimal("4.5"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := average.String(), "4.500000"; got != want {
		t.Fatalf("average is %s, want %s", got, want)
	}
}

func TestWeightedAverageIssueTakesAverage(t *testing.T) {
	method := WeightedAverage{}
	cost, err := method.IssueUnitCost(types.MustDecimal("150"), types.MustDecimal("1800"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cost.String(), "12.000000"; got != want {
		t.Fatalf("issue cost is %s, want %s", got, want)
	}
}

func TestWeightedAverageEmptyBinHasNoCost(t *testing.T) {
	method := WeightedAverage{}
	if _, err := method.IssueUnitCost(types.Decimal{}, types.Decimal{}); err == nil {
		t.Fatal("empty bin issue has a cost, want an error")
	}
}

func TestWeightedAverageRoundsHalfUp(t *testing.T) {
	method := WeightedAverage{}
	cost, err := method.IssueUnitCost(types.MustDecimal("3"), types.MustDecimal("10"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cost.String(), "3.333333"; got != want {
		t.Fatalf("issue cost is %s, want %s", got, want)
	}
}
