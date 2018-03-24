package market_test

import (
	"testing"

	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/require"
)

func TestEngine_Orders(t *testing.T) {
	state_ := testutil.NewState(t, nil)

	tenant, _ := testutil.CreateAccount(t, state_)

	deployment := testutil.Deployment(tenant.Address, tenant.Nonce)
	groups := testutil.DeploymentGroups(deployment.Address, tenant.Nonce)
	require.NoError(t, state_.Deployment().Save(deployment))

	for idx := range groups.GetItems() {
		require.NoError(t, state_.DeploymentGroup().Save(&groups.GetItems()[idx]))
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state_)
	require.NoError(t, err)

	require.Len(t, txs, 1)

	tx, ok := txs[0].(*types.TxCreateOrder)
	require.True(t, ok)

	require.Equal(t, deployment.Address, tx.Order.Deployment)
	require.Equal(t, groups.GetItems()[0].Seq, tx.Order.GetGroup())
	require.Equal(t, types.Order_OPEN, tx.Order.GetState())
}
