package deployment_test

import (
	"fmt"
	"testing"

	"github.com/ovrclk/akash/app/deployment"
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

func TestValidTx(t *testing.T) {
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
		groups, err := state.DeploymentGroup().ForDeployment(depl.Address)
		require.NoError(t, err)
		require.Len(t, groups, 1)

		assert.Equal(t, groups[0].Requirements, depl.Groups[0].Requirements)
		assert.Equal(t, groups[0].Resources, depl.Groups[0].Resources)
	}
}
