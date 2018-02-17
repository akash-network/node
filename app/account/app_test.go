package account_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/photon/app/account"
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestAccountApp(t *testing.T) {

	const (
		balance uint64 = 150
		amount  uint64 = 100
	)

	kmgr := testutil.KeyManager(t)

	keyfrom, _, err := kmgr.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	keyto, _, err := kmgr.Create("keyto", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	send := &types.TxPayload_TxSend{
		TxSend: &types.TxSend{
			From:   base.Bytes(keyfrom.Address),
			To:     base.Bytes(keyto.Address),
			Amount: amount,
		},
	}

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(keyfrom.Address), Balance: balance},
		},
	})

	key := base.PubKey(keyfrom.PubKey)

	ctx := apptypes.NewContext(&types.Tx{
		Key: &key,
		Payload: types.TxPayload{
			Payload: send,
		},
	})

	app, err := account.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", account.QueryPath, hex.EncodeToString(keyfrom.Address))}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(keyfrom.Address))}))

	assert.True(t, app.AcceptTx(ctx, send))

	{
		resp := app.CheckTx(ctx, send)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.DeliverTx(ctx, send)
		assert.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", account.QueryPath, hex.EncodeToString(keyfrom.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		addr := new(types.Account)
		require.NoError(t, addr.Unmarshal(resp.Value))

		assert.Equal(t, send.TxSend.From, addr.Address)
		assert.Equal(t, balance-amount, addr.Balance)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", account.QueryPath, hex.EncodeToString(keyto.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		addr := new(types.Account)
		require.NoError(t, addr.Unmarshal(resp.Value))

		assert.Equal(t, send.TxSend.To, addr.Address)
		assert.Equal(t, amount, addr.Balance)
	}

}
