package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, sdk.Error) {
		switch path[0] {
		case providersPath:
			return queryProviders(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdk.ErrUnknownRequest("unknown query path")
	}
}

func queryProviders(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, sdk.Error) {
	var values Providers
	keeper.WithProviders(ctx, func(obj types.Provider) bool {
		values = append(values, Provider(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}
