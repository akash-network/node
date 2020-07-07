package handler

import (
	"hash/fnv"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/pkg/errors"
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
	keepers.Market.WithActiveLeases(ctx, func(lease types.Lease) bool {

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

var errNoBids error = errors.New("no bids to pick winner from")

func pickBidWinner(bids []types.Bid) (winner *types.Bid, err error) {
	// open bids; match by lowest price; sort bids by price
	sort.Slice(bids, func(i, j int) bool {
		// The BidID DSeq is pulled from the original OrderID.
		// So it can't be used to determine who bid first like a timestamp.
		return bids[i].Price.IsLT(bids[j].Price)
	})
	switch len(bids) {
	case 0:
		// This is a fatal case
		return nil, errNoBids
	case 1:
		return &bids[0], nil
	}

	if !bids[0].Price.IsEqual(bids[1].Price) {
		// Lowest bid(0) is unique, return the winner
		return &bids[0], nil
	}

	// There are equivalent bid prices; select winner with deterministic
	// random ordering based on given bids.
	// FNV hash provider addresses all of the bids
	h := fnv.New32a()
	bidIndex := 0
	_, err = h.Write(bids[bidIndex+1].Provider.Bytes())
	if err != nil {
		return nil, err
	}
	for ; bidIndex+1 < len(bids); bidIndex++ {
		if !bids[bidIndex].Price.IsEqual(bids[bidIndex+1].Price) {
			break
		}
		_, err := h.Write(bids[bidIndex+1].Provider.Bytes())
		if err != nil {
			return nil, err
		}
	}

	// Create a numeric hash from the Stringified Bid values
	n := int(h.Sum32()) % (bidIndex + 1) // Calculate the remainder to select index of equal bids
	return &bids[n], nil
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

		winner, err := pickBidWinner(bids)
		if err != nil {
			pErr := errors.Wrap(err, "picking bid winner returned unrecoverable error")
			panic(pErr.Error())
		}

		// create lease
		keepers.Market.CreateLease(ctx, *winner)

		// set winning bid state to matched
		keepers.Market.OnBidMatched(ctx, *winner)

		// set losing bids to state lost
		// Set all but winning bid to State: Lost
		for _, bid := range bids {
			if winner.Equals(bid.BidID) {
				continue // skip setting state to lost
			}
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
