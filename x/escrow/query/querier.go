package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/akash-network/node/x/escrow/keeper"
)

// Querier defines a function type that a module querier must implement to handle
// custom client queries.
type Querier = func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error)

func NewQuerier(keeper keeper.Keeper, cdc *codec.LegacyAmino) Querier {
	return nil
}
