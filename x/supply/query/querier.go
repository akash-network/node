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

	supplyData.Total = supKeeper.GetSupply(ctx).GetTotal()

	accKeeper.IterateAccounts(ctx, func(account exported.Account) bool {
		if ma, ok := account.(*supply.ModuleAccount); ok {
			switch ma.Name {
			case staking.NotBondedPoolName, staking.BondedPoolName:
				return false
			}
		}

		va, ok := account.(vestingexported.VestingAccount)
		if !ok {
			supplyData.Available.Bonded = supplyData.Available.Bonded.Add(account.GetCoins().Sub(account.SpendableCoins(ctx.BlockTime()))...)
			supplyData.Available.Unbonded = supplyData.Available.Unbonded.Add(account.SpendableCoins(ctx.BlockTime())...)
		} else {
			supplyData.Available.Bonded = supplyData.Available.Bonded.Add(va.GetDelegatedFree()...)
			supplyData.Available.Unbonded = supplyData.Available.Unbonded.Add(va.SpendableCoins(ctx.BlockTime())...)
			supplyData.Vesting.Bonded = supplyData.Vesting.Bonded.Add(va.GetDelegatedVesting()...)
			supplyData.Vesting.Unbonded = supplyData.Vesting.Unbonded.Add(va.GetVestingCoins(ctx.BlockTime())...).Sub(va.GetDelegatedVesting())
		}

		return false
	})

	supplyData.Circulating = supplyData.Available.Unbonded.Add(supplyData.Available.Bonded...)

	return sdkutil.RenderQueryResponse(cdc, supplyData)
}
