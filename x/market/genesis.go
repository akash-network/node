package market

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/v2/x/market/keeper"
	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

// ValidateGenesis does validation check of the Genesis
func ValidateGenesis(data *mtypes.GenesisState) error {
	if err := data.Params.Validate(); err != nil {
		return err
	}

	return nil
}

// DefaultGenesisState returns default genesis state as raw bytes for the market
// module.
func DefaultGenesisState() *mtypes.GenesisState {
	return &mtypes.GenesisState{
		Params: mtypes.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, kpr keeper.IKeeper, data *mtypes.GenesisState) {
	store := ctx.KVStore(kpr.StoreKey())
	cdc := kpr.Codec()

	for _, record := range data.Orders {
		key := keys.MustOrderKey(keys.OrderStateToPrefix(record.State), record.ID)

		if store.Has(key) {
			panic(fmt.Errorf("market genesis orders init. order id %s: %w", record.ID, mv1.ErrOrderExists))
		}

		store.Set(key, cdc.MustMarshal(&record))
	}

	for _, record := range data.Bids {
		key := keys.MustBidKey(keys.BidStateToPrefix(record.State), record.ID)
		revKey := keys.MustBidReverseKey(keys.BidStateToPrefix(record.State), record.ID)

		if store.Has(key) {
			panic(fmt.Errorf("market genesis bids init. bid id %s: %w", record.ID, mv1.ErrBidExists))
		}
		if store.Has(revKey) {
			panic(fmt.Errorf("market genesis bids init. reverse key for bid id %s: %w", record.ID, mv1.ErrBidExists))
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
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *mtypes.GenesisState {
	params := k.GetParams(ctx)

	var bids mtypes.Bids
	var leases mv1.Leases
	var orders mtypes.Orders

	k.WithLeases(ctx, func(lease mv1.Lease) bool {
		leases = append(leases, lease)
		return false
	})

	k.WithOrders(ctx, func(order mtypes.Order) bool {
		orders = append(orders, order)
		return false
	})

	k.WithBids(ctx, func(bid mtypes.Bid) bool {
		bids = append(bids, bid)
		return false
	})

	return &mtypes.GenesisState{
		Params: params,
		Orders: orders,
		Leases: leases,
		Bids:   bids,
	}
}

// GetGenesisStateFromAppState returns x/market GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *mtypes.GenesisState {
	var genesisState mtypes.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
