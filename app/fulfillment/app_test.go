package fulfillment_test

import (
	"fmt"
	"testing"

	dapp "github.com/ovrclk/akash/app/deployment"
	app_ "github.com/ovrclk/akash/app/fulfillment"
	oapp "github.com/ovrclk/akash/app/order"
	papp "github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/query"
	state_ "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
)

func TestAcceptQuery(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, cacheState)
	address := account.Address

	app, err := app_.NewApp(testutil.Logger())
	require.NoError(t, err)

	{
		id := types.FulfillmentID{
			Deployment: testutil.DeploymentAddress(t),
			Group:      0,
			Order:      0,
			Provider:   testutil.Address(t),
		}
		path := query.FulfillmentPath(id)
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}

	{
		path := fmt.Sprintf("%v%x", "/foo/", address)
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}
}

func TestValidTx(t *testing.T) {

	_, cacheState := testutil.NewState(t, nil)
	app, err := app_.NewApp(testutil.Logger())

	// create provider
	papp, err := papp.NewApp(testutil.Logger())
	require.NoError(t, err)
	paccount, pkey := testutil.CreateAccount(t, cacheState)
	pnonce := uint64(1)
	provider := testutil.CreateProvider(t, cacheState, papp, paccount, pkey, pnonce)

	// create tenant
	taccount, tkey := testutil.CreateAccount(t, cacheState)

	// create deployment
	dapp, err := dapp.NewApp(testutil.Logger())
	require.NoError(t, err)
	tnonce := uint64(1)
	deployment, groups := testutil.CreateDeployment(t, cacheState, dapp, taccount, tkey, tnonce)
	groupSeq := groups.GetItems()[0].Seq
	daddress := state_.DeploymentAddress(taccount.Address, tnonce)

	// create order
	oapp, err := oapp.NewApp(testutil.Logger())
	require.NoError(t, err)
	oSeq := uint64(0)
	testutil.CreateOrder(t, cacheState, oapp, taccount, tkey, deployment.Address, groupSeq, oSeq)
	price := uint32(0)

	fulfillment := createFulfillment(t, cacheState, app, provider, pkey, daddress, groupSeq, oSeq, price)
	closeFulfillment(t, cacheState, app, pkey, fulfillment)
}

func createFulfillment(t *testing.T, state state_.State, app apptypes.Application, provider *types.Provider,
	pkey crypto.PrivKey, deployment []byte, groupSeq uint64, oSeq uint64, price uint32) *types.Fulfillment {
	// create fulfillment
	fulfillment := testutil.CreateFulfillment(t, state, app, provider.Address, pkey, deployment, groupSeq, oSeq, price)

	path := query.FulfillmentPath(fulfillment.FulfillmentID)
	resp := app.Query(state, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())
	ful := new(types.Fulfillment)
	require.NoError(t, ful.Unmarshal(resp.Value))

	assert.Equal(t, fulfillment.Deployment, ful.Deployment)
	assert.Equal(t, fulfillment.Group, ful.Group)
	assert.Equal(t, fulfillment.Order, ful.Order)
	assert.Equal(t, fulfillment.Provider, ful.Provider)
	assert.Equal(t, fulfillment.Price, ful.Price)
	assert.Equal(t, fulfillment.State, ful.State)

	return fulfillment
}

func closeFulfillment(t *testing.T, state state_.State, app apptypes.Application, key crypto.PrivKey, fulfillment *types.Fulfillment) {
	testutil.CloseFulfillment(t, state, app, key, fulfillment)
	path := query.FulfillmentPath(fulfillment.FulfillmentID)
	resp := app.Query(state, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())
	ful := new(types.Fulfillment)
	require.NoError(t, ful.Unmarshal(resp.Value))

	assert.Equal(t, types.Fulfillment_CLOSED, ful.State)
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
