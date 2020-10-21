package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
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
func NewQuerier(cdc *codec.Codec, accKeeper types.AccountKeeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) (res []byte, err error) {
		switch path[0] {
		case circulatingPath:
			return queryCirculatingSupply(ctx, cdc, accKeeper)
		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unknown query for endpoint: %s", path[0])
		}
	}
}

func queryCirculatingSupply(ctx sdk.Context, cdc *codec.Codec, accKeeper types.AccountKeeper) (res []byte, err error) {
	var total sdk.Coins

	accKeeper.IterateAccounts(ctx, func(account exported.Account) bool {
		if ma, ok := account.(*supply.ModuleAccount); ok {
			switch ma.Name {
			case staking.NotBondedPoolName, staking.BondedPoolName:
				return false
			}
		}

		coins := account.SpendableCoins(ctx.BlockTime())
		total = total.Add(coins...)
		return false
	})

	return sdkutil.RenderQueryResponse(cdc, total)
}
