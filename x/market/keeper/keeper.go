package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	dtypesBeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/x/market/keeper/keys"
)

type IKeeper interface {
	NewQuerier() Querier
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dtypesBeta.GroupSpec) (types.Order, error)
	CreateBid(ctx sdk.Context, oid mv1.OrderID, provider sdk.AccAddress, price sdk.DecCoin, roffer types.ResourcesOffer) (types.Bid, error)
	CreateLease(ctx sdk.Context, bid types.Bid) error
	OnOrderMatched(ctx sdk.Context, order types.Order)
	OnBidMatched(ctx sdk.Context, bid types.Bid)
	OnBidLost(ctx sdk.Context, bid types.Bid)
	OnBidClosed(ctx sdk.Context, bid types.Bid) error
	OnOrderClosed(ctx sdk.Context, order types.Order) error
	OnLeaseClosed(ctx sdk.Context, lease mv1.Lease, state mv1.Lease_State) error
	OnGroupClosed(ctx sdk.Context, id dtypes.GroupID) error
	GetOrder(ctx sdk.Context, id mv1.OrderID) (types.Order, bool)
	GetBid(ctx sdk.Context, id mv1.BidID) (types.Bid, bool)
	GetLease(ctx sdk.Context, id mv1.LeaseID) (mv1.Lease, bool)
	LeaseForOrder(ctx sdk.Context, oid mv1.OrderID) (mv1.Lease, bool)
	WithOrders(ctx sdk.Context, fn func(types.Order) bool)
	WithBids(ctx sdk.Context, fn func(types.Bid) bool)
	WithLeases(ctx sdk.Context, fn func(mv1.Lease) bool)
	WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, fn func(types.Order) bool)
	WithBidsForOrder(ctx sdk.Context, id mv1.OrderID, fn func(types.Bid) bool)
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
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, ekeeper EscrowKeeper, authority string) IKeeper {
	return Keeper{
		skey:      skey,
		cdc:       cdc,
		ekeeper:   ekeeper,
		authority: authority,
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
func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

// GetAuthority returns the x/mint module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// SetParams sets the x/market module parameters.
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(types.ParamsPrefix(), bz)

	return nil
}

// GetParams returns the current x/market module parameters.
func (k Keeper) GetParams(ctx sdk.Context) (p types.Params) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(types.ParamsPrefix())
	if bz == nil {
		return p
	}

	k.cdc.MustUnmarshal(bz, &p)
	return p
}

// CreateOrder creates a new order with given group id and specifications. It returns created order
func (k Keeper) CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dtypesBeta.GroupSpec) (types.Order, error) {
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
		return types.Order{}, fmt.Errorf("%w: create order: active order exists", err)
	}

	order := types.Order{
		ID:        mv1.MakeOrderID(gid, oseq),
		Spec:      spec,
		State:     types.OrderOpen,
		CreatedAt: ctx.BlockHeight(),
	}

	key := keys.OrderKey(order.ID)

	if store.Has(key) {
		return types.Order{}, types.ErrOrderExists
	}

	store.Set(key, k.cdc.MustMarshal(&order))

	err = ctx.EventManager().EmitTypedEvent(
		&mv1.EventOrderCreated{ID: order.ID},
	)
	if err != nil {
		return types.Order{}, err
	}

	return order, nil
}

// CreateBid creates a bid for a order with given orderID, price for bid and provider
func (k Keeper) CreateBid(ctx sdk.Context, oid mv1.OrderID, provider sdk.AccAddress, price sdk.DecCoin, roffer types.ResourcesOffer) (types.Bid, error) {
	store := ctx.KVStore(k.skey)

	bid := types.Bid{
		ID:             mv1.MakeBidID(oid, provider),
		State:          types.BidOpen,
		Price:          price,
		CreatedAt:      ctx.BlockHeight(),
		ResourcesOffer: roffer,
	}

	key := keys.BidKey(bid.ID)

	if store.Has(key) {
		return types.Bid{}, types.ErrBidExists
	}

	store.Set(key, k.cdc.MustMarshal(&bid))

	err := ctx.EventManager().EmitTypedEvent(
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
	store := ctx.KVStore(k.skey)

	lease := mv1.Lease{
		ID:        mv1.LeaseID(bid.ID),
		State:     mv1.LeaseActive,
		Price:     bid.Price,
		CreatedAt: ctx.BlockHeight(),
	}

	// create (active) lease in store
	key := keys.LeaseKey(lease.ID)
	store.Set(key, k.cdc.MustMarshal(&lease))

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
	order.State = types.OrderActive
	k.updateOrder(ctx, order)
}

// OnBidMatched updates bid state to matched
func (k Keeper) OnBidMatched(ctx sdk.Context, bid types.Bid) {
	bid.State = types.BidActive
	k.updateBid(ctx, bid)
}

// OnBidLost updates bid state to bid lost
func (k Keeper) OnBidLost(ctx sdk.Context, bid types.Bid) {
	bid.State = types.BidLost
	k.updateBid(ctx, bid)
}

// OnBidClosed updates bid state to closed
func (k Keeper) OnBidClosed(ctx sdk.Context, bid types.Bid) error {
	switch bid.State {
	case types.BidClosed, types.BidLost:
		return nil
	}

	bid.State = types.BidClosed
	k.updateBid(ctx, bid)

	_ = k.ekeeper.AccountClose(ctx, types.EscrowAccountForBid(bid.ID))

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

	order.State = types.OrderClosed
	k.updateOrder(ctx, order)

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
func (k Keeper) OnLeaseClosed(ctx sdk.Context, lease mv1.Lease, state mv1.Lease_State) error {
	switch lease.State {
	case mv1.LeaseClosed, mv1.LeaseInsufficientFunds:
		return nil
	}

	lease.State = state
	lease.ClosedOn = ctx.BlockHeight()
	k.updateLease(ctx, lease)

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventLeaseClosed{
			ID: lease.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnGroupClosed updates state of all orders, bids and leases in group to closed
func (k Keeper) OnGroupClosed(ctx sdk.Context, id dtypes.GroupID) error {
	var err error
	k.WithOrdersForGroup(ctx, id, func(order types.Order) bool {
		err = k.OnOrderClosed(ctx, order)
		if err != nil {
			return false
		}

		k.WithBidsForOrder(ctx, order.ID, func(bid types.Bid) bool {
			err = k.OnBidClosed(ctx, bid)
			if err != nil {
				return false
			}

			if lease, ok := k.GetLease(ctx, mv1.LeaseID(bid.ID)); ok {
				err = k.OnLeaseClosed(ctx, lease, mv1.LeaseClosed)
				if err != nil {
					return false
				}

				err = k.ekeeper.PaymentClose(ctx,
					dtypesBeta.EscrowAccountForDeployment(id.DeploymentID()),
					types.EscrowPaymentForLease(lease.ID))
				if err != nil {
					return false
				}

			}
			return false
		})
		return false
	})

	if err != nil {
		return err
	}

	return nil
}

// GetOrder returns order with given orderID from market store
func (k Keeper) GetOrder(ctx sdk.Context, id mv1.OrderID) (types.Order, bool) {
	store := ctx.KVStore(k.skey)
	key := keys.OrderKey(id)
	if !store.Has(key) {
		return types.Order{}, false
	}

	buf := store.Get(key)

	var val types.Order
	k.cdc.MustUnmarshal(buf, &val)
	return val, true
}

// GetBid returns bid with given bidID from market store
func (k Keeper) GetBid(ctx sdk.Context, id mv1.BidID) (types.Bid, bool) {
	store := ctx.KVStore(k.skey)
	key := keys.BidKey(id)
	if !store.Has(key) {
		return types.Bid{}, false
	}

	buf := store.Get(key)

	var val types.Bid
	k.cdc.MustUnmarshal(buf, &val)
	return val, true
}

// GetLease returns lease with given leaseID from market store
func (k Keeper) GetLease(ctx sdk.Context, id mv1.LeaseID) (mv1.Lease, bool) {
	store := ctx.KVStore(k.skey)
	key := keys.LeaseKey(id)
	if !store.Has(key) {
		return mv1.Lease{}, false
	}

	buf := store.Get(key)

	var val mv1.Lease
	k.cdc.MustUnmarshal(buf, &val)
	return val, true
}

// LeaseForOrder returns lease for order with given ID and lease found status
func (k Keeper) LeaseForOrder(ctx sdk.Context, oid mv1.OrderID) (mv1.Lease, bool) {
	var value mv1.Lease
	var found bool

	k.WithBidsForOrder(ctx, oid, func(item types.Bid) bool {
		if !item.ID.OrderID().Equals(oid) {
			return false
		}
		if item.State != types.BidActive {
			return false
		}
		value, found = k.GetLease(ctx, mv1.LeaseID(item.ID))
		return true
	})

	return value, found
}

// WithOrders iterates all orders in market
func (k Keeper) WithOrders(ctx sdk.Context, fn func(types.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, types.OrderPrefix())

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
	iter := sdk.KVStorePrefixIterator(store, types.BidPrefix())

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
func (k Keeper) WithLeases(ctx sdk.Context, fn func(mv1.Lease) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, types.LeasePrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val mv1.Lease
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithOrdersForGroup iterates all orders of a group in market with given GroupID
func (k Keeper) WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, fn func(types.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.OrdersForGroupPrefix(id))

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
func (k Keeper) WithBidsForOrder(ctx sdk.Context, id mv1.OrderID, fn func(types.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(id))

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

func (k Keeper) BidCountForOrder(ctx sdk.Context, id mv1.OrderID) uint32 {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	count := uint32(0)
	for ; iter.Valid(); iter.Next() {
		count++
	}
	return count
}

func (k Keeper) updateOrder(ctx sdk.Context, order types.Order) {
	store := ctx.KVStore(k.skey)
	key := keys.OrderKey(order.ID)
	store.Set(key, k.cdc.MustMarshal(&order))
}

func (k Keeper) updateBid(ctx sdk.Context, bid types.Bid) {
	store := ctx.KVStore(k.skey)
	key := keys.BidKey(bid.ID)
	store.Set(key, k.cdc.MustMarshal(&bid))
}

func (k Keeper) updateLease(ctx sdk.Context, lease mv1.Lease) {
	store := ctx.KVStore(k.skey)
	key := keys.LeaseKey(lease.ID)
	store.Set(key, k.cdc.MustMarshal(&lease))
}
