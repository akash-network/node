package handler_test

import (
	"strconv"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/market/handler"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/pkg/errors"
)

type winnerTest struct {
	desc      string
	bids      []types.Bid
	expWinner *types.Bid
	expErr    error
}

func (w *winnerTest) testFunc(t *testing.T) {
	winner, err := handler.PickBidWinner(w.bids)
	if !errors.Is(err, w.expErr) {
		t.Errorf("returned err: '%v' does not match '%v'", err, w.expErr)
	}
	if w.expWinner != nil && !winner.ID().Equals(w.expWinner.ID()) {
		t.Errorf("unexpected winner: %#v\n%s : %v", winner, winner.BidID, winner.Price)
	}
}

func TestBidWinner(t *testing.T) {
	originOID := testutil.OrderID(t)

	bid0 := createBid(t, originOID, 3)
	bid1 := createBid(t, originOID, 5)
	bid2 := createBid(t, originOID, 5)
	bid3 := createBid(t, originOID, 4)
	bid4 := createBid(t, originOID, 5)
	bid5 := createBid(t, originOID, 5)
	bid6 := createBid(t, originOID, 5)
	bid7 := createBid(t, originOID, 8)
	bid8 := createBid(t, originOID, 6)
	bid9 := createBid(t, originOID, 5)

	var winnerTests = []winnerTest{
		{
			desc:      "no bids",
			bids:      []types.Bid{},
			expWinner: nil,
			expErr:    handler.ErrNoBids,
		},
		{
			desc:      "single bid",
			bids:      []types.Bid{bid9},
			expWinner: &bid9,
		},
		{
			desc:      "two bids",
			bids:      []types.Bid{bid7, bid4},
			expWinner: &bid4,
		},
		{
			desc:      "two matching bids",
			bids:      []types.Bid{bid9, bid7, bid4},
			expWinner: &bid4,
		},
		{
			desc:      "all the same bid values",
			bids:      []types.Bid{bid9, bid9, bid9, bid9, bid9},
			expWinner: &bid9,
		},
		{
			desc:      "two of the same",
			bids:      []types.Bid{bid9, bid3},
			expWinner: &bid3,
		},
		{
			desc:      "multiple of the same, but one lowest value",
			bids:      []types.Bid{bid1, bid0, bid9, bid2, bid3, bid4},
			expWinner: &bid0,
		},
		{
			desc:      "confirm last bid in list is picked",
			bids:      []types.Bid{bid1, bid9, bid2, bid3, bid4, bid0},
			expWinner: &bid0,
		},
		{
			desc:      "two matching low bids",
			bids:      []types.Bid{bid5, bid6, bid7, bid8},
			expWinner: &bid6,
		},
	}

	for _, test := range winnerTests {
		t.Run(test.desc, test.testFunc)
	}
}

type testDist struct {
	desc      string
	bidNum    int
	rounds    int
	expErr    error
	expUneven bool
}

func (td *testDist) testFunc(t *testing.T) {
	t.Logf("testing function with description: %s", td.desc)
	originOID := testutil.OrderID(t)

	distributionSpread := make(map[int]int, td.bidNum)
	for i := 0; i < td.rounds; i++ {
		bIndex := make(map[string]int, td.bidNum)
		bids := make([]types.Bid, 0, td.bidNum)
		// generate N bids all with the same bidding amount
		for j := 0; j < td.bidNum; j++ {
			b := createBid(t, originOID, 5)
			bIndex[b.ID().Provider.String()] = j
			bids = append(bids, b)
		}

		winner, err := handler.PickBidWinner(bids)
		if !errors.Is(err, td.expErr) {
			t.Errorf("returned err: %v does not match %v", err, td.expErr)
		}
		// Check provider
		slot := bIndex[winner.ID().Provider.String()]
		distributionSpread[slot]++
	}

	// calculate a reasonable low expectation of wins given the number of
	// test rounds and number of bidders
	r := float64(td.rounds/td.bidNum) * 0.5

	for i := 0; i < td.bidNum; i++ {
		v := float64(distributionSpread[i])

		if v >= r {
			continue
		}

		if td.expUneven {
			continue
		}

		t.Errorf("[%d] expectation failed (%v < %v)", i, v, r)
	}

}

func TestWinningDistribution(t *testing.T) {
	tests := []testDist{
		{
			desc:   "one bidder",
			bidNum: 1,
			rounds: 50,
			expErr: nil,
		},
		{
			desc:   "five winners",
			bidNum: 5,
			rounds: 150,
			expErr: nil,
		},
		{
			desc:   "ten winners",
			bidNum: 10,
			rounds: 300,
			expErr: nil,
		},
		{
			desc:      "too many bidders",
			bidNum:    100,
			rounds:    100,
			expUneven: true,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), test.testFunc)
	}
}

func createBid(t *testing.T, oid types.OrderID, bid int) types.Bid {
	t.Helper()
	sdkInt := int64(bid)
	b := types.Bid{
		BidID: types.MakeBidID(oid, testutil.AccAddress(t)),
		State: types.BidOpen,
		Price: sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(sdkInt)),
	}
	return b
}
