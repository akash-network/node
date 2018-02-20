package store_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/photon/app/account"
	"github.com/ovrclk/photon/app/store"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/ovrclk/photon/types/code"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
	wire "github.com/tendermint/go-wire"
)

func TestStoreApp(t *testing.T) {
	const balance uint64 = 100

	kmgr := testutil.KeyManager(t)

	keyfrom, _, err := kmgr.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(keyfrom.Address), Balance: balance},
		},
	})

	app, err := store.NewApp(state, testutil.Logger())

	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: store.QueryPath}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", account.QueryPath, hex.EncodeToString(keyfrom.Address))}))

	assert.False(t, app.AcceptTx(nil, nil))

	{
		key := append([]byte(account.QueryPath), keyfrom.Address...)
		resp := app.Query(tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		assert.True(t, resp.IsOK())
		assert.Equal(t, key, resp.Key.Bytes())
		assert.Len(t, resp.Value, 8)
		assert.Equal(t, balance, wire.GetUint64(resp.Value))
		assert.Empty(t, resp.Proof.Bytes())
	}

	{
		key := append([]byte(account.QueryPath), keyfrom.Address...)
		resp := app.Query(tmtypes.RequestQuery{
			Path:  store.QueryPath,
			Data:  key,
			Prove: true,
		})
		assert.True(t, resp.IsOK())
		assert.Equal(t, key, resp.Key.Bytes())
		assert.Len(t, resp.Value, 8)
		assert.Equal(t, balance, wire.GetUint64(resp.Value))
		assert.NotEmpty(t, resp.Proof.Bytes())
	}

	{
		key := append([]byte(account.QueryPath), keyfrom.Address...)
		resp := app.Query(tmtypes.RequestQuery{
			Path: "/bad",
			Data: key,
		})
		assert.False(t, resp.IsOK())
		assert.Nil(t, resp.Key)
		assert.Empty(t, resp.Proof.Bytes())
	}

	{
		key := []byte("/bad")
		resp := app.Query(tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		assert.True(t, resp.IsOK())
		assert.Empty(t, resp.Value)
		assert.Equal(t, key, resp.Key.Bytes())
		assert.Empty(t, resp.Proof.Bytes())
	}

	{
		resp := app.CheckTx(nil, nil)
		assert.False(t, resp.IsOK())
		assert.Equal(t, code.UNKNOWN_TRANSACTION, resp.Code)
	}

	{
		resp := app.DeliverTx(nil, nil)
		assert.False(t, resp.IsOK())
		assert.Equal(t, code.UNKNOWN_TRANSACTION, resp.Code)
	}

}
