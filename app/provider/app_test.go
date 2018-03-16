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
	app, err := app_.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	provider := testutil.CreateProvider(t, app, account, &key, nonce)

	{
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(provider.Address))}))
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(provider.Address))}))
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(provider.Address))})
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
	state_ := testutil.NewState(t, nil)
	app, err := app_.NewApp(state_, testutil.Logger())
	account, key := testutil.CreateAccount(t, state_)
	pubkey := base.PubKey(key.PubKey())
	tx := &types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: &types.TxPayload_TxSend{
				TxSend: &types.TxSend{
					From:   base.Bytes(account.Address),
					To:     base.Bytes(account.Address),
					Amount: 0,
				},
			},
		},
	}
	ctx := apptypes.NewContext(tx)
	require.NoError(t, err)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}
