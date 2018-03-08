package market_test

import (
	"testing"

	"github.com/ovrclk/photon/app/market"
	"github.com/ovrclk/photon/app/market/mocks"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/go-crypto"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

func TestFacilitator(t *testing.T) {
	state_ := testutil.NewState(t, nil)
	account, key := testutil.CreateAccount(t, state_)

	nonce := uint64(10)

	account.Nonce = nonce
	state_.Account().Save(account)

	daddr := state.DeploymentAddress(account.Address, nonce)
	tx := &types.TxCreateDeploymentOrder{
		DeploymentOrder: &types.DeploymentOrder{
			Deployment: daddr,
		},
	}

	txs := []interface{}{tx}

	engine := new(mocks.Engine)
	engine.On("Run", state_).
		Return(txs, nil).Once()

	client := new(mocks.Client)
	client.On("BroadcastTxAsync", mock.AnythingOfType("types.Tx")).Run(func(args mock.Arguments) {
		txbuf, ok := args.Get(0).(tmtmtypes.Tx)
		require.True(t, ok)

		tx, err := txutil.ProcessTx(txbuf)
		require.NoError(t, err)

		assert.Equal(t, key.PubKey(), crypto.PubKey(*tx.Key))

		payload := tx.GetPayload()
		assert.Equal(t, nonce+1, payload.GetNonce())

		txdo := payload.GetTxCreateDeploymentOrder()
		require.NotNil(t, txdo)

	}).Return(nil, nil).Once()

	actor := market.NewActor(key)

	facilitator := market.NewFacilitator(testutil.Logger(), actor, engine, client)

	require.NoError(t, facilitator.Run(state_))

}
