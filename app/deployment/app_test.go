package deployment_test

import (
	"encoding/hex"
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

func TestDeploymentApp(t *testing.T) {

	const (
		name     = "region"
		value    = "us-west"
		number   = uint32(1)
		number64 = uint64(1)
	)

	kmgr := testutil.KeyManager(t)

	keyfrom, _, err := kmgr.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	address := []byte("address")

	resourceunit := &types.ResourceUnit{
		Cpu:    number,
		Memory: number,
		Disk:   number64,
	}

	resourcegroup := &types.ResourceGroup{
		Unit:  *resourceunit,
		Count: number,
		Price: number,
	}

	providerattribute := &types.ProviderAttribute{
		Name:  name,
		Value: value,
	}

	requirements := []types.ProviderAttribute{*providerattribute}
	resources := []types.ResourceGroup{*resourcegroup}

	deploymentgroup := &types.DeploymentGroup{
		Requirements: requirements,
		Resources:    resources,
	}

	groups := []types.DeploymentGroup{*deploymentgroup}

	deploymenttx := &types.TxPayload_TxDeployment{
		TxDeployment: &types.TxDeployment{
			From: base.Bytes(keyfrom.Address),
			Deployment: &types.Deployment{
				Address: address,
				Groups:  groups,
			},
		},
	}

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(keyfrom.Address), Balance: 0},
		},
	})

	key := base.PubKey(keyfrom.PubKey)

	ctx := apptypes.NewContext(&types.Tx{
		Key: &key,
		Payload: types.TxPayload{
			Payload: deploymenttx,
		},
	})

	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.DeploymentPath, hex.EncodeToString(address))}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(keyfrom.Address))}))

	assert.True(t, app.AcceptTx(ctx, deploymenttx))

	{
		resp := app.CheckTx(ctx, deploymenttx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.DeliverTx(ctx, deploymenttx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.DeploymentPath, hex.EncodeToString(address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		// assert.Equal(t, deployment.TxDeployment.From, dep.From)
		assert.Equal(t, deploymenttx.TxDeployment.Deployment.Address, dep.Address)
		assert.Equal(t, dep.Groups[0].Requirements[0].Name, name)
		assert.Equal(t, dep.Groups[0].Requirements[0].Value, value)
		assert.Equal(t, dep.Groups[0].Resources[0].Count, number)
		assert.Equal(t, dep.Groups[0].Resources[0].Price, number)
		assert.Equal(t, dep.Groups[0].Resources[0].Unit.Cpu, number)
		assert.Equal(t, dep.Groups[0].Resources[0].Unit.Disk, number64)
		assert.Equal(t, dep.Groups[0].Resources[0].Unit.Memory, number)

	}
}
