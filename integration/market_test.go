// +build integration,!mainnet

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
	f.TxCreateDeployment(deploymentFilePath, fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments, err := f.QueryDeployments()
	require.NoError(t, err)
	require.Len(t, deployments, 1, "Deployment Creation Failed in TestMarket")
	require.Equal(t, fooAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query orders
	orders, err := f.QueryOrders()
	require.NoError(t, err)
	require.Len(t, orders, 1)
	require.Equal(t, fooAddr.String(), orders[0].OrderID.Owner.String())

	// test query order
	createdOrder := orders[0]
	order := f.QueryOrder(createdOrder.OrderID)
	require.Equal(t, createdOrder, order)

	// test query orders with owner filter
	orders, err = f.QueryOrders(fmt.Sprintf("--owner=%s", fooAddr.String()))
	require.NoError(t, err, "Error when fetching orders with owner filter")
	require.Len(t, orders, 1)

	// test query orders with wrong owner value
	orders, err = f.QueryOrders("--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")
	require.Error(t, err)

	// test query orders with filters
	orders, err = f.QueryOrders("--state=closed")
	require.NoError(t, err)
	require.Len(t, orders, 0)

	// test query orders with wrong state filter
	orders, err = f.QueryOrders("--state=hello")
	require.Error(t, err)

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
	bids, err := f.QueryBids()
	require.NoError(t, err)
	require.Len(t, bids, 1, "Creating bid failed")
	require.Equal(t, barAddr.String(), bids[0].Provider.String())

	// test query bid
	createdBid := bids[0]
	bid := f.QueryBid(createdBid.BidID)
	require.Equal(t, createdBid, bid)

	// test query bids with owner filter
	bids, err = f.QueryBids(fmt.Sprintf("--owner=%s", fooAddr.String()))
	require.NoError(t, err, "Error when fetching bids with owner filter")
	require.Len(t, bids, 1)

	// test query bids with wrong owner value
	bids, err = f.QueryBids("--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")
	require.Error(t, err)

	// test query leases
	leases, err := f.QueryLeases()
	require.NoError(t, err)
	require.Len(t, leases, 1)

	// test query order
	createdLease := leases[0]
	lease := f.QueryLease(createdLease.LeaseID)
	require.Equal(t, createdLease, lease)

	// test query leases with owner filter
	leases, err = f.QueryLeases(fmt.Sprintf("--owner=%s", fooAddr.String()))
	require.NoError(t, err, "Error when fetching leases with owner filter")
	require.Len(t, leases, 1)

	// test query leases with wrong owner value
	leases, err = f.QueryLeases("--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")
	require.Error(t, err)

	// Close Bid
	f.TxCloseBid(createdOrder.OrderID, fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids with filter
	closedBids, err := f.QueryBids("--state=closed")
	require.NoError(t, err)
	require.Len(t, closedBids, 1, "Closing bid failed")
	require.Equal(t, barAddr.String(), closedBids[0].Provider.String())

	// test query leases with filter
	closedLeases, err := f.QueryLeases("--state=closed")
	require.NoError(t, err)
	require.Len(t, closedLeases, 1)

	// test query orders with filter state open
	openedOrders, err := f.QueryOrders("--state=open")
	require.NoError(t, err)
	require.Len(t, openedOrders, 1)

	// Creating bid again for new order
	f.TxCreateBid(openedOrders[0].OrderID, sdk.NewInt64Coin(denom, 20), fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids
	matchedBids, err := f.QueryBids("--state=matched")
	require.NoError(t, err)
	require.Len(t, matchedBids, 1, "Creating bid failed second time")

	// test query bids with wrong state filter
	bids, err = f.QueryBids("--state=hello")
	require.Error(t, err)

	// test query leases with wrong state filter
	leases, err = f.QueryLeases("--state=hello")
	require.Error(t, err)

	// Close Order
	f.TxCloseOrder(openedOrders[0].OrderID, fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query orders with filter state closed
	closedOrders, err := f.QueryOrders("--state=closed")
	require.NoError(t, err)
	require.Len(t, closedOrders, 2, "Closing Order failed")

	f.Cleanup()
}
