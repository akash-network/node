package deployment_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/akash/app/deployment"
	apptypes "github.com/ovrclk/akash/app/types"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	state := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, state)
	address := account.Address

	app, err := deployment.NewApp(state, testutil.Logger())
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

func TestCreateTx(t *testing.T) {
	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl := testutil.CreateDeployment(t, app, account, &key, nonce)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, depl.Tenant, dep.Tenant)
		assert.Equal(t, depl.Address, dep.Address)

		require.Len(t, dep.Groups, 1)
		assert.Equal(t, dep.Groups[0].Requirements, depl.Groups[0].Requirements)
		assert.Equal(t, dep.Groups[0].Resources, depl.Groups[0].Resources)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: pstate.DeploymentPath})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: pstate.DeploymentPath + hex.EncodeToString(pstate.DeploymentGroupID(depl.Address, 1))})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		path := pstate.DeploymentGroupPath + hex.EncodeToString(pstate.DeploymentGroupID(depl.Address, 1))
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		path := pstate.DeploymentGroupPath + hex.EncodeToString(pstate.DeploymentGroupID(depl.Address, 0))
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		groups, err := state.DeploymentGroup().ForDeployment(depl.Address)
		require.NoError(t, err)
		require.Len(t, groups, 1)

		assert.Equal(t, groups[0].Requirements, depl.Groups[0].Requirements)
		assert.Equal(t, groups[0].Resources, depl.Groups[0].Resources)
	}
}

func TestTx_BadTxType(t *testing.T) {
	state_ := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state_, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state_)
	tx := testutil.ProviderTx(account, &key, 10)
	ctx := apptypes.NewContext(tx)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}

func TestCloseTx(t *testing.T) {
	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl := testutil.CreateDeployment(t, app, account, &key, nonce)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, dep.State, depl.State)
		assert.Equal(t, dep.State, types.Deployment_ACTIVE)

		require.Len(t, dep.Groups, 1)
		assert.Equal(t, dep.Groups[0].State, depl.Groups[0].State)
		assert.Equal(t, dep.Groups[0].State, types.DeploymentGroup_OPEN)
	}

	testutil.CloseDeployment(t, app, &depl.Address, &key)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSING, dep.State)

		require.Len(t, dep.Groups, 1)
		assert.Equal(t, types.DeploymentGroup_CLOSING, dep.Groups[0].State)
	}

	testutil.DeploymentClosed(t, app, &depl.Address, &key)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSED, dep.State)

		require.Len(t, dep.Groups, 1)
		assert.Equal(t, types.DeploymentGroup_CLOSED, dep.Groups[0].State)
	}
}
