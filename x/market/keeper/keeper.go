package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"

	mv1 "pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

type IKeeper interface {
	NewQuerier() Querier
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dvbeta.GroupSpec) (types.Order, error)
	CreateBid(ctx sdk.Context, id mv1.BidID, price sdk.DecCoin, roffer types.ResourcesOffer) (types.Bid, error)
	CreateLease(ctx sdk.Context, bid types.Bid) error
	OnOrderMatched(ctx sdk.Context, order types.Order)
	OnBidMatched(ctx sdk.Context, bid types.Bid)
	OnBidLost(ctx sdk.Context, bid types.Bid)
	OnBidClosed(ctx sdk.Context, bid types.Bid) error
	OnOrderClosed(ctx sdk.Context, order types.Order) error
	OnLeaseClosed(ctx sdk.Context, lease mv1.Lease, state mv1.Lease_State, reason mv1.LeaseClosedReason) error
	OnGroupClosed(ctx sdk.Context, id dtypes.GroupID, state dvbeta.Group_State) error
	GetOrder(ctx sdk.Context, id mv1.OrderID) (types.Order, bool)
	GetBid(ctx sdk.Context, id mv1.BidID) (types.Bid, bool)
	GetLease(ctx sdk.Context, id mv1.LeaseID) (mv1.Lease, bool)
	LeaseForOrder(ctx sdk.Context, bs types.Bid_State, oid mv1.OrderID) (mv1.Lease, bool)
	WithOrders(ctx sdk.Context, fn func(types.Order) bool)
	WithBids(ctx sdk.Context, fn func(types.Bid) bool)
	WithBidsForOrder(ctx sdk.Context, id mv1.OrderID, state types.Bid_State, fn func(types.Bid) bool)
	WithLeases(ctx sdk.Context, fn func(mv1.Lease) bool)
	WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, state types.Order_State, fn func(types.Order) bool)
	BidCountForOrder(ctx sdk.Context, id mv1.OrderID) uint32
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params) error
	GetAuthority() string
}

// Keeper of the market store
type Keeper struct {
	cdc     codec.BinaryCodec
	skey    storetypes.StoreKey
	ekeeper EscrowKeeper
	// The address capable of executing a MsgUpdateParams message.
	// This should be the x/gov module account.
	authority string

	schema collections.Schema
	bids   *collections.IndexedMap[keys.BidPrimaryKey, types.Bid, BidIndexes]
	orders *collections.IndexedMap[keys.OrderPrimaryKey, types.Order, OrderIndexes]
	leases *collections.IndexedMap[keys.LeasePrimaryKey, mv1.Lease, LeaseIndexes]
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryCodec, skey *storetypes.KVStoreKey, ekeeper EscrowKeeper, authority string) IKeeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	bidIndexes := NewBidIndexes(sb)
	orderIndexes := NewOrderIndexes(sb)
	leaseIndexes := NewLeaseIndexes(sb)

	bids := collections.NewIndexedMap(sb, collections.NewPrefix(keys.BidPrefixNew), "bids", keys.BidPrimaryKeyCodec, codec.CollValue[types.Bid](cdc), bidIndexes)
	orders := collections.NewIndexedMap(sb, collections.NewPrefix(keys.OrderPrefixNew), "orders", keys.OrderPrimaryKeyCodec, codec.CollValue[types.Order](cdc), orderIndexes)
	leases := collections.NewIndexedMap(sb, collections.NewPrefix(keys.LeasePrefixNew), "leases", keys.LeasePrimaryKeyCodec, codec.CollValue[mv1.Lease](cdc), leaseIndexes)

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	res := &Keeper{
		skey:      skey,
		cdc:       cdc,
		ekeeper:   ekeeper,
		authority: authority,
		schema:    schema,
		bids:      bids,
		orders:    orders,
		leases:    leases,
	}

	return res
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

// GetAuthority returns the x/mint module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// Bids returns the bid IndexedMap for direct access (used by genesis and migration)
func (k Keeper) Bids() *collections.IndexedMap[keys.BidPrimaryKey, types.Bid, BidIndexes] {
	return k.bids
}

// Orders returns the order IndexedMap for direct access (used by genesis and migration)
func (k Keeper) Orders() *collections.IndexedMap[keys.OrderPrimaryKey, types.Order, OrderIndexes] {
	return k.orders
}

// Leases returns the lease IndexedMap for direct access (used by genesis and migration)
func (k Keeper) Leases() *collections.IndexedMap[keys.LeasePrimaryKey, mv1.Lease, LeaseIndexes] {
	return k.leases
}

// SetParams sets the x/market module parameters.
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(mv1.ParamsPrefix(), bz)

	return nil
}

// GetParams returns the current x/market module parameters.
func (k Keeper) GetParams(ctx sdk.Context) (p types.Params) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(mv1.ParamsPrefix())
	if bz == nil {
		return p
	}

	k.cdc.MustUnmarshal(bz, &p)
	return p
}

// CreateOrder creates a new order with given group id and specifications. It returns created order
func (k Keeper) CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dvbeta.GroupSpec) (types.Order, error) {
	oseq := uint32(1)
	var err error

	k.WithOrdersForGroup(ctx, gid, types.OrderActive, func(_ types.Order) bool {
		err = mv1.ErrOrderActive
		return true
	})

	k.WithOrdersForGroup(ctx, gid, types.OrderOpen, func(_ types.Order) bool {
		err = mv1.ErrOrderActive
		return true
	})

	k.WithOrdersForGroup(ctx, gid, types.OrderClosed, func(_ types.Order) bool {
		oseq++
		return false
	})

	if err != nil {
		return types.Order{}, fmt.Errorf("%w: create order: active order exists", err)
	}

	orderID := mv1.MakeOrderID(gid, oseq)

	pk := keys.OrderIDToKey(orderID)
	has, err := k.orders.Has(ctx, pk)
	if err != nil {
		return types.Order{}, err
	}
	if has {
		return types.Order{}, mv1.ErrOrderExists
	}

	order := types.Order{
		ID:        orderID,
		Spec:      spec,
		State:     types.OrderOpen,
		CreatedAt: ctx.BlockHeight(),
	}

	if err := k.orders.Set(ctx, pk, order); err != nil {
		return types.Order{}, fmt.Errorf("failed to create order: %w", err)
	}

	ctx.Logger().Info("created order", "order", order.ID)

	err = ctx.EventManager().EmitTypedEvent(
		&mv1.EventOrderCreated{ID: order.ID},
	)
	if err != nil {
		return types.Order{}, err
	}

	return order, nil
}

// CreateBid creates a bid for a order with given orderID, price for bid and provider
func (k Keeper) CreateBid(ctx sdk.Context, id mv1.BidID, price sdk.DecCoin, roffer types.ResourcesOffer) (types.Bid, error) {
	pk := keys.BidIDToKey(id)

	has, err := k.bids.Has(ctx, pk)
	if err != nil {
		return types.Bid{}, err
	}
	if has {
		return types.Bid{}, mv1.ErrBidExists
	}

	bid := types.Bid{
		ID:             id,
		State:          types.BidOpen,
		Price:          price,
		CreatedAt:      ctx.BlockHeight(),
		ResourcesOffer: roffer,
	}

	if err := k.bids.Set(ctx, pk, bid); err != nil {
		return types.Bid{}, err
	}

	err = ctx.EventManager().EmitTypedEvent(
		&mv1.EventBidCreated{
			ID:    bid.ID,
			Price: price,
		},
	)
	if err != nil {
		return types.Bid{}, err
	}

	return bid, nil
}

// CreateLease creates lease for bid with given bidID.
// Should only be called by the EndBlock handler or unit tests.
func (k Keeper) CreateLease(ctx sdk.Context, bid types.Bid) error {
	lease := mv1.Lease{
		ID:        mv1.LeaseID(bid.ID),
		State:     mv1.LeaseActive,
		Price:     bid.Price,
		CreatedAt: ctx.BlockHeight(),
	}

	pk := keys.LeaseIDToKey(lease.ID)
	if err := k.leases.Set(ctx, pk, lease); err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventLeaseCreated{
			ID:    lease.ID,
			Price: lease.Price,
		},
	)
	if err != nil {
		return err
	}

	return nil
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
func (k Keeper) OnBidClosed(ctx sdk.Context, bid types.Bid) error {
	switch bid.State {
	case types.BidClosed, types.BidLost:
		return nil
	}

	currState := bid.State
	bid.State = types.BidClosed
	k.updateBid(ctx, bid, currState)

	_ = k.ekeeper.AccountClose(ctx, bid.ID.ToEscrowAccountID())

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventBidClosed{
			ID: bid.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnOrderClosed updates order state to closed
func (k Keeper) OnOrderClosed(ctx sdk.Context, order types.Order) error {
	if order.State == types.OrderClosed {
		return nil
	}

	currState := order.State

	order.State = types.OrderClosed

	k.updateOrder(ctx, order, currState)

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventOrderClosed{
			ID: order.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnLeaseClosed updates lease state to closed
func (k Keeper) OnLeaseClosed(ctx sdk.Context, lease mv1.Lease, state mv1.Lease_State, reason mv1.LeaseClosedReason) error {
	switch lease.State {
	case mv1.LeaseClosed, mv1.LeaseInsufficientFunds:
		return nil
	}

	lease.State = state
	lease.ClosedOn = ctx.BlockHeight()
	lease.Reason = reason

	// IndexedMap.Set automatically updates all indexes
	if err := k.leases.Set(ctx, keys.LeaseIDToKey(lease.ID), lease); err != nil {
		return fmt.Errorf("failed to update lease: %w", err)
	}

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventLeaseClosed{
			ID:     lease.ID,
			Reason: reason,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnGroupClosed updates state of all orders, bids and leases in group to closed
func (k Keeper) OnGroupClosed(ctx sdk.Context, id dtypes.GroupID, state dvbeta.Group_State) error {
	leaseState := mv1.LeaseClosed
	leaseReason := mv1.LeaseClosedReasonOwner

	if state == dvbeta.GroupInsufficientFunds {
		leaseState = mv1.LeaseInsufficientFunds
		leaseReason = mv1.LeaseClosedReasonInsufficientFunds
	}

	processClose := func(ctx sdk.Context, bid types.Bid) error {
		err := k.OnBidClosed(ctx, bid)
		if err != nil {
			return err
		}

		if lease, ok := k.GetLease(ctx, bid.ID.LeaseID()); ok {
			err = k.OnLeaseClosed(ctx, lease, leaseState, leaseReason)
			if err != nil {
				return err
			}
			if err := k.ekeeper.PaymentClose(ctx, lease.ID.ToEscrowPaymentID()); err != nil {
				ctx.Logger().With("err", err).Info("error closing payment")
			}
			if err != nil {
				return err
			}
		}

		return nil
	}

	var err error
	k.WithOrdersForGroup(ctx, id, types.OrderActive, func(order types.Order) bool {
		err = k.OnOrderClosed(ctx, order)
		if err != nil {
			return true
		}

		k.WithBidsForOrder(ctx, order.ID, types.BidOpen, func(bid types.Bid) bool {
			err = processClose(ctx, bid)
			return err != nil
		})

		if err != nil {
			return true
		}

		k.WithBidsForOrder(ctx, order.ID, types.BidActive, func(bid types.Bid) bool {
			err = processClose(ctx, bid)
			return err != nil
		})

		return err != nil
	})

	if err != nil {
		return err
	}

	return nil
}

// GetOrder returns order with given orderID from market store
func (k Keeper) GetOrder(ctx sdk.Context, id mv1.OrderID) (types.Order, bool) {
	order, err := k.orders.Get(ctx, keys.OrderIDToKey(id))
	if err != nil {
		return types.Order{}, false
	}
	return order, true
}

// GetBid returns bid with given bidID from market store
func (k Keeper) GetBid(ctx sdk.Context, id mv1.BidID) (types.Bid, bool) {
	bid, err := k.bids.Get(ctx, keys.BidIDToKey(id))
	if err != nil {
		return types.Bid{}, false
	}
	return bid, true
}

// GetLease returns lease with given leaseID from market store
func (k Keeper) GetLease(ctx sdk.Context, id mv1.LeaseID) (mv1.Lease, bool) {
	lease, err := k.leases.Get(ctx, keys.LeaseIDToKey(id))
	if err != nil {
		return mv1.Lease{}, false
	}
	return lease, true
}

// LeaseForOrder returns lease for order with given ID and lease found status
func (k Keeper) LeaseForOrder(ctx sdk.Context, bs types.Bid_State, oid mv1.OrderID) (mv1.Lease, bool) {
	var value mv1.Lease
	var found bool

	k.WithBidsForOrder(ctx, oid, bs, func(item types.Bid) bool {
		value, found = k.GetLease(ctx, mv1.LeaseID(item.ID))
		return true
	})

	return value, found
}

// WithOrders iterates all orders in market
func (k Keeper) WithOrders(ctx sdk.Context, fn func(types.Order) bool) {
	err := k.orders.Walk(ctx, nil, func(_ keys.OrderPrimaryKey, order types.Order) (bool, error) {
		return fn(order), nil
	})
	if err != nil {
		panic(fmt.Sprintf("WithOrders iteration failed: %v", err))
	}
}

// WithBids iterates all bids in market
func (k Keeper) WithBids(ctx sdk.Context, fn func(types.Bid) bool) {
	err := k.bids.Walk(ctx, nil, func(_ keys.BidPrimaryKey, bid types.Bid) (bool, error) {
		return fn(bid), nil
	})
	if err != nil {
		panic(fmt.Sprintf("WithBids iteration failed: %v", err))
	}
}

// WithLeases iterates all leases in market
func (k Keeper) WithLeases(ctx sdk.Context, fn func(mv1.Lease) bool) {
	err := k.leases.Walk(ctx, nil, func(_ keys.LeasePrimaryKey, lease mv1.Lease) (bool, error) {
		return fn(lease), nil
	})
	if err != nil {
		panic(fmt.Sprintf("WithLeases iteration failed: %v", err))
	}
}

// WithOrdersForGroup iterates all orders of a group in market with given GroupID
func (k Keeper) WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, state types.Order_State, fn func(types.Order) bool) {
	groupPart := collections.Join3(id.Owner, id.DSeq, id.GSeq)
	refKey := collections.Join(groupPart, int32(state))

	iter, err := k.orders.Indexes.GroupState.MatchExact(ctx, refKey)
	if err != nil {
		panic(fmt.Sprintf("WithOrdersForGroup iteration failed: %v", err))
	}

	err = indexes.ScanValues(ctx, k.orders, iter, func(order types.Order) bool {
		return fn(order)
	})
	if err != nil {
		panic(fmt.Sprintf("WithOrdersForGroup scan failed: %v", err))
	}
}

// WithBidsForOrder iterates all bids of an order in market with given OrderID
func (k Keeper) WithBidsForOrder(ctx sdk.Context, id mv1.OrderID, state types.Bid_State, fn func(types.Bid) bool) {
	orderPart := collections.Join4(id.Owner, id.DSeq, id.GSeq, id.OSeq)
	refKey := collections.Join(orderPart, int32(state))

	iter, err := k.bids.Indexes.OrderState.MatchExact(ctx, refKey)
	if err != nil {
		panic(fmt.Sprintf("WithBidsForOrder iteration failed: %v", err))
	}

	err = indexes.ScanValues(ctx, k.bids, iter, func(bid types.Bid) bool {
		return fn(bid)
	})
	if err != nil {
		panic(fmt.Sprintf("WithBidsForOrder scan failed: %v", err))
	}
}

func (k Keeper) BidCountForOrder(ctx sdk.Context, id mv1.OrderID) uint32 {
	orderPart := collections.Join4(id.Owner, id.DSeq, id.GSeq, id.OSeq)
	count := uint32(0)

	for _, state := range []types.Bid_State{types.BidOpen, types.BidActive, types.BidClosed} {
		refKey := collections.Join(orderPart, int32(state))
		iter, err := k.bids.Indexes.OrderState.MatchExact(ctx, refKey)
		if err != nil {
			panic(fmt.Sprintf("BidCountForOrder failed: %v", err))
		}
		for ; iter.Valid(); iter.Next() {
			count++
		}
		_ = iter.Close()
	}

	return count
}

func (k Keeper) updateOrder(ctx sdk.Context, order types.Order, _ types.Order_State) {
	// IndexedMap.Set automatically updates all indexes:
	// - removes old index references via lazyOldValue
	// - creates new index references for the updated order
	if err := k.orders.Set(ctx, keys.OrderIDToKey(order.ID), order); err != nil {
		panic(fmt.Sprintf("failed to update order: %v", err))
	}
}

func (k Keeper) updateBid(ctx sdk.Context, bid types.Bid, _ types.Bid_State) {
	// IndexedMap.Set automatically updates all indexes:
	// - removes old index references via lazyOldValue
	// - creates new index references for the updated bid
	if err := k.bids.Set(ctx, keys.BidIDToKey(bid.ID), bid); err != nil {
		panic(fmt.Sprintf("failed to update bid: %v", err))
	}
}
