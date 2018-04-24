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
	tmtypes "github.com/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	state := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, state)
	address := account.Address

	app, err := app_.NewApp(state, testutil.Logger())
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
	testutil.CreateDeployment(t, dapp, taccount, tkey, tnonce)
	groupSeq := uint64(1)
	daddress := state_.DeploymentAddress(taccount.Address, tnonce)

	// create order
	oapp, err := oapp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	oSeq := uint64(0)
	testutil.CreateOrder(t, oapp, taccount, tkey, daddress, groupSeq, oSeq)
	price := uint32(0)

	// create fulfillment
	fapp, err := fapp.NewApp(state, testutil.Logger())
	testutil.CreateFulfillment(t, fapp, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	// create lease
	lease := testutil.CreateLease(t, app, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	{
		path := query.LeasePath(lease.Deployment, lease.Group, lease.Order, lease.Provider)
		resp := app.Query(tmtypes.RequestQuery{Path: path})
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
	leaseAddr := state.Lease().IDFor(lease)
	testutil.CloseLease(t, app, leaseAddr, pkey)
	{
		path := query.LeasePath(lease.Deployment, lease.Group, lease.Order, lease.Provider)
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
		lea := new(types.Lease)
		require.NoError(t, lea.Unmarshal(resp.Value))
		assert.Equal(t, types.Lease_CLOSED, lea.State)
	}
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

func TestBilling(t *testing.T) {

	state := testutil.NewState(t, nil)
	app, err := app_.NewApp(state, testutil.Logger())

	// create provider
	papp, err := papp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	paccount, pkey := testutil.CreateAccount(t, state)
	pnonce := uint64(1)
	provider := testutil.CreateProvider(t, papp, paccount, pkey, pnonce)

	// create tenant
	tenant, tkey := testutil.CreateAccount(t, state)

	// create deployment
	dapp, err := dapp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	tnonce := uint64(1)
	testutil.CreateDeployment(t, dapp, tenant, tkey, tnonce)
	groupSeq := uint64(1)
	daddress := state_.DeploymentAddress(tenant.Address, tnonce)

	// create order
	oapp, err := oapp.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	oSeq := uint64(0)
	testutil.CreateOrder(t, oapp, tenant, tkey, daddress, groupSeq, oSeq)
	price := uint32(1)
	p := uint64(price)

	// create fulfillment
	fapp, err := fapp.NewApp(state, testutil.Logger())
	testutil.CreateFulfillment(t, fapp, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	// create lease
	testutil.CreateLease(t, app, provider.Address, pkey, daddress, groupSeq, oSeq, price)

	iTenBal := getBalance(t, state, tenant.Address)
	iProBal := getBalance(t, state, provider.Owner)
	require.NotZero(t, iTenBal)
	require.NotZero(t, iProBal)

	app_.ProcessLeases(state)

	fTenBal := getBalance(t, state, tenant.Address)
	fProBal := getBalance(t, state, provider.Owner)
	require.Equal(t, iTenBal-p, fTenBal)
	require.Equal(t, iProBal+p, fProBal)
}

func getBalance(t *testing.T, state state_.State, address base.Bytes) uint64 {
	acc, err := state.Account().Get(address)
	require.NoError(t, err)
	return acc.GetBalance()
}
