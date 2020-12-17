package types

import (
	fmt "fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

var _ paramtypes.ParamSet = (*Params)(nil)

var (
	DefaultBidMinDeposit        = sdk.NewCoin("uakt", sdk.NewInt(50000000))
	defaultOrderMaxBids  uint32 = 20
	maxOrderMaxBids      uint32 = 500
)

const (
	keyBidMinDeposit = "BidMinDeposit"
	keyOrderMaxBids  = "OrderMaxBids"
)

func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair([]byte(keyBidMinDeposit), &p.BidMinDeposit, validateCoin),
		paramtypes.NewParamSetPair([]byte(keyOrderMaxBids), &p.OrderMaxBids, validateOrderMaxBids),
	}
}

func DefaultParams() Params {
	return Params{
		BidMinDeposit: DefaultBidMinDeposit,
		OrderMaxBids:  defaultOrderMaxBids,
	}
}

func (p Params) Validate() error {
	if err := validateCoin(p.BidMinDeposit); err != nil {
		return err
	}

	if err := validateOrderMaxBids(p.OrderMaxBids); err != nil {
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

func validateOrderMaxBids(i interface{}) error {
	val, ok := i.(uint32)

	if !ok {
		return fmt.Errorf("invalid parameter type: %T", i)
	}

	if val == 0 {
		return fmt.Errorf("order max bids too low")
	}

	if val > maxOrderMaxBids {
		return fmt.Errorf("order max bids too high")
	}

	return nil
}
