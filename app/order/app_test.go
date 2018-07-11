package order_test

import (
	"testing"

	deployment_ "github.com/ovrclk/akash/app/deployment"
	"github.com/ovrclk/akash/app/order"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	app, err := order.NewApp(testutil.Logger())
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
	_, cacheState := testutil.NewState(t, nil)
	app, err := order.NewApp(testutil.Logger())
	dapp, err := deployment_.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)

	deployment, groups := testutil.CreateDeployment(t, cacheState, dapp, account, key, 1)

	orderSeq := uint64(0)
	testutil.CreateOrder(t, cacheState, app, account, key, deployment.Address, groups.GetItems()[0].Seq, orderSeq)

	orders, err := cacheState.Order().ForGroup(groups.GetItems()[0].DeploymentGroupID)
	require.NoError(t, err)
	require.Len(t, orders, 1)

	order := orders[0]

	path := query.OrderPath(order.OrderID)
	resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())
	resp = app.Query(cacheState, tmtypes.RequestQuery{Path: state.OrderPath})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())
}

func TestTx_BadTxType(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)
	app, err := order.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	tx := testutil.ProviderTx(account, key, 10)
	ctx := apptypes.NewContext(tx)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(cacheState, ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(cacheState, ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}
