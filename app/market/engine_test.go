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
	state_ := testutil.NewState(t, nil)

	tenant, _ := testutil.CreateAccount(t, state_)

	pacc, _ := testutil.CreateAccount(t, state_)
	provider := testutil.Provider(pacc.Address, 0)
	require.NoError(t, state_.Provider().Save(provider))

	deployment := testutil.Deployment(tenant.Address, tenant.Nonce)
	groups := testutil.DeploymentGroups(deployment.Address, tenant.Nonce)
	require.NoError(t, state_.Deployment().Save(deployment))

	state_, tx := testCreateOrder(t, state_, tenant, deployment, groups)
	state_ = testCreateLease(t, state_, provider, deployment, groups, tx)
	state_ = testCloseDeployment(t, state_, tenant.Address, deployment.Address)
}

func testCreateOrder(t *testing.T, state state.State, tenant *types.Account, deployment *types.Deployment, groups *types.DeploymentGroups) (state.State, *types.TxCreateOrder) {
	for _, group := range groups.GetItems() {
		require.NoError(t, state.DeploymentGroup().Save(group))
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state)
	require.NoError(t, err)

	require.Len(t, txs, 1)

	tx, ok := txs[0].(*types.TxCreateOrder)
	require.True(t, ok)

	require.Equal(t, deployment.Address, tx.Order.Deployment)
	require.Equal(t, groups.GetItems()[0].Seq, tx.Order.GetGroup())
	require.Equal(t, types.Order_OPEN, tx.Order.GetState())
	require.NoError(t, state.Order().Save(tx.Order))

	return state, tx
}

func testCreateLease(t *testing.T, state state.State, provider *types.Provider, deployment *types.Deployment, groups *types.DeploymentGroups, tx *types.TxCreateOrder) state.State {
	fulfillment := testutil.Fulfillment(provider.Address, deployment.Address, tx.Order.GetGroup(), tx.Order.GetOrder(), 1)
	require.NoError(t, state.Fulfillment().Save(fulfillment))

	for i := int64(0); i <= groups.GetItems()[0].OrderTTL; i++ {
		state.Commit()
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state)
	require.NoError(t, err)
	require.Len(t, txs, 1)

	leaseTx, ok := txs[0].(*types.TxCreateLease)
	require.True(t, ok)
	require.NoError(t, state.Lease().Save(leaseTx.GetLease()))
	require.NoError(t, state.Lease().Save(leaseTx.GetLease()))

	matchedOrder := tx.GetOrder()
	matchedOrder.State = types.Order_MATCHED
	require.NoError(t, state.Order().Save(matchedOrder))

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
