package provider_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestProviderApp(t *testing.T) {

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

	providerattribute := &types.ProviderAttribute{
		Name:  name,
		Value: value,
	}

	attributes := []types.ProviderAttribute{*providerattribute}

	dc := &types.Provider{
		Attributes: attributes,
		Address:    address,
		Owner:      base.Bytes(keyfrom.Address),
	}

	providertx := &types.TxPayload_TxCreateProvider{
		TxCreateProvider: &types.TxCreateProvider{
			Provider: *dc,
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
			Payload: providertx,
		},
	})

	app, err := provider.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(address))}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(address))}))

	assert.True(t, app.AcceptTx(ctx, providertx))

	{
		resp := app.CheckTx(ctx, providertx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.DeliverTx(ctx, providertx)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dc := new(types.Provider)
		require.NoError(t, dc.Unmarshal(resp.Value))

		// assert.Equal(t, deployment.TxDeployment.From, dep.From)
		assert.Equal(t, providertx.TxCreateProvider.Provider.Address, dc.Address)
		assert.Equal(t, dc.Attributes[0].Name, name)
		assert.Equal(t, dc.Attributes[0].Value, value)
	}
}
