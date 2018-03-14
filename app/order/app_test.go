package order_test

import (
	"testing"

	deployment_ "github.com/ovrclk/akash/app/deployment"
	"github.com/ovrclk/akash/app/order"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
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
	app, err := order.NewApp(state_, testutil.Logger())
	dapp, err := deployment_.NewApp(state_, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state_)

	deployment := testutil.CreateDeployment(t, dapp, account, &key, 10)

	testutil.CreateOrder(t, app, account, &key, deployment.Address, deployment.Groups[0].Seq, 0)

	orders, err := state_.Order().ForGroup(&deployment.Groups[0])
	require.NoError(t, err)
	require.Len(t, orders, 1)
}
