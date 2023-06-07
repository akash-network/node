package testnetify

import (
	"encoding/hex"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (ga *GenesisState) modifyValidators(cdc codec.Codec, cfg *ValidatorsConfig) error {
	for _, val := range cfg.Add {
		addr, err := hex.DecodeString(val.PubKey.PubKey.Address().String())
		if err != nil {
			return err
		}

		operatorAddress := sdk.ValAddress(addr)

		if err = ga.AddNewValidator(cdc, operatorAddress, val.PubKey.PubKey, val.Name, val.Rates); err != nil {
			return err
		}

		for _, delegator := range val.Delegators {
			err = ga.IncreaseDelegatorStake(
				cdc,
				delegator.Address.AccAddress,
				operatorAddress,
				delegator.Coins.ToSDK(),
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
