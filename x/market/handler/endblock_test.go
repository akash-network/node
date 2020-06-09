package handler

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	fuzz "github.com/google/gofuzz"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/market/types"
)

type winnerTest struct {
	desc      string
	bids      []types.Bid
	expWinner *types.Bid
	expErr    error
}

func (w *winnerTest) testFunc(t *testing.T) {
	winner, err := pickBidWinner(w.bids)
	if !errors.Is(err, w.expErr) {
		t.Errorf("returned err: %v does not match %v", err, w.expErr)
	}
	if w.expWinner != nil && !winner.Equals(w.expWinner.ID()) {
		t.Errorf("unexpected winner: %#v\n%q : %v", winner, types.BidIDString(winner.BidID), winner.Price)
		t.Logf("winner: %+v coin: %s", winner.ID(), winner.Price.String())
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
			expErr:    errNoBids,
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
func absZeroLimit(x int) int {
	limit := 150
	r := 0
	if x < 0 {
		r = -x
	} else {
		r = x
	}

	if r > limit {
		r %= limit
	}

	if r == 0 {
		r = 1
	}

	return r
}

type testDist struct {
	desc      string
	BidNum    int
	Rounds    int
	expErr    error
	expUneven bool
}

func (td *testDist) testFunc(t *testing.T) {
	originOID := testutil.OrderID(t)
	td.BidNum = absZeroLimit(td.BidNum)
	td.Rounds = absZeroLimit(td.Rounds)

	t.Logf("bids: %d rounds: %d", td.BidNum, td.Rounds)
	if (float64(td.BidNum) / float64(td.Rounds)) > 0.2 {
		t.Logf("configuring uneven distribution expected")
		td.expUneven = true
	}

	distributionSpread := make(map[int]int, td.BidNum)
	for i := 0; i < td.Rounds; i++ {
		bIndex := make(map[string]int)
		bids := make([]types.Bid, 0)
		// generate N bids all with the same bidding amount
		for j := 0; j < td.BidNum; j++ {
			b := createBid(t, originOID, 5)
			bIndex[b.Provider.String()] = j
			bids = append(bids, b)
		}

		winner, err := pickBidWinner(bids)
		if !errors.Is(err, td.expErr) {
			t.Errorf("returned err: %v does not match %v", err, td.expErr)
		}
		// Check provider
		slot := bIndex[winner.Provider.String()]
		distributionSpread[slot]++
	}

	// calculate a reasonable low expectation of wins given the number of
	// test rounds and number of bidders
	r := float64(td.Rounds/td.BidNum) * 0.3

	for i := 0; i < td.BidNum; i++ {

		t.Logf("[%d] winner distribution %v (%v expected)", i, distributionSpread[i], r)

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
			BidNum: 1,
			Rounds: 50,
			expErr: nil,
		},
		{
			desc:   "five winners",
			BidNum: 5,
			Rounds: 150,
			expErr: nil,
		},
		{
			desc:   "ten winners",
			BidNum: 10,
			Rounds: 300,
			expErr: nil,
		},
		{
			desc:      "too many bidders",
			BidNum:    100,
			Rounds:    100,
			expUneven: true,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), test.testFunc)
	}

	for i := 0; i < 50; i++ {
		t.Run(fmt.Sprintf("fuzzer-%d", i), func(t *testing.T) {
			f := fuzz.New()
			test := &testDist{}
			f.Fuzz(test)
			t.Logf("fuzzed testDist: %+v", test)
			test.testFunc(t)
		})
	}
}

func createBid(t *testing.T, oid types.OrderID, bid int) types.Bid {
	t.Helper()
	akashDenom := "akash"
	sdkInt := int64(bid)
	b := types.Bid{
		BidID: types.MakeBidID(oid, testutil.AccAddress(t)),
		State: types.BidOpen,
		Price: sdk.NewCoin(akashDenom, sdk.NewInt(sdkInt)),
	}
	return b
}
