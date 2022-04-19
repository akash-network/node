package v1beta2

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/pkg/errors"
)

var _ paramtypes.ParamSet = (*Params)(nil)

const (
	keyInflationDecayFactor = "InflationDecayFactor"
	keyInitialInflation     = "InitialInflation"
	keyVariance             = "Variance"
)

func DefaultInflationDecayFactor() sdk.Dec { return sdk.NewDec(2) } // years

func DefaultInitialInflation() sdk.Dec { return sdk.NewDec(100) }
func DefaultVarince() sdk.Dec          { return sdk.MustNewDecFromStr("0.05") }

func MaxInitialInflation() sdk.Dec { return sdk.NewDec(100) }
func MinInitialInflation() sdk.Dec { return sdk.ZeroDec() }

func MaxVariance() sdk.Dec { return sdk.NewDec(1) }
func MinVariance() sdk.Dec { return sdk.ZeroDec() }

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
		InflationDecayFactor: DefaultInflationDecayFactor(),
		InitialInflation:     DefaultInitialInflation(),
		Variance:             DefaultVarince(),
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
	v, ok := i.(sdk.Dec)
	if !ok || v.LT(sdk.NewDec(1)) {
		return errors.Wrapf(ErrInvalidParam, "%T", i)
	}

	return nil
}

func validateInitialInflation(i interface{}) error {
	v, ok := i.(sdk.Dec)
	if !ok {
		return errors.Wrapf(ErrInvalidParam, "%T", i)
	}
	if v.GT(MaxInitialInflation()) || v.LT(MinInitialInflation()) {
		return errors.Wrapf(ErrInvalidInitialInflation, "%v", v)
	}

	return nil
}

func validateVariance(i interface{}) error {
	v, ok := i.(sdk.Dec)
	if !ok {
		return errors.Wrapf(ErrInvalidParam, "%T", i)
	}
	if v.GT(MaxVariance()) || v.LT(MinVariance()) {
		return errors.Wrapf(ErrInvalidVariance, "%v", v)
	}

	return nil
}
