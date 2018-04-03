package market_test

import (
	"testing"

	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/require"
)

func TestEngine_All(t *testing.T) {
	state_ := testutil.NewState(t, nil)

	tenant, _ := testutil.CreateAccount(t, state_)

	pacc, _ := testutil.CreateAccount(t, state_)
	provider := testutil.Provider(pacc.Address, 0)
	require.NoError(t, state_.Provider().Save(provider))

	deployment := testutil.Deployment(tenant.Address, tenant.Nonce)
	groups := testutil.DeploymentGroups(deployment.Address, tenant.Nonce)
	require.NoError(t, state_.Deployment().Save(deployment))

	state_, tx := testOrder(t, state_, tenant, deployment, groups)
	state_ = testLease(t, state_, provider, deployment, groups, tx)
}

func testOrder(t *testing.T, state state.State, tenant *types.Account, deployment *types.Deployment, groups *types.DeploymentGroups) (state.State, *types.TxCreateOrder) {
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
		Deployment: tx.Deployment,
		Group:      tx.Group,
		Seq:        tx.Seq,
		EndAt:      tx.EndAt,
		State:      types.Order_OPEN,
	}
	require.NoError(t, state.Order().Save(order))

	return state, tx
}

func testLease(t *testing.T, state state.State, provider *types.Provider, deployment *types.Deployment, groups *types.DeploymentGroups, tx *types.TxCreateOrder) state.State {
	fulfillment := testutil.Fulfillment(provider.Address, deployment.Address, tx.Group, tx.Seq, 1)
	require.NoError(t, state.Fulfillment().Save(fulfillment))

	for i := int64(0); i <= groups.GetItems()[0].OrderTTL; i++ {
		state.Commit()
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state)
	require.NoError(t, err)
	require.Len(t, txs, 1)

	leaseTx, ok := txs[0].(*types.TxCreateLease)
	lease := &types.Lease{
		Deployment: leaseTx.Deployment,
		Group:      leaseTx.Group,
		Order:      leaseTx.Order,
		Provider:   leaseTx.Provider,
		Price:      leaseTx.Price,
		State:      types.Lease_ACTIVE,
	}
	require.True(t, ok)
	require.NoError(t, state.Lease().Save(lease))
	require.NoError(t, state.Lease().Save(lease))

	return state
}
