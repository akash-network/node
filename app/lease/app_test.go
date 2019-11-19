package lease_test

import (
	"fmt"
	"testing"

	app_ "github.com/ovrclk/akash/app/lease"
	apptypes "github.com/ovrclk/akash/app/types"
	state_ "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, cacheState)
	address := account.Address

	app, err := app_.NewApp(testutil.Logger())
	require.NoError(t, err)

	{
		path := fmt.Sprintf("%v%x", state_.LeasePath, address)
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}

	{
		path := fmt.Sprintf("%v%x", "/foo/", address)
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}
}

func TestTx_BadTxType(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)
	app, err := app_.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	tx := testutil.ProviderTx(account, key, 10)
	ctx := apptypes.NewContext(tx)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(cacheState, ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(cacheState, ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}
