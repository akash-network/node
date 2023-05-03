package market

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/akash-network/node/x/market/keeper"
	types "github.com/akash-network/node/x/market/types/v1beta2"
)

// ValidateGenesis does validation check of the Genesis
func ValidateGenesis(data *types.GenesisState) error {
	if err := data.Params.Validate(); err != nil {
		return err
	}

	return nil
}

// DefaultGenesisState returns default genesis state as raw bytes for the market
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		Params: types.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	keeper.SetParams(ctx, data.Params)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the market module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	params := k.GetParams(ctx)

	var leases []types.Lease
	var orders []types.Order

	k.WithLeases(ctx, func(lease types.Lease) bool {
		leases = append(leases, lease)
		return false
	})

	k.WithOrders(ctx, func(order types.Order) bool {
		orders = append(orders, order)
		return false
	})

	return &types.GenesisState{
		Params: params,
		Orders: orders,
		Leases: leases,
	}
}

// GetGenesisStateFromAppState returns x/market GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
