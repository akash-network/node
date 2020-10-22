package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
	vestingexported "github.com/cosmos/cosmos-sdk/x/auth/vesting/exported"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/supply/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/exported"
	abci "github.com/tendermint/tendermint/abci/types"
)

// NewQuerier creates and returns a new supply querier instance
func NewQuerier(cdc *codec.Codec, accKeeper types.AccountKeeper, supKeeper types.SupplyKeeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) (res []byte, err error) {
		switch path[0] {
		case circulatingPath:
			return queryCirculatingSupply(ctx, cdc, accKeeper, supKeeper)
		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unknown query for endpoint: %s", path[0])
		}
	}
}

func queryCirculatingSupply(ctx sdk.Context, cdc *codec.Codec, accKeeper types.AccountKeeper,
	supKeeper types.SupplyKeeper) (res []byte, err error) {
	var supplyData Supply
	var totalOriginal, totalVested sdk.Coins

	initialSupply := sdk.NewCoins(sdk.NewCoin("uakt", sdk.NewInt(100000000000000)))

	totalSupply := supKeeper.GetSupply(ctx).GetTotal()

	accKeeper.IterateAccounts(ctx, func(account exported.Account) bool {
		if ma, ok := account.(*supply.ModuleAccount); ok {
			switch ma.Name {
			case staking.NotBondedPoolName, staking.BondedPoolName:
				return false
			}
		}

		va, ok := account.(vestingexported.VestingAccount)
		if !ok {
			return false
		}

		originalVesting := va.GetOriginalVesting()
		delegatedVesting := va.GetDelegatedVesting()
		supplyData.Vesting.Bonded = supplyData.Vesting.Bonded.Add(delegatedVesting...)
		supplyData.Vesting.Unbonded = supplyData.Vesting.Unbonded.Add(originalVesting.Sub(delegatedVesting)...)
		supplyData.Available.Bonded = supplyData.Available.Bonded.Add(va.GetDelegatedFree()...)
		supplyData.Available.Unbonded = supplyData.Available.Unbonded.Add(account.GetCoins()...)

		totalOriginal = totalOriginal.Add(originalVesting...)
		totalVested = totalVested.Add(va.GetVestedCoins(ctx.BlockTime())...)
		return false
	})

	supplyData.Circulating = totalSupply.Add(totalOriginal.Sub(totalVested)...).Sub(initialSupply)

	return sdkutil.RenderQueryResponse(cdc, supplyData)
}
