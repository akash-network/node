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
)

type marketPack struct{}

func (marketPack) Name() string { return "market" }

func (marketPack) Available(d *Discovery) bool {
	return d.HasModule("akash.market.v1beta5")
}

func (marketPack) Run(s *Suite) {
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

	// Bid/order queries.
	_, err := q.Order(s.Ctx, &mvbeta.QueryOrderRequest{ID: orderA.ID})
	require.NoError(s.T, err, "Order")
	_, err = q.Orders(s.Ctx, &mvbeta.QueryOrdersRequest{Filters: mvbeta.OrderFilters{Owner: depID.Owner}, Pagination: &sdkquery.PageRequest{Limit: 50}})
	require.NoError(s.T, err, "Orders")
	_, err = q.Bid(s.Ctx, &mvbeta.QueryBidRequest{ID: bidA.ID})
	require.NoError(s.T, err, "Bid")
	_, err = q.Bids(s.Ctx, &mvbeta.QueryBidsRequest{Filters: mvbeta.BidFilters{Owner: depID.Owner}, Pagination: &sdkquery.PageRequest{Limit: 50}})
	require.NoError(s.T, err, "Bids")

	// Tenant accepts the bid -> creates a lease.
	s.BroadcastOK("tenant", &mvbeta.MsgCreateLease{BidID: bidA.ID})
	leaseA := mv1.LeaseID{
		Owner: bidA.ID.Owner, DSeq: bidA.ID.DSeq, GSeq: bidA.ID.GSeq,
		OSeq: bidA.ID.OSeq, Provider: bidA.ID.Provider,
	}

	// Lease queries.
	_, err = q.Lease(s.Ctx, &mvbeta.QueryLeaseRequest{ID: leaseA})
	require.NoError(s.T, err, "Lease")
	_, err = q.Leases(s.Ctx, &mvbeta.QueryLeasesRequest{Filters: mv1.LeaseFilters{Owner: depID.Owner}, Pagination: &sdkquery.PageRequest{Limit: 50}})
	require.NoError(s.T, err, "Leases")

	// Let the lease's escrow payment settle/accrue before withdrawing.
	s.WaitBlocks(2)
	s.BroadcastOK("provider", &mvbeta.MsgWithdrawLease{ID: leaseA})

	// MsgLeaseStartReclaim requires a reclamation-enabled lease; this lease has none,
	// so exercise it and assert the expected rejection.
	_, err = s.TX.Broadcast("provider", &mvbeta.MsgLeaseStartReclaim{ID: leaseA})
	require.Error(s.T, err, "LeaseStartReclaim should be rejected for a non-reclamation lease")

	// Close the lease (tenant). Reason must be in the lease-closed-reason range.
	s.BroadcastOK("tenant", &mvbeta.MsgCloseLease{ID: leaseA, Reason: mv1.LeaseClosedReasonDecommissioned})

	// --- order B: bid then close the bid (un-leased) ---
	depB := createDeploymentFromSDL(s, "tenant")
	orderB := s.findOrder(q, depB.Owner, depB.DSeq)
	bidB := s.newBid(q, orderB, providerAddr.String())
	s.BroadcastOK("provider", bidB)
	s.BroadcastOK("provider", &mvbeta.MsgCloseBid{ID: bidB.ID, Reason: mv1.LeaseClosedReasonDecommissioned})
	s.logf("market lifecycle complete (bid/lease/withdraw/closeLease/closeBid)")
}

// findOrder polls for an open order belonging to owner/dseq (orders are created in
// the EndBlocker of the block that includes the deployment).
func (s *Suite) findOrder(q mvbeta.QueryClient, owner string, dseq uint64) mvbeta.Order {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, 30*time.Second)
	defer cancel()
	for {
		resp, err := q.Orders(s.Ctx, &mvbeta.QueryOrdersRequest{
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
	if !resp.Params.BidMinDeposit.IsZero() {
		return resp.Params.BidMinDeposit
	}
	if len(resp.Params.BidMinDeposits) > 0 {
		return resp.Params.BidMinDeposits[0]
	}
	s.T.Fatal("market params have no bid min deposit")
	return sdk.Coin{}
}
