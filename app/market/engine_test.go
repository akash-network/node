package market_test

import (
	"testing"

	"github.com/ovrclk/photon/app/market"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/stretchr/testify/require"
)

func TestEngine_DeploymentOrders(t *testing.T) {
	state_ := testutil.NewState(t, nil)

	tenant, _ := testutil.CreateAccount(t, state_)

	deployment := testutil.Deployment(t, tenant.Address, tenant.Nonce)
	require.NoError(t, state_.Deployment().Save(deployment))

	for idx := range deployment.Groups {
		require.NoError(t, state_.DeploymentGroup().Save(&deployment.Groups[idx]))
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state_)
	require.NoError(t, err)

	require.Len(t, txs, 1)

	tx, ok := txs[0].(*types.TxCreateDeploymentOrder)
	require.True(t, ok)

	require.Equal(t, deployment.Address, tx.DeploymentOrder.Deployment)
	require.Equal(t, deployment.Groups[0].Seq, tx.DeploymentOrder.GetGroup())
	require.Equal(t, types.DeploymentOrder_OPEN, tx.DeploymentOrder.GetState())
}
