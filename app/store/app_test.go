package store_test

import (
	"testing"

	"github.com/ovrclk/akash/app/store"
	"github.com/ovrclk/akash/query"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

func TestStoreApp(t *testing.T) {

	state, _ := testutil.NewState(t, nil)
	acct, _ := testutil.CreateAccount(t, state)
	fromaddr := acct.Address

	app, err := store.NewApp(testutil.Logger())
	require.NoError(t, err)

	assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: store.QueryPath}))
	assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: query.AccountPath(fromaddr)}))

	assert.False(t, app.AcceptTx(nil, nil))

	{
		key := append([]byte(pstate.AccountPath), fromaddr...)
		resp := app.Query(state, tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		acc := new(types.Account)
		acc.Unmarshal(resp.Value)
		assert.True(t, resp.IsOK())
		assert.Equal(t, acct.Balance, acc.Balance)
		assert.Empty(t, resp.Proof)
	}

	{
		key := append([]byte(pstate.AccountPath), fromaddr...)
		resp := app.Query(state, tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		acc := new(types.Account)
		acc.Unmarshal(resp.Value)
		assert.True(t, resp.IsOK())
		assert.Equal(t, acct.Balance, acc.Balance)
	}

	{
		key := append([]byte(pstate.AccountPath), fromaddr...)
		resp := app.Query(state, tmtypes.RequestQuery{
			Path: "/bad",
			Data: key,
		})
		assert.False(t, resp.IsOK())
		assert.Empty(t, resp.Proof)
	}

	{
		key := []byte("/bad")
		resp := app.Query(state, tmtypes.RequestQuery{
			Path: store.QueryPath,
			Data: key,
		})
		assert.True(t, resp.IsOK())
		assert.Empty(t, resp.Value)
		assert.Empty(t, resp.Proof)
	}

	{
		resp := app.CheckTx(nil, nil, nil)
		assert.False(t, resp.IsOK())
		assert.Equal(t, code.UNKNOWN_TRANSACTION, resp.Code)
	}

	{
		resp := app.DeliverTx(nil, nil, nil)
		assert.False(t, resp.IsOK())
		assert.Equal(t, code.UNKNOWN_TRANSACTION, resp.Code)
	}

}
