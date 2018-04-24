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
	state := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, state)
	address := account.Address

	app, err := app_.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	{
		path := query.FulfillmentPath(testutil.DeploymentAddress(t), 0, 0, testutil.Address(t))
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}

	{
		path := fmt.Sprintf("%v%x", "/foo/", address)
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}
}

func TestValidTx(t *testing.T) {

	state := testutil.NewState(t, nil)
	app, err := app_.NewApp(state, testutil.Logger())

	// create provider
	papp, err := papp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	paccount, pkey := testutil.CreateAccount(t, state)
	pnonce := uint64(1)
	provider := testutil.CreateProvider(t, papp, paccount, pkey, pnonce)

	// create tenant
	taccount, tkey := testutil.CreateAccount(t, state)

	// create deployment
	dapp, err := dapp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	tnonce := uint64(1)
	deployment, groups := testutil.CreateDeployment(t, dapp, taccount, tkey, tnonce)
	groupSeq := groups.GetItems()[0].Seq
	daddress := state_.DeploymentAddress(taccount.Address, tnonce)

	// create order
	oapp, err := oapp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	oSeq := uint64(0)
	testutil.CreateOrder(t, oapp, taccount, tkey, deployment.Address, groupSeq, oSeq)
	price := uint32(0)

	fulfillment := createFulfillment(t, app, provider, pkey, daddress, groupSeq, oSeq, price)
	closeFulfillment(t, app, pkey, fulfillment)
}

func createFulfillment(t *testing.T, app apptypes.Application, provider *types.Provider,
	pkey crypto.PrivKey, deployment []byte, groupSeq uint64, oSeq uint64, price uint32) *types.Fulfillment {
	// create fulfillment
	fulfillment := testutil.CreateFulfillment(t, app, provider.Address, pkey, deployment, groupSeq, oSeq, price)

	path := query.FulfillmentPath(fulfillment.Deployment, fulfillment.Group, fulfillment.Order, fulfillment.Provider)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
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

func closeFulfillment(t *testing.T, app apptypes.Application, key crypto.PrivKey, fulfillment *types.Fulfillment) {
	testutil.CloseFulfillment(t, app, key, fulfillment)
	path := query.FulfillmentPath(fulfillment.Deployment, fulfillment.Group, fulfillment.Order, fulfillment.Provider)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())
	ful := new(types.Fulfillment)
	require.NoError(t, ful.Unmarshal(resp.Value))

	assert.Equal(t, types.Fulfillment_CLOSED, ful.State)
}

func TestTx_BadTxType(t *testing.T) {
	state_ := testutil.NewState(t, nil)
	app, err := app_.NewApp(state_, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state_)
	tx := testutil.ProviderTx(account, key, 10)
	ctx := apptypes.NewContext(tx)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}
