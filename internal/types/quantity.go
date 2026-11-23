package types

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrUoM         = errors.New("types: invalid unit of measure")
	ErrUoMMismatch = errors.New("types: unit of measure mismatch")
)

var uomCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// ValidUoM reports whether uom is a well-formed unit-of-measure key: a
// lowercase letter followed by up to 31 lowercase letters, digits or
// underscores. The UoM master reuses it so saved units always parse here.
func ValidUoM(uom string) bool { return uomCode.MatchString(uom) }

// Quantity is an exact decimal amount of one unit of measure. The UoM is a
// key (for example "kg" or "box") that refers to a UoM master; conversion
// factors live in that master and are passed to Convert by the caller.
type Quantity struct {
	Value Decimal
	UoM   string
}

// NewQuantity validates the UoM key and returns a Quantity.
func NewQuantity(v Decimal, uom string) (Quantity, error) {
	if !ValidUoM(uom) {
		return Quantity{}, ErrUoM
	}
	return Quantity{Value: v, UoM: uom}, nil
}

func (q Quantity) sameUoM(o Quantity) error {
	if q.UoM != o.UoM {
		return fmt.Errorf("%w: %s and %s", ErrUoMMismatch, q.UoM, o.UoM)
	}
	return nil
}

// Add returns q + o. Both must use the same UoM.
func (q Quantity) Add(o Quantity) (Quantity, error) {
	if err := q.sameUoM(o); err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: q.Value.Add(o.Value), UoM: q.UoM}, nil
}

// Sub returns q - o. Both must use the same UoM.
func (q Quantity) Sub(o Quantity) (Quantity, error) {
	if err := q.sameUoM(o); err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: q.Value.Sub(o.Value), UoM: q.UoM}, nil
}

// Convert multiplies by factor (target units per one source unit) and rounds
// once to scale fraction digits with mode.
func (q Quantity) Convert(factor Decimal, to string, scale int, mode RoundingMode) (Quantity, error) {
	if !ValidUoM(to) {
		return Quantity{}, ErrUoM
	}
	if factor.Sign() <= 0 {
		return Quantity{}, errors.New("types: conversion factor must be positive")
	}
	v, err := q.Value.Mul(factor).Round(scale, mode)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: v, UoM: to}, nil
}

// FromBase divides by factor (base units per one target unit) and rounds
// once to scale fraction digits with mode. It is the inverse of Convert
// for UoM round trips: ToBase multiplies an entered quantity into base
// units, FromBase divides base units back into the entered unit.
func (q Quantity) FromBase(factor Decimal, to string, scale int, mode RoundingMode) (Quantity, error) {
	if !ValidUoM(to) {
		return Quantity{}, ErrUoM
	}
	if factor.Sign() <= 0 {
		return Quantity{}, errors.New("types: conversion factor must be positive")
	}
	v, err := q.Value.Div(factor, scale, mode)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: v, UoM: to}, nil
}

// String formats the quantity as "12.5 kg".
func (q Quantity) String() string { return q.Value.String() + " " + q.UoM }
