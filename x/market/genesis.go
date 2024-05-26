package market

import (
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/akashd/x/market/keeper"
)

// ValidateGenesis does validation check of the Genesis
func ValidateGenesis(data *v1beta5.GenesisState) error {
	if err := data.Params.Validate(); err != nil {
		return err
	}

	return nil
}

// DefaultGenesisState returns default genesis state as raw bytes for the market
// module.
func DefaultGenesisState() *v1beta5.GenesisState {
	return &v1beta5.GenesisState{
		Params: v1beta5.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *v1beta5.GenesisState) []abci.ValidatorUpdate {
	keeper.SetParams(ctx, data.Params)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the market module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *v1beta5.GenesisState {
	params := k.GetParams(ctx)

	var bids v1beta5.Bids
	var leases v1.Leases
	var orders v1beta5.Orders

	k.WithLeases(ctx, func(lease v1.Lease) bool {
		leases = append(leases, lease)
		return false
	})

	k.WithOrders(ctx, func(order v1beta5.Order) bool {
		orders = append(orders, order)
		return false
	})

	k.WithBids(ctx, func(bid v1beta5.Bid) bool {
		bids = append(bids, bid)
		return false
	})

	return &v1beta5.GenesisState{
		Params: params,
		Orders: orders,
		Leases: leases,
		Bids:   bids,
	}
}

// GetGenesisStateFromAppState returns x/market GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *v1beta5.GenesisState {
	var genesisState v1beta5.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
