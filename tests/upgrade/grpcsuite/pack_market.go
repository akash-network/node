package grpcsuite

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/sdkutil"
)

type marketPack struct{}

func (marketPack) Name() string { return "market" }

func (marketPack) Available(d *Discovery) bool {
	return d.HasModule("akash.market.v1beta5")
}

func (mp marketPack) Run(s *Suite) {
	q := mvbeta.NewQueryClient(s.Conn)

	// Prerequisites created by the deployment and provider packs.
	depID, ok := s.World.Get(wDeploymentID).(dv1.DeploymentID)
	require.True(s.T, ok, "market pack needs the deployment pack's open deployment")
	require.NotEmpty(s.T, s.World.Get(wProviderSigner), "market pack needs the provider pack")
	providerAddr := s.Addr("provider")

	// --- order A: full bid -> lease -> withdraw -> close flow ---
	orderA := s.findOrder(q, depID.Owner, depID.DSeq)
	bidA := s.newBid(q, orderA, providerAddr.String())
	s.BroadcastOK("provider", bidA)
	s.logf("created bid on order %s/%d/%d/%d", orderA.ID.Owner, orderA.ID.DSeq, orderA.ID.GSeq, orderA.ID.OSeq)

	// Order queries (order is still open until a lease is created): assert content.
	orderResp, err := q.Order(s.Ctx, &mvbeta.QueryOrderRequest{ID: orderA.ID})
	require.NoError(s.T, err, "Order")
	require.Equal(s.T, orderA.ID, orderResp.Order.ID, "queried order id should match")
	require.Equal(s.T, mvbeta.OrderOpen, orderResp.Order.State, "order should be open before a lease")
	require.NotEmpty(s.T, orderResp.Order.Spec.Resources, "order spec should carry resources")

	orders, err := q.Orders(s.Ctx, &mvbeta.QueryOrdersRequest{
		Filters:    mvbeta.OrderFilters{Owner: depID.Owner, DSeq: depID.DSeq, State: mvbeta.OrderOpen.String()},
		Pagination: &sdkquery.PageRequest{Limit: 50},
	})
	require.NoError(s.T, err, "Orders")
	require.NotEmpty(s.T, orders.Orders, "owner should have an open order")
	require.True(s.T, containsOrder(orders.Orders, orderA.ID), "filtered orders should include order A")
	for _, o := range orders.Orders {
		require.Equal(s.T, depID.Owner, o.ID.Owner, "owner filter must only return owner's orders")
	}
	mp.assertOrdersPaginate(s, q, depID.Owner)

	// Bid queries: assert content.
	bidResp, err := q.Bid(s.Ctx, &mvbeta.QueryBidRequest{ID: bidA.ID})
	require.NoError(s.T, err, "Bid")
	require.Equal(s.T, bidA.ID, bidResp.Bid.ID, "queried bid id should match")
	require.Equal(s.T, bidA.Price, bidResp.Bid.Price, "queried bid price should match")
	require.NotEmpty(s.T, bidResp.EscrowAccount.ID.XID, "bid should have an escrow account")

	bids, err := q.Bids(s.Ctx, &mvbeta.QueryBidsRequest{
		Filters:    mvbeta.BidFilters{Owner: depID.Owner, DSeq: depID.DSeq, Provider: providerAddr.String()},
		Pagination: &sdkquery.PageRequest{Limit: 50},
	})
	require.NoError(s.T, err, "Bids")
	require.NotEmpty(s.T, bids.Bids, "owner+provider filter should return the bid")
	for _, b := range bids.Bids {
		require.Equal(s.T, providerAddr.String(), b.Bid.ID.Provider, "provider filter must only return that provider's bids")
	}

	// Tenant accepts the bid -> creates a lease.
	s.BroadcastOK(s.TenantSigner(), &mvbeta.MsgCreateLease{BidID: bidA.ID})
	leaseA := mv1.LeaseID{
		Owner: bidA.ID.Owner, DSeq: bidA.ID.DSeq, GSeq: bidA.ID.GSeq,
		OSeq: bidA.ID.OSeq, Provider: bidA.ID.Provider,
	}

	// Lease queries: assert content.
	leaseResp, err := q.Lease(s.Ctx, &mvbeta.QueryLeaseRequest{ID: leaseA})
	require.NoError(s.T, err, "Lease")
	require.Equal(s.T, leaseA, leaseResp.Lease.ID, "queried lease id should match")
	require.Equal(s.T, mv1.LeaseActive, leaseResp.Lease.State, "new lease should be active")

	leases, err := q.Leases(s.Ctx, &mvbeta.QueryLeasesRequest{
		Filters:    mv1.LeaseFilters{Owner: depID.Owner, DSeq: depID.DSeq, State: mv1.LeaseActive.String()},
		Pagination: &sdkquery.PageRequest{Limit: 50},
	})
	require.NoError(s.T, err, "Leases")
	require.NotEmpty(s.T, leases.Leases, "owner should have an active lease")
	for _, l := range leases.Leases {
		require.Equal(s.T, depID.Owner, l.Lease.ID.Owner, "owner filter must only return owner's leases")
	}

	// Params query.
	pResp, err := q.Params(s.Ctx, &mvbeta.QueryParamsRequest{})
	require.NoError(s.T, err, "market Params")
	require.False(s.T, pResp.Params.BidMinDeposit.IsNil(), "market params should expose a bid min deposit")

	// Let the lease's escrow payment settle/accrue before withdrawing.
	s.WaitBlocks(2)
	s.BroadcastOK("provider", &mvbeta.MsgWithdrawLease{ID: leaseA})

	// MsgLeaseStartReclaim requires a reclamation-enabled lease; this lease has none,
	// so exercise it and assert the expected rejection.
	_, err = s.TX.Broadcast("provider", &mvbeta.MsgLeaseStartReclaim{ID: leaseA})
	require.Error(s.T, err, "LeaseStartReclaim should be rejected for a non-reclamation lease")

	// Close the lease (tenant). Reason must be in the lease-closed-reason range.
	s.BroadcastOK(s.TenantSigner(), &mvbeta.MsgCloseLease{ID: leaseA, Reason: mv1.LeaseClosedReasonDecommissioned})

	// --- order B: bid then close the bid (un-leased) ---
	depB := createDeploymentFromSDL(s, s.TenantSigner())
	orderB := s.findOrder(q, depB.Owner, depB.DSeq)
	bidB := s.newBid(q, orderB, providerAddr.String())
	s.BroadcastOK("provider", bidB)
	s.BroadcastOK("provider", &mvbeta.MsgCloseBid{ID: bidB.ID, Reason: mv1.LeaseClosedReasonDecommissioned})
	s.logf("market lifecycle complete (bid/lease/withdraw/closeLease/closeBid)")

	// Negative / edge cases.
	mp.marketNegatives(s, q, providerAddr.String())
}

// assertOrdersPaginate verifies the pagination Limit caps the returned orders.
func (marketPack) assertOrdersPaginate(s *Suite, q mvbeta.QueryClient, owner string) {
	// pagination is verified against the global order set (limit must be honored).
	resp, err := q.Orders(s.Ctx, &mvbeta.QueryOrdersRequest{
		Filters:    mvbeta.OrderFilters{Owner: owner},
		Pagination: &sdkquery.PageRequest{Limit: 1},
	})
	require.NoError(s.T, err, "Orders pagination")
	require.NotNil(s.T, resp, "Orders pagination response")
	require.LessOrEqual(s.T, len(resp.Orders), 1, "Orders pagination limit must be honored")
}

// marketNegatives exercises edge cases that must be rejected, against bogus or
// out-of-bounds inputs so the live order/lease state is untouched.
func (marketPack) marketNegatives(s *Suite, q mvbeta.QueryClient, providerAddr string) {
	// Fresh open order to derive a structurally-valid bid for the price-too-high case.
	depN := createDeploymentFromSDL(s, s.TenantSigner())
	orderN := s.findOrder(q, depN.Owner, depN.DSeq)
	deposit := depositv1.Deposit{Amount: s.marketBidDeposit(q), Sources: depositv1.Sources{depositv1.SourceBalance}}
	offer := resourcesOfferFrom(orderN.Spec)

	// 1. Bid on a non-existent order (bogus dseq).
	s.BroadcastExpectErr("provider", &mvbeta.MsgCreateBid{
		ID:             mv1.BidID{Owner: orderN.ID.Owner, DSeq: orderN.ID.DSeq + 9_000_000, GSeq: 1, OSeq: 1, Provider: providerAddr},
		Price:          orderN.Spec.Price(),
		Deposit:        deposit,
		ResourcesOffer: offer,
	})

	// 2. Bid priced far above the order's max acceptable price.
	max := orderN.Spec.Price()
	tooHigh := sdk.NewDecCoinFromDec(max.Denom, max.Amount.MulInt64(1_000_000))
	s.BroadcastExpectErr("provider", &mvbeta.MsgCreateBid{
		ID:             mv1.BidID{Owner: orderN.ID.Owner, DSeq: orderN.ID.DSeq, GSeq: orderN.ID.GSeq, OSeq: orderN.ID.OSeq, Provider: providerAddr},
		Price:          tooHigh,
		Deposit:        deposit,
		ResourcesOffer: offer,
	})

	// 3. Create a lease referencing a non-existent bid.
	s.BroadcastExpectErr(s.TenantSigner(), &mvbeta.MsgCreateLease{
		BidID: mv1.BidID{Owner: orderN.ID.Owner, DSeq: orderN.ID.DSeq + 8_000_000, GSeq: 1, OSeq: 1, Provider: providerAddr},
	})

	// 4. Close a lease with an invalid (out-of-range) reason.
	s.BroadcastExpectErr(s.TenantSigner(), &mvbeta.MsgCloseLease{
		ID:     mv1.LeaseID{Owner: orderN.ID.Owner, DSeq: orderN.ID.DSeq, GSeq: 1, OSeq: 1, Provider: providerAddr},
		Reason: 0,
	})

	// 5. Withdraw from a non-existent lease.
	s.BroadcastExpectErr("provider", &mvbeta.MsgWithdrawLease{
		ID: mv1.LeaseID{Owner: orderN.ID.Owner, DSeq: orderN.ID.DSeq + 7_000_000, GSeq: 1, OSeq: 1, Provider: providerAddr},
	})
	s.logf("market negatives complete (missing order/bid/lease + over-max price + invalid close reason rejected)")
}

func containsOrder(orders mvbeta.Orders, id mv1.OrderID) bool {
	for _, o := range orders {
		if o.ID == id {
			return true
		}
	}
	return false
}

// findOrder polls for an open order belonging to owner/dseq (orders are created in
// the EndBlocker of the block that includes the deployment).
func (s *Suite) findOrder(q mvbeta.QueryClient, owner string, dseq uint64) mvbeta.Order {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, 30*time.Second)
	defer cancel()
	for {
		resp, err := q.Orders(ctx, &mvbeta.QueryOrdersRequest{
			Filters:    mvbeta.OrderFilters{Owner: owner, DSeq: dseq},
			Pagination: &sdkquery.PageRequest{Limit: 10},
		})
		if err == nil && len(resp.Orders) > 0 {
			return resp.Orders[0]
		}
		select {
		case <-ctx.Done():
			s.T.Fatalf("no order found for %s/%d: %v", owner, dseq, err)
		case <-time.After(time.Second):
		}
	}
}

// newBid builds a MsgCreateBid that matches the order: it offers each of the order's
// resources and bids at the order's max price.
func (s *Suite) newBid(q mvbeta.QueryClient, order mvbeta.Order, provider string) *mvbeta.MsgCreateBid {
	s.T.Helper()
	return &mvbeta.MsgCreateBid{
		ID: mv1.BidID{
			Owner: order.ID.Owner, DSeq: order.ID.DSeq, GSeq: order.ID.GSeq,
			OSeq: order.ID.OSeq, Provider: provider,
		},
		Price:          order.Spec.Price(),
		Deposit:        depositv1.Deposit{Amount: s.marketBidDeposit(q), Sources: depositv1.Sources{depositv1.SourceBalance}},
		ResourcesOffer: resourcesOfferFrom(order.Spec),
	}
}

func resourcesOfferFrom(spec dvbeta.GroupSpec) mvbeta.ResourcesOffer {
	offer := make(mvbeta.ResourcesOffer, 0, len(spec.Resources))
	for _, ru := range spec.Resources {
		offer = append(offer, mvbeta.ResourceOffer{Resources: ru.Resources, Count: ru.Count})
	}
	return offer
}

func (s *Suite) marketBidDeposit(q mvbeta.QueryClient) sdk.Coin {
	s.T.Helper()
	resp, err := q.Params(s.Ctx, &mvbeta.QueryParamsRequest{})
	require.NoError(s.T, err, "market Params")
	// Prefer the uakt min deposit — providers are funded in uakt.
	for _, c := range resp.Params.BidMinDeposits {
		if c.Denom == sdkutil.DenomUakt && !c.IsZero() {
			return c
		}
	}
	if resp.Params.BidMinDeposit.Denom == sdkutil.DenomUakt && !resp.Params.BidMinDeposit.IsZero() {
		return resp.Params.BidMinDeposit
	}
	s.T.Fatal("market params have no uakt bid min deposit")
	return sdk.Coin{}
}
