package types

import (
	fmt "fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

var _ paramtypes.ParamSet = (*Params)(nil)

var (
	DefaultDeploymentMinDeposit = sdk.NewCoin("uakt", sdk.NewInt(5000000))
)

const (
	keyDeploymentMinDeposit = "DeploymentMinDeposit"
)

func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair([]byte(keyDeploymentMinDeposit), &p.DeploymentMinDeposit, validateCoin),
	}
}

func DefaultParams() Params {
	return Params{
		DeploymentMinDeposit: DefaultDeploymentMinDeposit,
	}
}

func (p Params) Validate() error {
	if err := validateCoin(p.DeploymentMinDeposit); err != nil {
		return err
	}

	return nil
}

func validateCoin(i interface{}) error {
	_, ok := i.(sdk.Coin)
	if !ok {
		return fmt.Errorf("invalid parameter type: %T", i)
	}

	return nil
}
