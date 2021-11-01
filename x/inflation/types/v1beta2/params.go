package v1beta2

import (
	"strconv"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/pkg/errors"
)

var _ paramtypes.ParamSet = (*Params)(nil)

const (
	DefaultInflationDecayFactor uint32 = 2 // years
	DefaultInitialInflation     string = "100.0"
	DefaultVarince              string = "0.05"

	keyInflationDecayFactor = "InflationDecayFactor"
	keyInitialInflation     = "InitialInflation"
	keyVariance             = "Variance"
)

func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair([]byte(keyInflationDecayFactor), &p.InflationDecayFactor, validateInflationDecayFactor),
		paramtypes.NewParamSetPair([]byte(keyInitialInflation), &p.InitialInflation, validateInitialInflation),
		paramtypes.NewParamSetPair([]byte(keyVariance), &p.Variance, validateVariance),
	}
}

func DefaultParams() Params {
	return Params{
		InflationDecayFactor: DefaultInflationDecayFactor,
		InitialInflation:     DefaultInitialInflation,
		Variance:             DefaultVarince,
	}
}

func (p Params) Validate() error {
	if err := validateInflationDecayFactor(p.InflationDecayFactor); err != nil {
		return err
	}
	if err := validateInitialInflation(p.InitialInflation); err != nil {
		return err
	}
	if err := validateVariance(p.Variance); err != nil {
		return err
	}

	return nil
}

func validateInflationDecayFactor(i interface{}) error {
	v, ok := i.(uint32)
	if !ok || v < 1 {
		return errors.Wrapf(ErrInvalidParam, "%T", i)
	}

	return nil
}

func validateInitialInflation(i interface{}) error {
	v, ok := i.(string)
	if !ok {
		return errors.Wrapf(ErrInvalidParam, "%T", i)
	}
	initialInflation, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return err
	}
	if initialInflation > 100.0 {
		return errors.Wrapf(ErrInvalidInitialInflation, "%v", initialInflation)
	}

	return nil
}

func validateVariance(i interface{}) error {
	v, ok := i.(string)
	if !ok {
		return errors.Wrapf(ErrInvalidParam, "%T", i)
	}
	variance, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return err
	}
	if variance < 0.0 || variance > 1.0 {
		return errors.Wrapf(ErrInvalidVariance, "%v", variance)
	}

	return nil
}
