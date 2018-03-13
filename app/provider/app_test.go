package provider_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	app_ "github.com/ovrclk/akash/app/provider"
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
	state := testutil.NewState(t, nil)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(0)

	provider := testutil.Provider(account.Address, nonce)

	providertx := &types.TxPayload_TxCreateProvider{
		TxCreateProvider: &types.TxCreateProvider{
			Provider: *provider,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: providertx,
		},
	})

	app, err := app_.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(provider.Address))}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(provider.Address))}))

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
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(provider.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		queriedprovider := new(types.Provider)
		require.NoError(t, queriedprovider.Unmarshal(resp.Value))
		assert.NotEmpty(t, resp.Value)

		// assert.Equal(t, deployment.TxDeployment.From, dep.From)
		assert.Equal(t, providertx.TxCreateProvider.Provider.Address, queriedprovider.Address)
		assert.Equal(t, provider.Attributes[0].Name, queriedprovider.Attributes[0].Name)
		assert.Equal(t, provider.Attributes[0].Value, queriedprovider.Attributes[0].Value)
	}
}
