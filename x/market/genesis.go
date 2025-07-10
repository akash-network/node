package market

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/x/market/keeper"
	"pkg.akt.dev/node/x/market/keeper/keys"
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
func InitGenesis(ctx sdk.Context, kpr keeper.IKeeper, data *v1beta5.GenesisState) {
	store := ctx.KVStore(kpr.StoreKey())
	cdc := kpr.Codec()

	for _, record := range data.Orders {
		key := keys.MustOrderKey(keys.OrderStateToPrefix(record.State), record.ID)

		if store.Has(key) {
			panic(fmt.Errorf("market genesis orders init. order id %s: %w", record.ID, v1.ErrOrderExists))
		}

		store.Set(key, cdc.MustMarshal(&record))
	}

	for _, record := range data.Bids {
		key := keys.MustBidKey(keys.BidStateToPrefix(record.State), record.ID)
		revKey := keys.MustBidReverseKey(keys.BidStateToPrefix(record.State), record.ID)

		if store.Has(key) {
			panic(fmt.Errorf("market genesis bids init. bid id %s: %w", record.ID, v1.ErrBidExists))
		}
		if store.Has(revKey) {
			panic(fmt.Errorf("market genesis bids init. reverse key for bid id %s: %w", record.ID, v1.ErrBidExists))
		}

		data := cdc.MustMarshal(&record)
		store.Set(key, data)
		store.Set(revKey, data)
	}

	for _, record := range data.Leases {
		key := keys.MustLeaseKey(keys.LeaseStateToPrefix(record.State), record.ID)
		revKey := keys.MustLeaseReverseKey(keys.LeaseStateToPrefix(record.State), record.ID)

		if store.Has(key) {
			panic(fmt.Errorf("market genesis leases init. lease id %s: lease exists", record.ID))
		}

		if store.Has(revKey) {
			panic(fmt.Errorf("market genesis leases init. reverse key for lease id %s: lease exists", record.ID))
		}

		data := cdc.MustMarshal(&record)
		store.Set(key, data)
		store.Set(revKey, data)
	}

	err := kpr.SetParams(ctx, data.Params)
	if err != nil {
		panic(err)
	}
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
