package provider_test

import (
	"fmt"
	"testing"

	app_ "github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

func TestProviderApp(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)
	app, err := app_.NewApp(testutil.Logger())
	require.NoError(t, err)

	account, key := testutil.CreateAccount(t, cacheState)
	nonce := uint64(1)

	provider := testutil.CreateProvider(t, cacheState, app, account, key, nonce)

	{
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: query.ProviderPath(provider.Address)}))
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", util.X(provider.Address))}))
	}

	{
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: query.ProviderPath(provider.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		queriedprovider := new(types.Provider)
		require.NoError(t, queriedprovider.Unmarshal(resp.Value))
		assert.NotEmpty(t, resp.Value)

		assert.Equal(t, provider.Address, queriedprovider.Address)
		assert.Equal(t, provider.Attributes[0].Name, queriedprovider.Attributes[0].Name)
		assert.Equal(t, provider.Attributes[0].Value, queriedprovider.Attributes[0].Value)
	}
}

func TestTx_BadTxType(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)
	app, err := app_.NewApp(testutil.Logger())
	account, key := testutil.CreateAccount(t, cacheState)

	tx := &types.Tx{
		Key: key.PubKey().Bytes(),
		Payload: types.TxPayload{
			Payload: &types.TxPayload_TxSend{
				TxSend: &types.TxSend{
					From:   account.Address,
					To:     account.Address,
					Amount: 0,
				},
			},
		},
	}
	ctx := apptypes.NewContext(tx)
	require.NoError(t, err)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(cacheState, ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(cacheState, ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}
