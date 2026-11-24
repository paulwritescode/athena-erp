package stock

import (
	"fmt"

	"athena-erp/internal/types"
)

// CostMethod values stock movements. ReceiptAverage returns the moving
// average after receiving rcptQty base units at unit cost; IssueUnitCost
// returns the unit cost of an issue from a bin holding binQty base units
// worth binValue. Z3 fixes WeightedAverage for all stock; the interface
// keeps that choice explicit and testable.
type CostMethod interface {
	Name() string
	ReceiptAverage(binQty, binValue, rcptQty, rcptCost types.Decimal) (types.Decimal, error)
	IssueUnitCost(binQty, binValue types.Decimal) (types.Decimal, error)
}

// WeightedAverage is the perpetual weighted (moving) average: every receipt
// reblends the average over the bin, and every issue takes the current
// average. Averages round to LedgerScale with LedgerRounding.
type WeightedAverage struct{}

// Name returns the method name.
func (WeightedAverage) Name() string { return "weighted_average" }

// ReceiptAverage returns (binValue + rcptQty * rcptCost) / (binQty +
// rcptQty). A receipt into an empty bin sets the average to its own cost.
func (WeightedAverage) ReceiptAverage(binQty, binValue, rcptQty, rcptCost types.Decimal) (types.Decimal, error) {
	totalQty := binQty.Add(rcptQty)
	if totalQty.Sign() <= 0 {
		return types.Decimal{}, fmt.Errorf("%w: receipt leaves no quantity to average", ErrInvalid)
	}
	totalValue, err := round6(binValue.Add(rcptQty.Mul(rcptCost)))
	if err != nil {
		return types.Decimal{}, err
	}
	return totalValue.Div(totalQty, LedgerScale, LedgerRounding)
}

// IssueUnitCost returns binValue / binQty. An empty bin has no average.
func (WeightedAverage) IssueUnitCost(binQty, binValue types.Decimal) (types.Decimal, error) {
	if binQty.Sign() <= 0 {
		return types.Decimal{}, fmt.Errorf("%w: empty bin has no average cost", ErrNegativeStock)
	}
	return binValue.Div(binQty, LedgerScale, LedgerRounding)
}
