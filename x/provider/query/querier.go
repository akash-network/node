package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// NewQuerier creates and returns a new provider querier instance
func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[0] {
		case providersPath:
			return queryProviders(ctx, path[1:], req, keeper)
		case providerPath:
			return queryProvider(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func queryProviders(ctx sdk.Context, _ []string, _ abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Providers
	keeper.WithProviders(ctx, func(obj types.Provider) bool {
		values = append(values, Provider(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryProvider(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	if len(path) != 1 {
		return nil, sdkerrors.ErrInvalidRequest
	}

	id, err := sdk.AccAddressFromBech32(path[0])
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	provider, ok := keeper.Get(ctx, id)
	if !ok {
		return nil, types.ErrProviderNotFound
	}

	return sdkutil.RenderQueryResponse(keeper.Codec(), Provider(provider))
}
