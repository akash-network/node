package order_test

import (
	"testing"

	"github.com/ovrclk/akash/app/order"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"

	apptypes "github.com/ovrclk/akash/app/types"
)

func TestAcceptQuery(t *testing.T) {
	state := testutil.NewState(t, nil)

	app, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err, "failed to create app")

	{
		data := make([]byte, 0)
		path := "/orders/"
		prove := false
		height := int64(0)
		query := tmtypes.RequestQuery{
			Data:   data,
			Path:   path,
			Height: height,
			Prove:  prove,
		}
		res := app.AcceptQuery(query)
		assert.True(t, res, "app rejcted valid query")
	}

	{
		data := make([]byte, 0)
		path := "/deployments/"
		prove := false
		height := int64(0)
		query := tmtypes.RequestQuery{
			Data:   data,
			Path:   path,
			Height: height,
			Prove:  prove,
		}
		res := app.AcceptQuery(query)
		assert.False(t, res, "app accepted invalid query")
	}
}

func TestTx(t *testing.T) {
	state_ := testutil.NewState(t, nil)
	account, key := testutil.CreateAccount(t, state_)

	deployment := createDeployment(t, state_, account)

	tx := &types.TxPayload_TxCreateOrder{
		TxCreateOrder: &types.TxCreateOrder{
			Order: &types.Order{
				Deployment: deployment.Address,
				Group:      deployment.Groups[0].Seq,
			},
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: tx,
		},
	})

	app, err := order.NewApp(state_, testutil.Logger())
	require.NoError(t, err)

	assert.True(t, app.AcceptTx(ctx, tx))

	{
		res := app.CheckTx(ctx, tx)
		require.True(t, res.IsOK())
	}

	{
		res := app.DeliverTx(ctx, tx)
		require.True(t, res.IsOK())
	}

	orders, err := state_.Order().ForGroup(&deployment.Groups[0])
	require.NoError(t, err)
	require.Len(t, orders, 1)
}

func createDeployment(t *testing.T, state_ state.State, account *types.Account) *types.Deployment {
	deployment := testutil.Deployment(account.Address, 10)

	require.NoError(t, state_.Deployment().Save(deployment))

	for idx := range deployment.Groups {
		require.NoError(t, state_.DeploymentGroup().Save(&deployment.Groups[idx]))
	}
	return deployment
}
