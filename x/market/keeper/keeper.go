package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/pkg/errors"
)

const (
	// TODO: parameterize somewhere.
	orderTTL = 5 // blocks
)

// Keeper of the market store
type Keeper struct {
	cdc  *codec.Codec
	skey sdk.StoreKey
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc *codec.Codec, skey sdk.StoreKey) Keeper {
	return Keeper{cdc: cdc, skey: skey}
}

// Codec returns keeper codec
func (k Keeper) Codec() *codec.Codec {
	return k.cdc
}

// CreateOrder creates a new order with given group id and specifications. It returns created order
func (k Keeper) CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dtypes.GroupSpec) (types.Order, error) {
	store := ctx.KVStore(k.skey)

	oseq := uint32(1)
	var err error

	k.WithOrdersForGroup(ctx, gid, func(order types.Order) bool {
		if err = order.ValidateInactive(); err != nil {
			return true
		}
		oseq++
		return false
	})

	if err != nil {
		return types.Order{}, errors.Wrap(err, "create order: active order exists")
	}

	order := types.Order{
		OrderID: types.MakeOrderID(gid, oseq),
		Spec:    spec,
		State:   types.OrderOpen,
		StartAt: ctx.BlockHeight() + orderTTL, // TODO: check overflow
	}

	key := orderKey(order.ID())

	// XXX TODO: check not overwrite
	store.Set(key, k.cdc.MustMarshalBinaryBare(order))

	ctx.Logger().Info("created order", "order", order.ID())
	ctx.EventManager().EmitEvent(
		types.NewEventOrderCreated(order.ID()).
			ToSDKEvent(),
	)
	return order, nil
}

// CreateBid creates a bid for a order with given orderID, price for bid and provider
func (k Keeper) CreateBid(ctx sdk.Context, oid types.OrderID, provider sdk.AccAddress, price sdk.Coin) (types.Bid, error) {
	store := ctx.KVStore(k.skey)

	bid := types.Bid{
		BidID: types.MakeBidID(oid, provider),
		State: types.BidOpen,
		Price: price,
	}

	key := bidKey(bid.ID())

	if store.Has(key) {
		return types.Bid{}, types.ErrBidExists
	}

	// XXX TODO: check not overwrite
	store.Set(key, k.cdc.MustMarshalBinaryBare(bid))

	ctx.EventManager().EmitEvent(
		types.NewEventBidCreated(bid.ID(), price).
			ToSDKEvent(),
	)

	return bid, nil
}

// CreateLease creates lease for bid with given bidID.
// Should only be called by the EndBlock handler or unit tests.
func (k Keeper) CreateLease(ctx sdk.Context, bid types.Bid) {
	store := ctx.KVStore(k.skey)

	lease := types.Lease{
		LeaseID: types.LeaseID(bid.ID()),
		State:   types.LeaseActive,
		Price:   bid.Price,
	}

	// create (active) lease in store
	key := leaseKey(lease.ID())
	store.Set(key, k.cdc.MustMarshalBinaryBare(lease))
	k.updateActiveLeaseIndex(store, lease)

	ctx.Logger().Info("created lease", "lease", lease.ID())
	ctx.EventManager().EmitEvent(
		types.NewEventLeaseCreated(lease.ID(), lease.Price).
			ToSDKEvent(),
	)
}

// OnOrderMatched updates order state to matched
func (k Keeper) OnOrderMatched(ctx sdk.Context, order types.Order) {
	// TODO: assert state transition
	order.State = types.OrderMatched
	k.updateOrder(ctx, order)
}

// OnBidMatched updates bid state to matched
func (k Keeper) OnBidMatched(ctx sdk.Context, bid types.Bid) {
	// TODO: assert state transition
	bid.State = types.BidMatched
	k.updateBid(ctx, bid)
}

// OnBidLost updates bid state to bid lost
func (k Keeper) OnBidLost(ctx sdk.Context, bid types.Bid) {
	// TODO: assert state transition
	bid.State = types.BidLost
	k.updateBid(ctx, bid)
}

// OnBidClosed updates bid state to closed
func (k Keeper) OnBidClosed(ctx sdk.Context, bid types.Bid) {
	// TODO: assert state transition
	switch bid.State {
	case types.BidClosed, types.BidLost:
		return
	}
	bid.State = types.BidClosed
	k.updateBid(ctx, bid)
	ctx.EventManager().EmitEvent(
		types.NewEventBidClosed(bid.ID(), bid.Price).
			ToSDKEvent(),
	)
}

// OnOrderClosed updates order state to closed
func (k Keeper) OnOrderClosed(ctx sdk.Context, order types.Order) {
	// TODO: assert state transition
	if order.State == types.OrderClosed {
		return
	}

	order.State = types.OrderClosed
	k.updateOrder(ctx, order)
	ctx.EventManager().EmitEvent(
		types.NewEventOrderClosed(order.ID()).
			ToSDKEvent(),
	)
}

// OnInsufficientFunds updates lease state to insufficient funds
func (k Keeper) OnInsufficientFunds(ctx sdk.Context, lease types.Lease) {
	// TODO: assert state transition
	switch lease.State {
	case types.LeaseClosed, types.LeaseInsufficientFunds:
		return
	}
	lease.State = types.LeaseInsufficientFunds
	k.updateLease(ctx, lease)
	ctx.EventManager().EmitEvent(
		types.NewEventLeaseClosed(lease.ID(), lease.Price).
			ToSDKEvent(),
	)
}

// OnLeaseClosed updates lease state to closed
func (k Keeper) OnLeaseClosed(ctx sdk.Context, lease types.Lease) {
	// TODO: assert state transition
	switch lease.State {
	case types.LeaseClosed, types.LeaseInsufficientFunds:
		return
	}
	lease.State = types.LeaseClosed
	k.updateLease(ctx, lease)
	ctx.Logger().Info("closed lease", "lease", lease.ID())
	ctx.EventManager().EmitEvent(
		types.NewEventLeaseClosed(lease.ID(), lease.Price).
			ToSDKEvent(),
	)
}

// OnGroupClosed updates state of all orders, bids and leases in group to closed
func (k Keeper) OnGroupClosed(ctx sdk.Context, id dtypes.GroupID) {
	k.WithOrdersForGroup(ctx, id, func(order types.Order) bool {
		k.OnOrderClosed(ctx, order)
		k.WithBidsForOrder(ctx, order.ID(), func(bid types.Bid) bool {
			k.OnBidClosed(ctx, bid)
			if lease, ok := k.GetLease(ctx, types.LeaseID(bid.ID())); ok {
				// TODO: emit events
				k.OnLeaseClosed(ctx, lease)
			}
			return false
		})
		return false
	})
}

// GetOrder returns order with given orderID from market store
func (k Keeper) GetOrder(ctx sdk.Context, id types.OrderID) (types.Order, bool) {
	store := ctx.KVStore(k.skey)
	key := orderKey(id)
	if !store.Has(key) {
		return types.Order{}, false
	}

	buf := store.Get(key)

	var val types.Order
	k.cdc.MustUnmarshalBinaryBare(buf, &val)
	return val, true
}

// GetBid returns bid with given bidID from market store
func (k Keeper) GetBid(ctx sdk.Context, id types.BidID) (types.Bid, bool) {
	store := ctx.KVStore(k.skey)
	key := bidKey(id)
	if !store.Has(key) {
		return types.Bid{}, false
	}

	buf := store.Get(key)

	var val types.Bid
	k.cdc.MustUnmarshalBinaryBare(buf, &val)
	return val, true
}

// GetLease returns lease with given leaseID from market store
func (k Keeper) GetLease(ctx sdk.Context, id types.LeaseID) (types.Lease, bool) {
	store := ctx.KVStore(k.skey)
	key := leaseKey(id)
	if !store.Has(key) {
		return types.Lease{}, false
	}

	buf := store.Get(key)

	var val types.Lease
	k.cdc.MustUnmarshalBinaryBare(buf, &val)
	return val, true
}

// LeaseForOrder returns lease for order with given ID and lease found status
func (k Keeper) LeaseForOrder(ctx sdk.Context, oid types.OrderID) (types.Lease, bool) {
	var (
		value types.Lease
		found bool
	)

	k.WithBidsForOrder(ctx, oid, func(item types.Bid) bool {
		if !item.OrderID().Equals(oid) {
			return false
		}
		if item.State != types.BidMatched {
			return false
		}
		value, found = k.GetLease(ctx, types.LeaseID(item.ID()))
		return true
	})

	return value, found
}

// WithOrders iterates all orders in market
func (k Keeper) WithOrders(ctx sdk.Context, fn func(types.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, orderPrefix)
	for ; iter.Valid(); iter.Next() {
		var val types.Order
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithBids iterates all bids in market
func (k Keeper) WithBids(ctx sdk.Context, fn func(types.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, bidPrefix)
	for ; iter.Valid(); iter.Next() {
		var val types.Bid
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithLeases iterates all leases in market
func (k Keeper) WithLeases(ctx sdk.Context, fn func(types.Lease) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, leasePrefix)
	for ; iter.Valid(); iter.Next() {
		var val types.Lease
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithActiveLeases iterates through all leases that were marked as Active
// and
func (k Keeper) WithActiveLeases(ctx sdk.Context, fn func(types.Lease) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, leaseActivePrefix)
	for ; iter.Valid(); iter.Next() {
		dataKey, err := convertLeaseActiveKey(iter.Key())
		if err != nil {
			continue
		}

		buf := store.Get(dataKey)
		var val types.Lease
		k.cdc.MustUnmarshalBinaryBare(buf, &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithOrdersForGroup iterates all orders of a group in market with given GroupID
func (k Keeper) WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, fn func(types.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, ordersForGroupPrefix(id))
	for ; iter.Valid(); iter.Next() {
		var val types.Order
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithBidsForOrder iterates all bids of a order in market with given OrderID
func (k Keeper) WithBidsForOrder(ctx sdk.Context, id types.OrderID, fn func(types.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, bidsForOrderPrefix(id))
	for ; iter.Valid(); iter.Next() {
		var val types.Bid
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k Keeper) updateOrder(ctx sdk.Context, order types.Order) {
	store := ctx.KVStore(k.skey)
	key := orderKey(order.ID())
	store.Set(key, k.cdc.MustMarshalBinaryBare(order))
}

func (k Keeper) updateBid(ctx sdk.Context, bid types.Bid) {
	store := ctx.KVStore(k.skey)
	key := bidKey(bid.ID())
	store.Set(key, k.cdc.MustMarshalBinaryBare(bid))
}

func (k Keeper) updateLease(ctx sdk.Context, lease types.Lease) {
	store := ctx.KVStore(k.skey)
	key := leaseKey(lease.ID())
	store.Set(key, k.cdc.MustMarshalBinaryBare(lease))
	k.updateActiveLeaseIndex(store, lease)
}

func (k Keeper) updateActiveLeaseIndex(store sdk.KVStore, lease types.Lease) {
	key := leaseKeyActive(lease.ID())

	switch lease.State {
	case types.LeaseActive:
		store.Set(key, []byte{})
	default:
		store.Delete(key)

	}
}
