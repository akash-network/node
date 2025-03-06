package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	keys "github.com/akash-network/node/x/market/keeper/keys/v1beta4"
)

// TODO: use interface for all keepers, queriers
type IKeeper interface {
	NewQuerier() Querier
	Codec() codec.BinaryCodec
	StoreKey() sdk.StoreKey
	CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dtypes.GroupSpec) (types.Order, error)
	CreateBid(ctx sdk.Context, oid types.OrderID, provider sdk.AccAddress, price sdk.DecCoin, roffer types.ResourcesOffer) (types.Bid, error)
	CreateLease(ctx sdk.Context, bid types.Bid)
	OnOrderMatched(ctx sdk.Context, order types.Order)
	OnBidMatched(ctx sdk.Context, bid types.Bid)
	OnBidLost(ctx sdk.Context, bid types.Bid)
	OnBidClosed(ctx sdk.Context, bid types.Bid)
	OnOrderClosed(ctx sdk.Context, order types.Order)
	OnLeaseClosed(ctx sdk.Context, lease types.Lease, state types.Lease_State)
	OnGroupClosed(ctx sdk.Context, id dtypes.GroupID)
	GetOrder(ctx sdk.Context, id types.OrderID) (types.Order, bool)
	GetBid(ctx sdk.Context, id types.BidID) (types.Bid, bool)
	GetLease(ctx sdk.Context, id types.LeaseID) (types.Lease, bool)
	WithOrders(ctx sdk.Context, fn func(types.Order) bool)
	WithBids(ctx sdk.Context, fn func(types.Bid) bool)
	WithLeases(ctx sdk.Context, fn func(types.Lease) bool)
	WithBidsForOrder(ctx sdk.Context, id types.OrderID, state types.Bid_State, fn func(types.Bid) bool)
	BidCountForOrder(ctx sdk.Context, id types.OrderID) uint32
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params)
}

// Keeper of the market store
type Keeper struct {
	cdc     codec.BinaryCodec
	skey    sdk.StoreKey
	pspace  paramtypes.Subspace
	ekeeper EscrowKeeper
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryCodec, skey sdk.StoreKey, pspace paramtypes.Subspace, ekeeper EscrowKeeper) IKeeper {
	if !pspace.HasKeyTable() {
		pspace = pspace.WithKeyTable(types.ParamKeyTable())
	}

	return Keeper{
		skey:    skey,
		cdc:     cdc,
		pspace:  pspace,
		ekeeper: ekeeper,
	}
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k Keeper) StoreKey() sdk.StoreKey {
	return k.skey
}

// CreateOrder creates a new order with given group id and specifications. It returns created order
func (k Keeper) CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dtypes.GroupSpec) (types.Order, error) {
	store := ctx.KVStore(k.skey)

	oseq := uint32(1)
	var err error

	k.WithOrdersForGroup(ctx, gid, types.OrderActive, func(order types.Order) bool {
		err = types.ErrOrderActive
		return true
	})

	k.WithOrdersForGroup(ctx, gid, types.OrderOpen, func(order types.Order) bool {
		err = types.ErrOrderActive
		return true
	})

	k.WithOrdersForGroup(ctx, gid, types.OrderClosed, func(order types.Order) bool {
		oseq++
		return false
	})

	if err != nil {
		return types.Order{}, fmt.Errorf("%w: create order: active order exists", err)
	}

	orderID := types.MakeOrderID(gid, oseq)

	if res := k.findOrder(ctx, orderID); len(res) > 0 {
		return types.Order{}, types.ErrOrderExists
	}

	order := types.Order{
		OrderID:   types.MakeOrderID(gid, oseq),
		Spec:      spec,
		State:     types.OrderOpen,
		CreatedAt: ctx.BlockHeight(),
	}

	key := keys.MustOrderKey(keys.OrderStateOpenPrefix, order.ID())

	store.Set(key, k.cdc.MustMarshal(&order))

	ctx.Logger().Info("created order", "order", order.ID())

	ctx.EventManager().EmitEvent(
		types.NewEventOrderCreated(order.ID()).
			ToSDKEvent(),
	)
	return order, nil
}

// CreateBid creates a bid for a order with given orderID, price for bid and provider
func (k Keeper) CreateBid(ctx sdk.Context, oid types.OrderID, provider sdk.AccAddress, price sdk.DecCoin, roffer types.ResourcesOffer) (types.Bid, error) {
	store := ctx.KVStore(k.skey)

	bidID := types.MakeBidID(oid, provider)

	if key := k.findBid(ctx, bidID); len(key) > 0 {
		return types.Bid{}, types.ErrBidExists
	}

	bid := types.Bid{
		BidID:          bidID,
		State:          types.BidOpen,
		Price:          price,
		CreatedAt:      ctx.BlockHeight(),
		ResourcesOffer: roffer,
	}

	data := k.cdc.MustMarshal(&bid)

	key := keys.MustBidKey(keys.BidStateToPrefix(bid.State), bidID)
	revKey := keys.MustBidStateRevereKey(bid.State, bidID)

	store.Set(key, data)

	if len(revKey) > 0 {
		store.Set(revKey, data)
	}

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
		LeaseID:   types.LeaseID(bid.ID()),
		State:     types.LeaseActive,
		Price:     bid.Price,
		CreatedAt: ctx.BlockHeight(),
	}

	data := k.cdc.MustMarshal(&lease)

	// create (active) lease in store
	key := keys.MustLeaseKey(keys.LeaseStateToPrefix(lease.State), lease.ID())
	revKey := keys.MustLeaseStateReverseKey(lease.State, lease.LeaseID)

	store.Set(key, data)
	if len(revKey) > 0 {
		store.Set(revKey, data)
	}

	ctx.Logger().Info("created lease", "lease", lease.ID())
	ctx.EventManager().EmitEvent(
		types.NewEventLeaseCreated(lease.ID(), lease.Price).
			ToSDKEvent(),
	)
}

// OnOrderMatched updates order state to matched
func (k Keeper) OnOrderMatched(ctx sdk.Context, order types.Order) {
	currState := order.State
	order.State = types.OrderActive
	k.updateOrder(ctx, order, currState)
}

// OnBidMatched updates bid state to matched
func (k Keeper) OnBidMatched(ctx sdk.Context, bid types.Bid) {
	currState := bid.State
	bid.State = types.BidActive
	k.updateBid(ctx, bid, currState)
}

// OnBidLost updates bid state to bid lost
func (k Keeper) OnBidLost(ctx sdk.Context, bid types.Bid) {
	currState := bid.State
	bid.State = types.BidLost
	k.updateBid(ctx, bid, currState)
}

// OnBidClosed updates bid state to closed
func (k Keeper) OnBidClosed(ctx sdk.Context, bid types.Bid) {
	switch bid.State {
	case types.BidClosed, types.BidLost:
		return
	}

	currState := bid.State
	bid.State = types.BidClosed
	k.updateBid(ctx, bid, currState)

	_ = k.ekeeper.AccountClose(ctx, types.EscrowAccountForBid(bid.ID()))

	ctx.EventManager().EmitEvent(
		types.NewEventBidClosed(bid.ID(), bid.Price).
			ToSDKEvent(),
	)
}

// OnOrderClosed updates order state to closed
func (k Keeper) OnOrderClosed(ctx sdk.Context, order types.Order) {
	if order.State == types.OrderClosed {
		return
	}

	currState := order.State

	order.State = types.OrderClosed
	k.updateOrder(ctx, order, currState)

	ctx.EventManager().EmitEvent(
		types.NewEventOrderClosed(order.ID()).
			ToSDKEvent(),
	)
}

// OnLeaseClosed updates lease state to closed
func (k Keeper) OnLeaseClosed(ctx sdk.Context, lease types.Lease, state types.Lease_State) {
	switch lease.State {
	case types.LeaseClosed, types.LeaseInsufficientFunds:
		return
	}

	currState := lease.State
	lease.State = state
	lease.ClosedOn = ctx.BlockHeight()

	store := ctx.KVStore(k.skey)

	key := keys.MustLeaseKey(keys.LeaseStateToPrefix(currState), lease.ID())
	revKey := keys.MustLeaseStateReverseKey(currState, lease.LeaseID)

	store.Delete(key)
	if len(revKey) > 0 {
		store.Delete(revKey)
	}

	key = keys.MustLeaseKey(keys.LeaseStateToPrefix(lease.State), lease.ID())
	store.Set(key, k.cdc.MustMarshal(&lease))

	ctx.EventManager().EmitEvent(
		types.NewEventLeaseClosed(lease.ID(), lease.Price).
			ToSDKEvent(),
	)
}

// OnGroupClosed updates state of all orders, bids and leases in group to closed
func (k Keeper) OnGroupClosed(ctx sdk.Context, id dtypes.GroupID) {
	processClose := func(ctx sdk.Context, bid types.Bid) {
		k.OnBidClosed(ctx, bid)
		if lease, ok := k.GetLease(ctx, bid.ID().LeaseID()); ok {
			k.OnLeaseClosed(ctx, lease, types.LeaseClosed)

			if err := k.ekeeper.PaymentClose(ctx,
				dtypes.EscrowAccountForDeployment(id.DeploymentID()),
				types.EscrowPaymentForLease(lease.ID())); err != nil {
				ctx.Logger().With("err", err).Info("error closing payment")
			}

		}
	}

	k.WithOrdersForGroup(ctx, id, types.OrderActive, func(order types.Order) bool {
		k.OnOrderClosed(ctx, order)

		k.WithBidsForOrder(ctx, order.ID(), types.BidOpen, func(bid types.Bid) bool {
			processClose(ctx, bid)
			return false
		})

		k.WithBidsForOrder(ctx, order.ID(), types.BidActive, func(bid types.Bid) bool {
			processClose(ctx, bid)
			return false
		})

		return false
	})
}

func (k Keeper) findOrder(ctx sdk.Context, id types.OrderID) []byte {
	store := ctx.KVStore(k.skey)
	aKey := keys.MustOrderKey(keys.OrderStateActivePrefix, id)
	oKey := keys.MustOrderKey(keys.OrderStateOpenPrefix, id)
	cKey := keys.MustOrderKey(keys.OrderStateClosedPrefix, id)

	var key []byte

	// nolint: gocritic
	if store.Has(aKey) {
		key = aKey
	} else if store.Has(oKey) {
		key = oKey
	} else if store.Has(cKey) {
		key = cKey
	}

	return key
}

// GetOrder returns order with given orderID from market store
func (k Keeper) GetOrder(ctx sdk.Context, id types.OrderID) (types.Order, bool) {
	key := k.findOrder(ctx, id)

	if len(key) == 0 {
		return types.Order{}, false
	}

	store := ctx.KVStore(k.skey)

	buf := store.Get(key)

	var val types.Order
	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

func (k Keeper) findBid(ctx sdk.Context, id types.BidID) []byte {
	store := ctx.KVStore(k.skey)

	aKey := keys.MustBidKey(keys.BidStateActivePrefix, id)
	oKey := keys.MustBidKey(keys.BidStateOpenPrefix, id)
	lKey := keys.MustBidKey(keys.BidStateLostPrefix, id)
	cKey := keys.MustBidKey(keys.BidStateClosedPrefix, id)

	var key []byte

	// nolint: gocritic
	if store.Has(aKey) {
		key = aKey
	} else if store.Has(oKey) {
		key = oKey
	} else if store.Has(lKey) {
		key = lKey
	} else if store.Has(cKey) {
		key = cKey
	}

	return key
}

// GetBid returns bid with given bidID from market store
func (k Keeper) GetBid(ctx sdk.Context, id types.BidID) (types.Bid, bool) {
	store := ctx.KVStore(k.skey)

	key := k.findBid(ctx, id)

	if len(key) == 0 {
		return types.Bid{}, false
	}

	buf := store.Get(key)

	var val types.Bid
	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

func (k Keeper) findLease(ctx sdk.Context, id types.LeaseID) []byte {
	store := ctx.KVStore(k.skey)

	aKey := keys.MustLeaseKey(keys.LeaseStateActivePrefix, id)
	iKey := keys.MustLeaseKey(keys.LeaseStateInsufficientFundsPrefix, id)
	cKey := keys.MustLeaseKey(keys.LeaseStateClosedPrefix, id)

	var key []byte

	// nolint: gocritic
	if store.Has(aKey) {
		key = aKey
	} else if store.Has(iKey) {
		key = iKey
	} else if store.Has(cKey) {
		key = cKey
	}

	return key
}

// GetLease returns lease with given leaseID from market store
func (k Keeper) GetLease(ctx sdk.Context, id types.LeaseID) (types.Lease, bool) {
	store := ctx.KVStore(k.skey)

	key := k.findLease(ctx, id)

	if len(key) == 0 {
		return types.Lease{}, false
	}

	buf := store.Get(key)

	var val types.Lease
	k.cdc.MustUnmarshal(buf, &val)
	return val, true
}

// WithOrders iterates all orders in market
func (k Keeper) WithOrders(ctx sdk.Context, fn func(types.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.OrderPrefix)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Order
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithBids iterates all bids in market
func (k Keeper) WithBids(ctx sdk.Context, fn func(types.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.BidPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Bid
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithLeases iterates all leases in market
func (k Keeper) WithLeases(ctx sdk.Context, fn func(types.Lease) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.LeasePrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Lease
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithOrdersForGroup iterates all orders of a group in market with given GroupID
func (k Keeper) WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, state types.Order_State, fn func(types.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.OrdersForGroupPrefix(keys.OrderStateToPrefix(state), id))

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Order
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithBidsForOrder iterates all bids of a order in market with given OrderID
func (k Keeper) WithBidsForOrder(ctx sdk.Context, id types.OrderID, state types.Bid_State, fn func(types.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateToPrefix(state), id))

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Bid
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k Keeper) BidCountForOrder(ctx sdk.Context, id types.OrderID) uint32 {
	store := ctx.KVStore(k.skey)
	oiter := sdk.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateOpenPrefix, id))
	aiter := sdk.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateActivePrefix, id))
	citer := sdk.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateClosedPrefix, id))

	defer func() {
		_ = oiter.Close()
		_ = aiter.Close()
		_ = citer.Close()
	}()

	count := uint32(0)
	for ; oiter.Valid(); oiter.Next() {
		count++
	}

	for ; aiter.Valid(); aiter.Next() {
		count++
	}

	for ; citer.Valid(); citer.Next() {
		count++
	}

	return count
}

// GetParams returns the total set of deployment parameters.
func (k Keeper) GetParams(ctx sdk.Context) (params types.Params) {
	k.pspace.GetParamSet(ctx, &params)
	return params
}

// SetParams sets the deployment parameters to the paramspace.
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) {
	k.pspace.SetParamSet(ctx, &params)
}

func (k Keeper) updateOrder(ctx sdk.Context, order types.Order, currState types.Order_State) {
	store := ctx.KVStore(k.skey)

	switch currState {
	case types.OrderOpen:
	case types.OrderActive:
	default:
		panic(fmt.Sprintf("unexpected current state of the order: %d", currState))
	}

	key := keys.MustOrderKey(keys.OrderStateToPrefix(currState), order.ID())
	store.Delete(key)

	switch order.State {
	case types.OrderActive:
	case types.OrderClosed:
	default:
		panic(fmt.Sprintf("unexpected new state of the order: %d", order.State))
	}

	data := k.cdc.MustMarshal(&order)

	key = keys.MustOrderKey(keys.OrderStateToPrefix(order.State), order.ID())
	store.Set(key, data)
}

func (k Keeper) updateBid(ctx sdk.Context, bid types.Bid, currState types.Bid_State) {
	store := ctx.KVStore(k.skey)

	switch currState {
	case types.BidOpen:
	case types.BidActive:
	default:
		panic(fmt.Sprintf("unexpected current state of the bid: %d", currState))
	}

	key := keys.MustBidKey(keys.BidStateToPrefix(currState), bid.ID())
	revKey := keys.MustBidStateRevereKey(currState, bid.ID())
	store.Delete(key)
	if revKey != nil {
		store.Delete(revKey)
	}

	switch bid.State {
	case types.BidActive:
	case types.BidLost:
	case types.BidClosed:
	default:
		panic(fmt.Sprintf("unexpected new state of the bid: %d", bid.State))
	}

	data := k.cdc.MustMarshal(&bid)

	key = keys.MustBidKey(keys.BidStateToPrefix(bid.State), bid.ID())
	revKey = keys.MustBidStateRevereKey(bid.State, bid.ID())

	store.Set(key, data)
	if len(revKey) > 0 {
		store.Set(revKey, data)
	}
}
