package keeper

import (
	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

// OrderIndexes defines the secondary indexes for the order IndexedMap
type OrderIndexes struct {
	// State indexes orders by their state (Open, Active, Closed)
	State *indexes.Multi[int32, keys.OrderPrimaryKey, mvbeta.Order]

	// GroupState indexes orders by (owner, dseq, gseq, state) for WithOrdersForGroup queries
	GroupState *indexes.Multi[collections.Pair[keys.GroupPartKey, int32], keys.OrderPrimaryKey, mvbeta.Order]
}

// BidIndexes defines the secondary indexes for the bid IndexedMap
type BidIndexes struct {
	// State indexes bids by their state (Open, Active, Lost, Closed)
	State *indexes.Multi[int32, keys.BidPrimaryKey, mvbeta.Bid]

	// Provider indexes bids by provider address (covers all states)
	Provider *indexes.Multi[string, keys.BidPrimaryKey, mvbeta.Bid]

	// OrderState indexes bids by (owner, dseq, gseq, oseq, state) for WithBidsForOrder queries
	OrderState *indexes.Multi[collections.Pair[keys.OrderPrimaryKey, int32], keys.BidPrimaryKey, mvbeta.Bid]
}

// LeaseIndexes defines the secondary indexes for the lease IndexedMap
type LeaseIndexes struct {
	// State indexes leases by their state (Active, InsufficientFunds, Closed)
	State *indexes.Multi[int32, keys.LeasePrimaryKey, mv1.Lease]

	// Provider indexes leases by provider address (covers all states, replaces old reverse keys)
	Provider *indexes.Multi[string, keys.LeasePrimaryKey, mv1.Lease]
}

func (b BidIndexes) IndexesList() []collections.Index[keys.BidPrimaryKey, mvbeta.Bid] {
	return []collections.Index[keys.BidPrimaryKey, mvbeta.Bid]{
		b.State,
		b.Provider,
		b.OrderState,
	}
}

func (l LeaseIndexes) IndexesList() []collections.Index[keys.LeasePrimaryKey, mv1.Lease] {
	return []collections.Index[keys.LeasePrimaryKey, mv1.Lease]{
		l.State,
		l.Provider,
	}
}

func (o OrderIndexes) IndexesList() []collections.Index[keys.OrderPrimaryKey, mvbeta.Order] {
	return []collections.Index[keys.OrderPrimaryKey, mvbeta.Order]{
		o.State,
		o.GroupState,
	}
}

// NewOrderIndexes creates all secondary indexes for the order IndexedMap
func NewOrderIndexes(sb *collections.SchemaBuilder) OrderIndexes {
	return OrderIndexes{
		State: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.OrderIndexStatePrefix),
			"orders_by_state",
			collections.Int32Key,
			keys.OrderPrimaryKeyCodec,
			func(_ keys.OrderPrimaryKey, order mvbeta.Order) (int32, error) {
				return int32(order.State), nil
			},
		),
		GroupState: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.OrderIndexGroupStatePrefix),
			"orders_by_group_state",
			collections.PairKeyCodec(
				collections.TripleKeyCodec(
					collections.StringKey,
					collections.Uint64Key,
					collections.Uint32Key,
				),
				collections.Int32Key,
			),
			keys.OrderPrimaryKeyCodec,
			func(_ keys.OrderPrimaryKey, order mvbeta.Order) (collections.Pair[keys.GroupPartKey, int32], error) {
				groupPart := collections.Join3(order.ID.Owner, order.ID.DSeq, order.ID.GSeq)
				return collections.Join(groupPart, int32(order.State)), nil
			},
		),
	}
}

// NewBidIndexes creates all secondary indexes for the bid IndexedMap
func NewBidIndexes(sb *collections.SchemaBuilder) BidIndexes {
	return BidIndexes{
		State: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.BidIndexStatePrefix),
			"bids_by_state",
			collections.Int32Key,
			keys.BidPrimaryKeyCodec,
			func(_ keys.BidPrimaryKey, bid mvbeta.Bid) (int32, error) {
				return int32(bid.State), nil
			},
		),
		Provider: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.BidIndexProviderPrefix),
			"bids_by_provider",
			collections.StringKey,
			keys.BidPrimaryKeyCodec,
			func(_ keys.BidPrimaryKey, bid mvbeta.Bid) (string, error) {
				return bid.ID.Provider, nil
			},
		),
		OrderState: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.BidIndexOrderStatePrefix),
			"bids_by_order_state",
			collections.PairKeyCodec(
				collections.QuadKeyCodec(
					collections.StringKey,
					collections.Uint64Key,
					collections.Uint32Key,
					collections.Uint32Key,
				),
				collections.Int32Key,
			),
			keys.BidPrimaryKeyCodec,
			func(_ keys.BidPrimaryKey, bid mvbeta.Bid) (collections.Pair[keys.OrderPrimaryKey, int32], error) {
				orderPart := collections.Join4(bid.ID.Owner, bid.ID.DSeq, bid.ID.GSeq, bid.ID.OSeq)
				return collections.Join(orderPart, int32(bid.State)), nil
			},
		),
	}
}

// NewLeaseIndexes creates all secondary indexes for the lease IndexedMap
func NewLeaseIndexes(sb *collections.SchemaBuilder) LeaseIndexes {
	return LeaseIndexes{
		State: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.LeaseIndexStatePrefix),
			"leases_by_state",
			collections.Int32Key,
			keys.LeasePrimaryKeyCodec,
			func(_ keys.LeasePrimaryKey, lease mv1.Lease) (int32, error) {
				return int32(lease.State), nil
			},
		),
		Provider: indexes.NewMulti(
			sb,
			collections.NewPrefix(keys.LeaseIndexProviderPrefix),
			"leases_by_provider",
			collections.StringKey,
			keys.LeasePrimaryKeyCodec,
			func(_ keys.LeasePrimaryKey, lease mv1.Lease) (string, error) {
				return lease.ID.Provider, nil
			},
		),
	}
}
