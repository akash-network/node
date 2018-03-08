package store_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/photon/app/store"
	pstate "github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/ovrclk/photon/types/code"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

const (
	balance uint64 = 100
	nonce   uint64 = 1
)

func TestStoreApp(t *testing.T) {

	kmgr := testutil.KeyManager(t)

	keyfrom, _, err := kmgr.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(keyfrom.Address), Balance: balance, Nonce: nonce},
		},
	})

	app, err := store.NewApp(state, testutil.Logger())

	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: store.QueryPath}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.AccountPath, hex.EncodeToString(keyfrom.Address))}))

	assert.False(t, app.AcceptTx(nil, nil))

	{
		key := append([]byte(pstate.AccountPath), keyfrom.Address...)
		resp := app.Query(tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		acc := new(types.Account)
		acc.Unmarshal(resp.Value)
		assert.True(t, resp.IsOK())
		assert.Equal(t, key, resp.Key)
		assert.Equal(t, balance, acc.Balance)
		assert.Empty(t, resp.Proof)
	}

	{
		key := append([]byte(pstate.AccountPath), keyfrom.Address...)
		resp := app.Query(tmtypes.RequestQuery{
			Path:  store.QueryPath,
			Data:  key,
			Prove: true,
		})
		acc := new(types.Account)
		acc.Unmarshal(resp.Value)
		assert.True(t, resp.IsOK())
		assert.Equal(t, key, resp.Key)
		assert.Equal(t, balance, acc.Balance)
		assert.NotEmpty(t, resp.Proof)
	}

	{
		key := append([]byte(pstate.AccountPath), keyfrom.Address...)
		resp := app.Query(tmtypes.RequestQuery{
			Path: "/bad",
			Data: key,
		})
		assert.False(t, resp.IsOK())
		assert.Nil(t, resp.Key)
		assert.Empty(t, resp.Proof)
	}

	{
		key := []byte("/bad")
		resp := app.Query(tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		assert.True(t, resp.IsOK())
		assert.Empty(t, resp.Value)
		assert.Equal(t, key, resp.Key)
		assert.Empty(t, resp.Proof)
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
