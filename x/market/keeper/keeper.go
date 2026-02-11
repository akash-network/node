package keeper

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

type IKeeper interface {
	NewQuerier() Querier
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dvbeta.GroupSpec) (mtypes.Order, error)
	CreateBid(ctx sdk.Context, id mv1.BidID, price sdk.DecCoin, roffer mtypes.ResourcesOffer) (mtypes.Bid, error)
	CreateLease(ctx sdk.Context, bid mtypes.Bid) error
	OnOrderMatched(ctx sdk.Context, order mtypes.Order)
	OnBidMatched(ctx sdk.Context, bid mtypes.Bid)
	OnBidLost(ctx sdk.Context, bid mtypes.Bid)
	OnBidClosed(ctx sdk.Context, bid mtypes.Bid) error
	OnOrderClosed(ctx sdk.Context, order mtypes.Order) error
	OnLeaseClosed(ctx sdk.Context, lease mv1.Lease, state mv1.Lease_State, reason mv1.LeaseClosedReason) error
	OnGroupClosed(ctx sdk.Context, id dtypes.GroupID, state dvbeta.Group_State) error
	GetOrder(ctx sdk.Context, id mv1.OrderID) (mtypes.Order, bool)
	GetBid(ctx sdk.Context, id mv1.BidID) (mtypes.Bid, bool)
	GetLease(ctx sdk.Context, id mv1.LeaseID) (mv1.Lease, bool)
	LeaseForOrder(ctx sdk.Context, bs mtypes.Bid_State, oid mv1.OrderID) (mv1.Lease, bool)
	WithOrders(ctx sdk.Context, fn func(mtypes.Order) bool)
	WithBids(ctx sdk.Context, fn func(mtypes.Bid) bool)
	WithBidsForOrder(ctx sdk.Context, id mv1.OrderID, state mtypes.Bid_State, fn func(mtypes.Bid) bool)
	WithLeases(ctx sdk.Context, fn func(mv1.Lease) bool)
	WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, state mtypes.Order_State, fn func(mtypes.Order) bool)
	BidCountForOrder(ctx sdk.Context, id mv1.OrderID) uint32
	GetParams(ctx sdk.Context) (params mtypes.Params)
	SetParams(ctx sdk.Context, params mtypes.Params) error
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
func (k Keeper) SetParams(ctx sdk.Context, p mtypes.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(mv1.ParamsPrefix(), bz)

	return nil
}

// GetParams returns the current x/market module parameters.
func (k Keeper) GetParams(ctx sdk.Context) (p mtypes.Params) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(mv1.ParamsPrefix())
	if bz == nil {
		return p
	}

	k.cdc.MustUnmarshal(bz, &p)
	return p
}

// CreateOrder creates a new order with given group id and specifications. It returns created order
func (k Keeper) CreateOrder(ctx sdk.Context, gid dtypes.GroupID, spec dvbeta.GroupSpec) (mtypes.Order, error) {
	store := ctx.KVStore(k.skey)

	oseq := uint32(1)
	var err error

	k.WithOrdersForGroup(ctx, gid, mtypes.OrderActive, func(_ mtypes.Order) bool {
		err = mv1.ErrOrderActive
		return true
	})

	k.WithOrdersForGroup(ctx, gid, mtypes.OrderOpen, func(_ mtypes.Order) bool {
		err = mv1.ErrOrderActive
		return true
	})

	k.WithOrdersForGroup(ctx, gid, mtypes.OrderClosed, func(_ mtypes.Order) bool {
		oseq++
		return false
	})

	if err != nil {
		return mtypes.Order{}, fmt.Errorf("%w: create order: active order exists", err)
	}

	orderID := mv1.MakeOrderID(gid, oseq)

	if res := k.findOrder(ctx, orderID); len(res) > 0 {
		return mtypes.Order{}, mv1.ErrOrderExists
	}

	order := mtypes.Order{
		ID:        mv1.MakeOrderID(gid, oseq),
		Spec:      spec,
		State:     mtypes.OrderOpen,
		CreatedAt: ctx.BlockHeight(),
	}

	key := keys.MustOrderKey(keys.OrderStateOpenPrefix, order.ID)
	store.Set(key, k.cdc.MustMarshal(&order))

	ctx.Logger().Info("created order", "order", order.ID)

	err = ctx.EventManager().EmitTypedEvent(
		&mv1.EventOrderCreated{ID: order.ID},
	)
	if err != nil {
		return mtypes.Order{}, err
	}

	return order, nil
}

// CreateBid creates a bid for a order with given orderID, price for bid and provider
func (k Keeper) CreateBid(ctx sdk.Context, id mv1.BidID, price sdk.DecCoin, roffer mtypes.ResourcesOffer) (mtypes.Bid, error) {
	store := ctx.KVStore(k.skey)

	if key := k.findBid(ctx, id); len(key) > 0 {
		return mtypes.Bid{}, mv1.ErrBidExists
	}

	bid := mtypes.Bid{
		ID:             id,
		State:          mtypes.BidOpen,
		Price:          price,
		CreatedAt:      ctx.BlockHeight(),
		ResourcesOffer: roffer,
	}

	data := k.cdc.MustMarshal(&bid)

	key := keys.MustBidKey(keys.BidStateToPrefix(bid.State), id)
	revKey := keys.MustBidStateRevereKey(bid.State, id)

	store.Set(key, data)

	if len(revKey) > 0 {
		store.Set(revKey, data)
	}

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventBidCreated{
			ID:    bid.ID,
			Price: price,
		},
	)
	if err != nil {
		return mtypes.Bid{}, err
	}

	return bid, nil
}

// CreateLease creates lease for bid with given bidID.
// Should only be called by the EndBlock handler or unit tests.
func (k Keeper) CreateLease(ctx sdk.Context, bid mtypes.Bid) error {
	store := ctx.KVStore(k.skey)

	lease := mv1.Lease{
		ID:        mv1.LeaseID(bid.ID),
		State:     mv1.LeaseActive,
		Price:     bid.Price,
		CreatedAt: ctx.BlockHeight(),
	}

	data := k.cdc.MustMarshal(&lease)

	// create (active) lease in store
	key := keys.MustLeaseKey(keys.LeaseStateToPrefix(lease.State), lease.ID)
	revKey := keys.MustLeaseStateReverseKey(lease.State, lease.ID)

	store.Set(key, data)
	if len(revKey) > 0 {
		store.Set(revKey, data)
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
func (k Keeper) OnOrderMatched(ctx sdk.Context, order mtypes.Order) {
	currState := order.State
	order.State = mtypes.OrderActive
	k.updateOrder(ctx, order, currState)
}

// OnBidMatched updates bid state to matched
func (k Keeper) OnBidMatched(ctx sdk.Context, bid mtypes.Bid) {
	currState := bid.State
	bid.State = mtypes.BidActive
	k.updateBid(ctx, bid, currState)
}

// OnBidLost updates bid state to bid lost
func (k Keeper) OnBidLost(ctx sdk.Context, bid mtypes.Bid) {
	currState := bid.State
	bid.State = mtypes.BidLost
	k.updateBid(ctx, bid, currState)
}

// OnBidClosed updates bid state to closed
func (k Keeper) OnBidClosed(ctx sdk.Context, bid mtypes.Bid) error {
	switch bid.State {
	case mtypes.BidClosed, mtypes.BidLost:
		return nil
	}

	currState := bid.State
	bid.State = mtypes.BidClosed
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
func (k Keeper) OnOrderClosed(ctx sdk.Context, order mtypes.Order) error {
	if order.State == mtypes.OrderClosed {
		return nil
	}

	currState := order.State

	order.State = mtypes.OrderClosed

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

	currState := lease.State

	lease.State = state
	lease.ClosedOn = ctx.BlockHeight()
	lease.Reason = reason

	store := ctx.KVStore(k.skey)

	key := keys.MustLeaseKey(keys.LeaseStateToPrefix(currState), lease.ID)
	revKey := keys.MustLeaseStateReverseKey(currState, lease.ID)

	store.Delete(key)
	if len(revKey) > 0 {
		store.Delete(revKey)
	}

	key = keys.MustLeaseKey(keys.LeaseStateToPrefix(lease.State), lease.ID)
	store.Set(key, k.cdc.MustMarshal(&lease))

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

	processClose := func(ctx sdk.Context, bid mtypes.Bid) error {
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
	k.WithOrdersForGroup(ctx, id, mtypes.OrderActive, func(order mtypes.Order) bool {
		err = k.OnOrderClosed(ctx, order)
		if err != nil {
			return true
		}

		k.WithBidsForOrder(ctx, order.ID, mtypes.BidOpen, func(bid mtypes.Bid) bool {
			err = processClose(ctx, bid)
			return err != nil
		})

		if err != nil {
			return true
		}

		k.WithBidsForOrder(ctx, order.ID, mtypes.BidActive, func(bid mtypes.Bid) bool {
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

func (k Keeper) findOrder(ctx sdk.Context, id mv1.OrderID) []byte {
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
func (k Keeper) GetOrder(ctx sdk.Context, id mv1.OrderID) (mtypes.Order, bool) {
	key := k.findOrder(ctx, id)

	if len(key) == 0 {
		return mtypes.Order{}, false
	}

	store := ctx.KVStore(k.skey)

	buf := store.Get(key)

	var val mtypes.Order
	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

func (k Keeper) findBid(ctx sdk.Context, id mv1.BidID) []byte {
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
func (k Keeper) GetBid(ctx sdk.Context, id mv1.BidID) (mtypes.Bid, bool) {
	store := ctx.KVStore(k.skey)

	key := k.findBid(ctx, id)

	if len(key) == 0 {
		return mtypes.Bid{}, false
	}

	buf := store.Get(key)

	var val mtypes.Bid
	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

func (k Keeper) findLease(ctx sdk.Context, id mv1.LeaseID) []byte {
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
func (k Keeper) GetLease(ctx sdk.Context, id mv1.LeaseID) (mv1.Lease, bool) {
	store := ctx.KVStore(k.skey)
	key := k.findLease(ctx, id)

	if len(key) == 0 {
		return mv1.Lease{}, false
	}

	buf := store.Get(key)

	var val mv1.Lease
	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

// LeaseForOrder returns lease for order with given ID and lease found status
func (k Keeper) LeaseForOrder(ctx sdk.Context, bs mtypes.Bid_State, oid mv1.OrderID) (mv1.Lease, bool) {
	var value mv1.Lease
	var found bool

	k.WithBidsForOrder(ctx, oid, bs, func(item mtypes.Bid) bool {
		value, found = k.GetLease(ctx, mv1.LeaseID(item.ID))
		return true
	})

	return value, found
}

// WithOrders iterates all orders in market
func (k Keeper) WithOrders(ctx sdk.Context, fn func(mtypes.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, keys.OrderPrefix)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val mtypes.Order
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithBids iterates all bids in market
func (k Keeper) WithBids(ctx sdk.Context, fn func(mtypes.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, keys.BidPrefix)

	defer func() {
		_ = iter.Close()
	}()

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val mtypes.Bid
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithLeases iterates all leases in market
func (k Keeper) WithLeases(ctx sdk.Context, fn func(mv1.Lease) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, keys.LeasePrefix)

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
func (k Keeper) WithOrdersForGroup(ctx sdk.Context, id dtypes.GroupID, state mtypes.Order_State, fn func(mtypes.Order) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, keys.OrdersForGroupPrefix(keys.OrderStateToPrefix(state), id))

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val mtypes.Order
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithBidsForOrder iterates all bids of an order in market with given OrderID
func (k Keeper) WithBidsForOrder(ctx sdk.Context, id mv1.OrderID, state mtypes.Bid_State, fn func(mtypes.Bid) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateToPrefix(state), id))

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val mtypes.Bid
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k Keeper) BidCountForOrder(ctx sdk.Context, id mv1.OrderID) uint32 {
	store := ctx.KVStore(k.skey)
	oiter := storetypes.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateOpenPrefix, id))
	aiter := storetypes.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateActivePrefix, id))
	citer := storetypes.KVStorePrefixIterator(store, keys.BidsForOrderPrefix(keys.BidStateClosedPrefix, id))

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

func (k Keeper) updateOrder(ctx sdk.Context, order mtypes.Order, currState mtypes.Order_State) {
	store := ctx.KVStore(k.skey)

	switch currState {
	case mtypes.OrderOpen:
	case mtypes.OrderActive:
	default:
		panic(fmt.Sprintf("unexpected current state of the order: %d", currState))
	}

	key := keys.MustOrderKey(keys.OrderStateToPrefix(currState), order.ID)
	store.Delete(key)

	switch order.State {
	case mtypes.OrderActive:
	case mtypes.OrderClosed:
	default:
		panic(fmt.Sprintf("unexpected new state of the order: %d", order.State))
	}

	data := k.cdc.MustMarshal(&order)

	key = keys.MustOrderKey(keys.OrderStateToPrefix(order.State), order.ID)
	store.Set(key, data)
}

func (k Keeper) updateBid(ctx sdk.Context, bid mtypes.Bid, currState mtypes.Bid_State) {
	store := ctx.KVStore(k.skey)

	switch currState {
	case mtypes.BidOpen:
	case mtypes.BidActive:
	default:
		panic(fmt.Sprintf("unexpected current state of the bid: %d", currState))
	}

	key := keys.MustBidKey(keys.BidStateToPrefix(currState), bid.ID)
	revKey := keys.MustBidStateRevereKey(currState, bid.ID)
	store.Delete(key)
	if revKey != nil {
		store.Delete(revKey)
	}

	switch bid.State {
	case mtypes.BidActive:
	case mtypes.BidLost:
	case mtypes.BidClosed:
	default:
		panic(fmt.Sprintf("unexpected new state of the bid: %d", bid.State))
	}

	data := k.cdc.MustMarshal(&bid)

	key = keys.MustBidKey(keys.BidStateToPrefix(bid.State), bid.ID)
	revKey = keys.MustBidStateRevereKey(bid.State, bid.ID)

	store.Set(key, data)
	if len(revKey) > 0 {
		store.Set(revKey, data)
	}
}
