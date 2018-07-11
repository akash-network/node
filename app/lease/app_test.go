package lease_test

import (
	"fmt"
	"testing"

	dapp "github.com/ovrclk/akash/app/deployment"
	fapp "github.com/ovrclk/akash/app/fulfillment"
	app_ "github.com/ovrclk/akash/app/lease"
	oapp "github.com/ovrclk/akash/app/order"
	papp "github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/query"
	state_ "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
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
	testutil.CreateDeployment(t, cacheState, dapp, taccount, tkey, tnonce)
	groupSeq := uint64(1)
	daddress := state_.DeploymentAddress(taccount.Address, tnonce)

	// create order
	oapp, err := oapp.NewApp(testutil.Logger())
	require.NoError(t, err)
	oSeq := uint64(0)
	testutil.CreateOrder(t, cacheState, oapp, taccount, tkey, daddress, groupSeq, oSeq)
	price := uint64(1)

	// create fulfillment
	fapp, err := fapp.NewApp(testutil.Logger())
	testutil.CreateFulfillment(t, cacheState, fapp, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	// create lease
	lease := testutil.CreateLease(t, cacheState, app, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	{
		path := query.LeasePath(lease.LeaseID)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
		lea := new(types.Lease)
		require.NoError(t, lea.Unmarshal(resp.Value))
		assert.Equal(t, lease.Deployment, lea.Deployment)
		assert.Equal(t, lease.Group, lea.Group)
		assert.Equal(t, lease.Order, lea.Order)
		assert.Equal(t, lease.Provider, lea.Provider)
		assert.Equal(t, lease.Price, lea.Price)
		assert.Equal(t, types.Lease_ACTIVE, lea.State)
	}

	// close lease
	testutil.CloseLease(t, cacheState, app, lease.LeaseID, pkey)
	{
		path := query.LeasePath(lease.LeaseID)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
		lea := new(types.Lease)
		require.NoError(t, lea.Unmarshal(resp.Value))
		assert.Equal(t, types.Lease_CLOSED, lea.State)
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

func TestBilling(t *testing.T) {

	_, cacheState := testutil.NewState(t, nil)
	app, err := app_.NewApp(testutil.Logger())

	// create provider
	papp, err := papp.NewApp(testutil.Logger())
	require.NoError(t, err)
	paccount, pkey := testutil.CreateAccount(t, cacheState)
	pnonce := uint64(1)
	provider := testutil.CreateProvider(t, cacheState, papp, paccount, pkey, pnonce)

	// create tenant
	tenant, tkey := testutil.CreateAccount(t, cacheState)

	// create deployment
	dapp, err := dapp.NewApp(testutil.Logger())
	require.NoError(t, err)
	tnonce := uint64(1)
	testutil.CreateDeployment(t, cacheState, dapp, tenant, tkey, tnonce)
	groupSeq := uint64(1)
	daddress := state_.DeploymentAddress(tenant.Address, tnonce)

	// create order
	oapp, err := oapp.NewApp(testutil.Logger())
	require.NoError(t, err)
	oSeq := uint64(0)
	testutil.CreateOrder(t, cacheState, oapp, tenant, tkey, daddress, groupSeq, oSeq)
	price := uint64(1)
	p := uint64(price)

	// create fulfillment
	fapp, err := fapp.NewApp(testutil.Logger())
	testutil.CreateFulfillment(t, cacheState, fapp, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	// create lease
	testutil.CreateLease(t, cacheState, app, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	iTenBal := getBalance(t, cacheState, tenant.Address)
	iProBal := getBalance(t, cacheState, provider.Owner)
	require.NotZero(t, iTenBal)
	require.NotZero(t, iProBal)

	err = app_.ProcessLeases(cacheState)
	require.NoError(t, err)

	fTenBal := getBalance(t, cacheState, tenant.Address)
	fProBal := getBalance(t, cacheState, provider.Owner)
	require.Equal(t, iTenBal-p, fTenBal)
	require.Equal(t, iProBal+p, fProBal)
}

func getBalance(t *testing.T, state state_.State, address base.Bytes) uint64 {
	acc, err := state.Account().Get(address)
	require.NoError(t, err)
	return acc.GetBalance()
}
