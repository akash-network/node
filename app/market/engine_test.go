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

	deployment := testutil.Deployment(tenant.Address, tenant.Nonce)
	groups := testutil.DeploymentGroups(deployment.Address, tenant.Nonce)
	require.NoError(t, state_.Deployment().Save(deployment))

	for idx := range groups.GetItems() {
		require.NoError(t, state_.DeploymentGroup().Save(groups.GetItems()[idx]))
	}

	txs, err := market.NewEngine(testutil.Logger()).Run(state_)
	require.NoError(t, err)

	require.Len(t, txs, 1)

	tx, ok := txs[0].(*types.TxCreateOrder)
	require.True(t, ok)

	require.Equal(t, deployment.Address, tx.Order.Deployment)
	require.Equal(t, groups.GetItems()[0].Seq, tx.Order.GetGroup())
	require.Equal(t, types.Order_OPEN, tx.Order.GetState())
	require.NoError(t, state_.Order().Save(tx.Order))

	pacc, _ := testutil.CreateAccount(t, state_)
	provider := testutil.Provider(pacc.Address, 0)
	require.NoError(t, state_.Provider().Save(provider))

	fulfillment := testutil.Fulfillment(provider.Address, deployment.Address, tx.Order.GetGroup(), tx.Order.GetOrder(), 1)
	require.NoError(t, state_.Fulfillment().Save(fulfillment))

	for i := int64(0); i <= groups.GetItems()[0].OrderTTL; i++ {
		state_.Commit()
	}

	txs, err = market.NewEngine(testutil.Logger()).Run(state_)
	require.NoError(t, err)
	require.Len(t, txs, 1)

	leaseTx, ok := txs[0].(*types.TxCreateLease)
	require.True(t, ok)
	require.NoError(t, state_.Lease().Save(leaseTx.GetLease()))
	require.NoError(t, state_.Lease().Save(leaseTx.GetLease()))

	matchedOrder := tx.GetOrder()
	matchedOrder.State = types.Order_MATCHED
	require.NoError(t, state_.Order().Save(matchedOrder))

	iTenBal := getBalance(t, state_, tenant.Address)
	iProBal := getBalance(t, state_, provider.Owner)
	require.NotZero(t, iTenBal)
	require.NotZero(t, iProBal)

	txs, err = market.NewEngine(testutil.Logger()).Run(state_)
	require.NoError(t, err)
	require.Len(t, txs, 0)

	fTenBal := getBalance(t, state_, tenant.Address)
	fProBal := getBalance(t, state_, provider.Owner)
	require.Equal(t, iTenBal-1, fTenBal)
	require.Equal(t, iProBal+1, fProBal)
}

func getBalance(t *testing.T, state state.State, address base.Bytes) uint64 {
	acc, err := state.Account().Get(address)
	require.NoError(t, err)
	return acc.GetBalance()
}
