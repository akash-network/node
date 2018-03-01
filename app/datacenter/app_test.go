package datacenter_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/photon/app/datacenter"
	apptypes "github.com/ovrclk/photon/app/types"
	pstate "github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestDatacenterApp(t *testing.T) {

	const (
		name     = "region"
		value    = "us-west"
		number   = uint32(1)
		number64 = uint64(1)
	)

	address := []byte("address")

	kmgr := testutil.KeyManager(t)

	keyfrom, _, err := kmgr.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

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

	attributes := []types.ProviderAttribute{*providerattribute}
	resources := []types.ResourceGroup{*resourcegroup}

	dc := &types.Datacenter{
		Attributes: attributes,
		Resources:  resources,
		Address:    address,
		Owner:      base.Bytes(keyfrom.Address),
	}

	datacentertx := &types.TxPayload_TxCreateDatacenter{
		TxCreateDatacenter: &types.TxCreateDatacenter{
			Datacenter: *dc,
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
			Payload: datacentertx,
		},
	})

	app, err := datacenter.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.DatacenterPath, hex.EncodeToString(address))}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(address))}))

	assert.True(t, app.AcceptTx(ctx, datacentertx))

	{
		resp := app.CheckTx(ctx, datacentertx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.DeliverTx(ctx, datacentertx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.DatacenterPath, hex.EncodeToString(address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dc := new(types.Datacenter)
		require.NoError(t, dc.Unmarshal(resp.Value))

		// assert.Equal(t, deployment.TxDeployment.From, dep.From)
		assert.Equal(t, datacentertx.TxCreateDatacenter.Datacenter.Address, dc.Address)
		assert.Equal(t, dc.Attributes[0].Name, name)
		assert.Equal(t, dc.Attributes[0].Value, value)
		assert.Equal(t, dc.Resources[0].Count, number)
		assert.Equal(t, dc.Resources[0].Price, number)
		assert.Equal(t, dc.Resources[0].Unit.Cpu, number)
		assert.Equal(t, dc.Resources[0].Unit.Disk, number64)
		assert.Equal(t, dc.Resources[0].Unit.Memory, number)
	}
}
