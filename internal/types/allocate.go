package types

import (
	"errors"
	"math/big"
	"sort"
)

var (
	ErrNoWeights      = errors.New("types: allocation needs at least one weight")
	ErrNegativeWeight = errors.New("types: allocation weights must not be negative")
	ErrZeroWeights    = errors.New("types: allocation weights sum to zero")
)

// AllocateUnits splits total into len(weights) integer shares proportional to
// weights. The shares always add up to total exactly. Each share is the floor
// of its exact portion (on the absolute value); the leftover units go one each
// to the largest fractional remainders, ties to the lowest index. The result
// depends only on the inputs, so a repeat call gives the same split.
func AllocateUnits(total *big.Int, weights []int64) ([]*big.Int, error) {
	if len(weights) == 0 {
		return nil, ErrNoWeights
	}
	sum := new(big.Int)
	for _, w := range weights {
		if w < 0 {
			return nil, ErrNegativeWeight
		}
		sum.Add(sum, big.NewInt(w))
	}
	if sum.Sign() == 0 {
		return nil, ErrZeroWeights
	}
	abs := new(big.Int).Abs(total)
	shares := make([]*big.Int, len(weights))
	rems := make([]*big.Int, len(weights))
	given := new(big.Int)
	for i, w := range weights {
		p := new(big.Int).Mul(abs, big.NewInt(w))
		shares[i], rems[i] = new(big.Int).QuoRem(p, sum, new(big.Int))
		given.Add(given, shares[i])
	}
	left := new(big.Int).Sub(abs, given).Int64() // < len(weights)
	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return rems[order[a]].Cmp(rems[order[b]]) > 0 })
	one := big.NewInt(1)
	for _, i := range order[:left] {
		shares[i].Add(shares[i], one)
	}
	if total.Sign() < 0 {
		for _, s := range shares {
			s.Neg(s)
		}
	}
	return shares, nil
}

// Allocate splits d across weights at the given scale (see AllocateUnits).
// d must be exactly representable at that scale; otherwise it returns
// ErrRounding, because rounding first would break the exact-sum guarantee.
func (d Decimal) Allocate(weights []int64, scale int) ([]Decimal, error) {
	r, err := d.Rescale(scale)
	if err != nil {
		return nil, err
	}
	units, err := AllocateUnits(r.unscaled, weights)
	if err != nil {
		return nil, err
	}
	out := make([]Decimal, len(units))
	for i, u := range units {
		out[i] = newDecimal(u, scale)
	}
	return out, nil
}
