package handler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/testutil/state"
	"github.com/ovrclk/akash/x/market/handler"
	"github.com/ovrclk/akash/x/market/types"
)

func TestMatchOrders(t *testing.T) {
	codec := app.MakeCodec()
	suite := state.SetupTestSuite(t, codec)
	genOrders := make([]types.Order, 0)

	dID1 := testutil.DeploymentID(t)
	dID1.DSeq = uint64(suite.Context().BlockHeight())
	openGroups := testutil.DeploymentGroups(t, dID1, uint32(1))
	dID2 := testutil.DeploymentID(t)
	dID2.DSeq = uint64(suite.Context().BlockHeight())
	closedGroups := testutil.DeploymentGroups(t, dID2, uint32(2))

	t.Run("create open orders", func(t *testing.T) {
		for _, g := range openGroups {
			order, err := suite.MarketKeeper().CreateOrder(
				suite.Context(),
				g.ID(),
				g.GroupSpec,
			)
			require.NoError(t, err)
			genOrders = append(genOrders, order)

			// Create bids for orders
			_, err = suite.MarketKeeper().CreateBid(
				suite.Context(),
				order.ID(),
				testutil.AccAddress(t),
				order.Price(),
			)
			assert.NoError(t, err)

			// create a loosing bid
			bidSubtraction := testutil.AkashCoin(t, int64(2))
			_, err = suite.MarketKeeper().CreateBid(
				suite.Context(),
				order.ID(),
				testutil.AccAddress(t),
				order.Price().Sub(bidSubtraction),
			)
			assert.NoError(t, err)
		}
	})
	t.Run("create closed orders", func(t *testing.T) {
		for _, g := range closedGroups {
			order, err := suite.MarketKeeper().CreateOrder(
				suite.Context(),
				g.ID(),
				g.GroupSpec,
			)
			require.NoError(t, err)
			genOrders = append(genOrders, order)

			// close the order
			suite.MarketKeeper().OnOrderClosed(suite.Context(), order)
		}
	})

	t.Run("assert open orders", func(t *testing.T) {
		count := 0
		suite.MarketKeeper().WithOpenOrders(suite.Context(), func(o types.Order) bool {
			count++
			return false
		})
		assert.Len(t, openGroups, count)
	})

	t.Run("fail to match orders due to block height below bid threshold", func(t *testing.T) {
		k := handler.Keepers{
			Market:     suite.MarketKeeper(),
			Deployment: suite.DeploymentKeeper(),
			Provider:   suite.ProviderKeeper(),
			Bank:       suite.BankKeeper(),
		}
		err := handler.OnEndBlock(suite.Context(), k)
		require.NoError(t, err)
	})

	t.Run("assert still open orders", func(t *testing.T) {
		count := 0
		suite.MarketKeeper().WithOpenOrders(suite.Context(), func(o types.Order) bool {
			count++
			return false
		})
		assert.Equal(t, len(openGroups), count)
	})

	t.Run("match orders after setting block height", func(t *testing.T) {
		suite.SetBlockHeight(int64(100))
		k := handler.Keepers{
			Market:     suite.MarketKeeper(),
			Deployment: suite.DeploymentKeeper(),
			Provider:   suite.ProviderKeeper(),
			Bank:       suite.BankKeeper(),
		}
		err := handler.OnEndBlock(suite.Context(), k)
		require.NoError(t, err)
	})

	t.Run("assert no open orders", func(t *testing.T) {
		count := 0
		suite.MarketKeeper().WithOpenOrders(suite.Context(), func(o types.Order) bool {
			count++
			return false
		})
		assert.Equal(t, count, 0)
	})
	t.Run("assert leases created", func(t *testing.T) {
		count := 0
		suite.MarketKeeper().WithActiveLeases(suite.Context(), func(l types.Lease) bool {
			count++
			return false
		})
		assert.Equal(t, len(openGroups), count)
	})
}
