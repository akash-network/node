// +build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestMarket(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start akashd server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)
	barAddr := f.KeyAddress(keyBar)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Create deployment
	f.TxCreateDeployment(fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments := f.QueryDeployments()
	require.Len(t, deployments, 1, "Deployment Creation Failed in TestMarket")
	require.Equal(t, fooAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query orders
	orders := f.QueryOrders()
	require.Len(t, orders, 1)
	require.Equal(t, fooAddr.String(), orders[0].OrderID.Owner.String())

	// test query order
	createdOrder := orders[0]
	order := f.QueryOrder(createdOrder.OrderID)
	require.Equal(t, createdOrder, order)

	// Create provider
	f.TxCreateProvider(fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query providers
	providers := f.QueryProviders()
	require.Len(t, providers, 1, "Provider Creation Failed in TestMarket")
	require.Equal(t, barAddr.String(), providers[0].Owner.String())

	// Create Bid
	f.TxCreateBid(createdOrder.OrderID, sdk.NewInt64Coin(denom, 20), fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids
	bids := f.QueryBids()
	require.Len(t, bids, 1, "Creating bid failed")
	require.Equal(t, barAddr.String(), bids[0].Provider.String())

	// test query bid
	createdBid := bids[0]
	bid := f.QueryBid(createdBid.BidID)
	require.Equal(t, createdBid, bid)

	// test query leases
	leases := f.QueryLeases()
	require.Len(t, leases, 1)

	// test query order
	createdLease := leases[0]
	lease := f.QueryLease(createdLease.LeaseID)
	require.Equal(t, createdLease, lease)

	// Close Bid
	f.TxCloseBid(createdOrder.OrderID, fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids with filter
	closedBids := f.QueryBids("--state=closed")
	require.Len(t, closedBids, 1, "Closing bid failed")
	require.Equal(t, barAddr.String(), closedBids[0].Provider.String())

	// test query leases with filter
	closedLeases := f.QueryLeases("--state=closed")
	require.Len(t, closedLeases, 1)

	// test query orders with filter state open
	openedOrders := f.QueryOrders("--state=open")
	require.Len(t, openedOrders, 1)

	// Creating bid again for new order
	f.TxCreateBid(openedOrders[0].OrderID, sdk.NewInt64Coin(denom, 20), fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids
	matchedBids := f.QueryBids("--state=matched")
	require.Len(t, matchedBids, 1, "Creating bid failed second time")

	// Close Order
	f.TxCloseOrder(openedOrders[0].OrderID, fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query orders with filter state closed
	closedOrders := f.QueryOrders("--state=closed")
	require.Len(t, closedOrders, 2, "Closing Order failed")

	f.Cleanup()
}
