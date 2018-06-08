package market_test

import (
	"testing"

	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/require"
)

func TestEngine_All(t *testing.T) {
	commitState, _ := testutil.NewState(t, nil)

	tenant, _ := testutil.CreateAccount(t, commitState)

	pacc, _ := testutil.CreateAccount(t, commitState)
	provider := testutil.Provider(pacc.Address, 0)
	require.NoError(t, commitState.Provider().Save(provider))

	deployment := testutil.Deployment(tenant.Address, tenant.Nonce)
	groups := testutil.DeploymentGroups(deployment.Address, tenant.Nonce)
	require.NoError(t, commitState.Deployment().Save(deployment))

	_, order := testCreateOrder(t, commitState, tenant, deployment, groups)
	testCreateLease(t, commitState, provider, deployment, groups, order)
	testCloseDeployment(t, commitState, tenant.Address, deployment.Address)
}

func testCreateOrder(t *testing.T, state state.State, tenant *types.Account, deployment *types.Deployment, groups *types.DeploymentGroups) (state.State, *types.Order) {
	for _, group := range groups.GetItems() {
		require.NoError(t, state.DeploymentGroup().Save(group))
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state)
	require.NoError(t, err)

	require.Len(t, txs, 1)

	tx, ok := txs[0].(*types.TxCreateOrder)
	require.True(t, ok)

	require.Equal(t, deployment.Address, tx.Deployment)
	require.Equal(t, groups.GetItems()[0].Seq, tx.Group)
	order := &types.Order{
		OrderID: tx.OrderID,
		EndAt:   tx.EndAt,
		State:   types.Order_OPEN,
	}
	require.NoError(t, state.Order().Save(order))

	return state, order
}

func testCreateLease(t *testing.T, state state.CommitState, provider *types.Provider, deployment *types.Deployment, groups *types.DeploymentGroups, order *types.Order) state.State {
	fulfillment := testutil.Fulfillment(provider.Address, deployment.Address, order.Group, order.Seq, 1)
	require.NoError(t, state.Fulfillment().Save(fulfillment))

	for i := int64(0); i <= groups.GetItems()[0].OrderTTL; i++ {
		state.Commit()
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state)
	require.NoError(t, err)
	require.Len(t, txs, 1)

	leaseTx, ok := txs[0].(*types.TxCreateLease)
	lease := &types.Lease{
		LeaseID: leaseTx.LeaseID,
		Price:   leaseTx.Price,
		State:   types.Lease_ACTIVE,
	}
	require.True(t, ok)
	require.NoError(t, state.Lease().Save(lease))

	order.State = types.Order_MATCHED
	require.NoError(t, state.Order().Save(order))

	return state
}

func testCloseDeployment(t *testing.T, state state.State, address base.Bytes, deployment base.Bytes) state.State {
	tenant, err := state.Account().Get(address)
	require.NoError(t, err)

	tenant.Balance = uint64(0)
	require.NoError(t, state.Account().Save(tenant))

	txs, err := market.NewEngine(testutil.Logger()).Run(state)
	require.NoError(t, err)
	require.Len(t, txs, 1)

	tx, ok := txs[0].(*types.TxCloseDeployment)
	require.True(t, ok)
	require.Equal(t, types.TxCloseDeployment_INSUFFICIENT, tx.Reason)
	require.Equal(t, deployment, tx.Deployment)

	return state
}
