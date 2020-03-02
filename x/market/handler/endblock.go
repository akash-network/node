package handler

import (
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/types"
)

// OnEndBlock transfer funds for active leases and update order states
// Executed at the end of block
func OnEndBlock(ctx sdk.Context, keepers Keepers) error {
	if err := transferFundsForActiveLeases(ctx, keepers); err != nil {
		return err
	}
	if err := matchOrders(ctx, keepers); err != nil {
		return err
	}
	return nil
}

func transferFundsForActiveLeases(ctx sdk.Context, keepers Keepers) error {

	// for all active leases, transfer funds
	count := 0
	keepers.Market.WithLeases(ctx, func(lease types.Lease) bool {

		if lease.State != types.LeaseActive {
			return false
		}

		amt := sdk.NewCoins(lease.Price)

		if !keepers.Bank.HasCoins(ctx, lease.Owner, amt) {
			keepers.Deployment.OnLeaseInsufficientFunds(ctx, lease.GroupID())
			keepers.Market.OnInsufficientFunds(ctx, lease)
			return false
		}

		err := keepers.Bank.SendCoins(ctx, lease.Owner, lease.Provider, amt)

		if err != nil {
			ctx.Logger().Error("error transferring funds", "err", err)
			// TODO: cancel order, lease.
			// TODO: notify deployment module.
			return false
		}

		count++
		return false
	})

	ctx.Logger().Info("processed active leases", "count", count)

	return nil
}

func matchOrders(ctx sdk.Context, keepers Keepers) error {

	// match unmatched orders.
	keepers.Market.WithOrders(ctx, func(order types.Order) bool {
		if err := order.ValidateCanMatch(ctx.BlockHeight()); err != nil {
			return false
		}

		var bids []types.Bid

		keepers.Market.WithBidsForOrder(ctx, order.ID(), func(bid types.Bid) bool {
			if bid.State != types.BidOpen {
				return false
			}
			bids = append(bids, bid)
			return false
		})

		// no open bids
		if len(bids) == 0 {
			return false
		}

		// open bids; match by lowest price
		sort.Slice(bids, func(i, j int) bool {
			// TODO handle same price
			return bids[i].Price.IsLT(bids[j].Price)
		})

		winner := bids[0]

		// create lease
		keepers.Market.CreateLease(ctx, winner)

		// set winning bid state to matched
		keepers.Market.OnBidMatched(ctx, winner)

		// set losing bids to state lost
		for _, bid := range bids[1:] {
			keepers.Market.OnBidLost(ctx, bid)
		}

		// set order state to matched
		keepers.Market.OnOrderMatched(ctx, order)

		// notify group of match
		keepers.Deployment.OnLeaseCreated(ctx, order.GroupID())

		return false
	})
	return nil
}
