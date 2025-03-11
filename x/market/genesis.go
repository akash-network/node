package market

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	"github.com/akash-network/node/x/market/keeper"
	keys "github.com/akash-network/node/x/market/keeper/keys/v1beta4"
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
func InitGenesis(ctx sdk.Context, kpr keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	store := ctx.KVStore(kpr.StoreKey())
	cdc := kpr.Codec()

	for _, record := range data.Orders {
		key := keys.MustOrderKey(keys.OrderStateToPrefix(record.State), record.ID())

		if store.Has(key) {
			panic(fmt.Errorf("market genesis orders init. order id %s: %w", record.ID(), types.ErrOrderExists))
		}

		store.Set(key, cdc.MustMarshal(&record))
	}

	for _, record := range data.Bids {
		key := keys.MustBidKey(keys.BidStateToPrefix(record.State), record.ID())

		if store.Has(key) {
			panic(fmt.Errorf("market genesis bids init. bid id %s: %w", record.ID(), types.ErrBidExists))
		}

		store.Set(key, cdc.MustMarshal(&record))
	}

	for _, record := range data.Leases {
		key := keys.MustLeaseKey(keys.LeaseStateToPrefix(record.State), record.ID())

		if store.Has(key) {
			panic(fmt.Errorf("market genesis leases init. order id %s: lease exists", record.ID()))
		}

		store.Set(key, cdc.MustMarshal(&record))
	}

	kpr.SetParams(ctx, data.Params)

	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the market module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	params := k.GetParams(ctx)

	var bids []types.Bid
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

	k.WithBids(ctx, func(bid types.Bid) bool {
		bids = append(bids, bid)
		return false
	})

	return &types.GenesisState{
		Params: params,
		Orders: orders,
		Leases: leases,
		Bids:   bids,
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
