package deployment_test

import (
	"fmt"
	"testing"

	"github.com/ovrclk/photon/app/deployment"
	apptypes "github.com/ovrclk/photon/app/types"
	pstate "github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	state_ := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, state_)
	address := account.Address

	app, err := deployment.NewApp(state_, testutil.Logger())
	require.NoError(t, err)

	{
		path := fmt.Sprintf("%v%X", pstate.DeploymentPath, address)
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}

	{
		path := fmt.Sprintf("%v%X", "/foo/", address)
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}
}

func TestValidTx(t *testing.T) {

	state_ := testutil.NewState(t, nil)

	account, key := testutil.CreateAccount(t, state_)

	depl := testutil.Deployment(t, account.Address, 0)

	tx := &types.TxPayload_TxCreateDeployment{
		TxCreateDeployment: &types.TxCreateDeployment{
			Deployment: depl,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: tx,
		},
	})

	app, err := deployment.NewApp(state_, testutil.Logger())
	require.NoError(t, err)

	assert.True(t, app.AcceptTx(ctx, tx))

	{
		resp := app.CheckTx(ctx, tx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.DeliverTx(ctx, tx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, tx.TxCreateDeployment.Deployment.Tenant, dep.Tenant)
		assert.Equal(t, tx.TxCreateDeployment.Deployment.Address, dep.Address)

		require.Len(t, dep.Groups, 1)
		assert.Equal(t, dep.Groups[0].Requirements, depl.Groups[0].Requirements)
		assert.Equal(t, dep.Groups[0].Resources, depl.Groups[0].Resources)
	}

	{
		groups, err := state_.DeploymentGroup().ForDeployment(depl.Address)
		require.NoError(t, err)
		require.Len(t, groups, 1)

		assert.Equal(t, groups[0].Requirements, depl.Groups[0].Requirements)
		assert.Equal(t, groups[0].Resources, depl.Groups[0].Resources)
	}
}
