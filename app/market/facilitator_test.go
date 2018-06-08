package market_test

import (
	"testing"

	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/app/market/mocks"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

func TestFacilitator(t *testing.T) {
	commitState, _ := testutil.NewState(t, nil)
	account, key := testutil.CreateAccount(t, commitState)

	nonce := uint64(10)

	account.Nonce = nonce
	commitState.Account().Save(account)

	daddr := state.DeploymentAddress(account.Address, nonce)
	tx := &types.TxCreateOrder{
		OrderID: types.OrderID{
			Deployment: daddr,
		},
	}

	txs := []interface{}{tx}

	engine := new(mocks.Engine)
	engine.On("Run", commitState).
		Return(txs, nil).Once()

	client := new(mocks.Client)
	client.On("BroadcastTxAsync", mock.AnythingOfType("types.Tx")).Run(func(args mock.Arguments) {
		txbuf, ok := args.Get(0).(tmtmtypes.Tx)
		require.True(t, ok)

		tx, err := txutil.ProcessTx(txbuf)
		require.NoError(t, err)

		assert.Equal(t, key.PubKey().Bytes(), tx.Key)

		payload := tx.GetPayload()
		assert.Equal(t, nonce+1, payload.GetNonce())

		txdo := payload.GetTxCreateOrder()
		require.NotNil(t, txdo)

	}).Return(nil, nil).Once()

	actor := market.NewActor(key)

	facilitator := market.NewFacilitator(testutil.Logger(), actor, engine, client)

	require.NoError(t, facilitator.Run(commitState))

}
