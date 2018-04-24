package deployment_test

import (
	"fmt"
	"testing"

	"github.com/ovrclk/akash/app/deployment"
	"github.com/ovrclk/akash/app/fulfillment"
	"github.com/ovrclk/akash/app/lease"
	"github.com/ovrclk/akash/app/order"
	"github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/query"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	state := testutil.NewState(t, nil)

	address := testutil.DeploymentAddress(t)

	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	{
		path := query.DeploymentPath(address)
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}

	{
		path := fmt.Sprintf("%v%x", "/foo/", address)
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}
}

func TestCreateTx(t *testing.T) {
	const groupseq = 1
	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl, groups := testutil.CreateDeployment(t, app, account, key, nonce)

	{
		path := query.DeploymentPath(depl.Address)
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, depl.Tenant, dep.Tenant)
		assert.Equal(t, depl.Address, dep.Address)
	}

	{
		path := query.DeploymentGroupPath(depl.Address, groupseq)
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		grps := new(types.DeploymentGroup)
		require.NoError(t, grps.Unmarshal(resp.Value))

		assert.Equal(t, grps.Requirements, groups.GetItems()[0].Requirements)
		assert.Equal(t, grps.Resources, groups.GetItems()[0].Resources)
	}

	{
		path := pstate.DeploymentPath
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		path := fmt.Sprintf("%v%x",
			pstate.DeploymentPath,
			pstate.DeploymentGroupID(depl.Address, 1))
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		path := query.DeploymentGroupPath(depl.Address, 1)
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		path := query.DeploymentGroupPath(depl.Address, 0)
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		grps, err := state.DeploymentGroup().ForDeployment(depl.Address)
		require.NoError(t, err)
		require.Len(t, grps, 1)

		assert.Equal(t, grps[0].Requirements, groups.GetItems()[0].Requirements)
		assert.Equal(t, grps[0].Resources, groups.GetItems()[0].Resources)
	}
}

func TestTx_BadTxType(t *testing.T) {
	state_ := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state_, testutil.Logger())
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

func TestCloseTx_1(t *testing.T) {
	const gseq = 1
	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl, _ := testutil.CreateDeployment(t, app, account, key, nonce)

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState) {
		assertDeploymentState(t, app, depl.Address, dstate)
		assertDeploymentGroupState(t, app, depl.Address, gseq, gstate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN)

	testutil.CloseDeployment(t, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED)
}

func TestCloseTx_2(t *testing.T) {

	const (
		gseq = 1
		oseq = 3
	)

	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl, _ := testutil.CreateDeployment(t, app, account, key, nonce)

	oapp, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	testutil.CreateOrder(t, oapp, account, key, depl.Address, gseq, oseq)

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState,
		ostate types.Order_OrderState) {
		assertDeploymentState(t, app, depl.Address, dstate)
		assertDeploymentGroupState(t, app, depl.Address, gseq, gstate)
		assertOrderState(t, oapp, depl.Address, gseq, oseq, ostate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN, types.Order_OPEN)

	testutil.CloseDeployment(t, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED, types.Order_CLOSED)
}

func TestCloseTx_3(t *testing.T) {

	const (
		gseq  = 1
		oseq  = 3
		price = 0
	)

	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)
	depl, _ := testutil.CreateDeployment(t, app, account, key, nonce)

	orderapp, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	testutil.CreateOrder(t, orderapp, account, key, depl.Address, gseq, oseq)

	providerapp, err := provider.NewApp(state, testutil.Logger())
	prov := testutil.CreateProvider(t, providerapp, account, key, nonce)

	fulfillmentapp, err := fulfillment.NewApp(state, testutil.Logger())
	testutil.CreateFulfillment(t, fulfillmentapp, prov.Address, key, depl.Address, gseq, oseq, price)

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState,
		ostate types.Order_OrderState,
		fstate types.Fulfillment_FulfillmentState) {
		assertDeploymentState(t, app, depl.Address, dstate)
		assertDeploymentGroupState(t, app, depl.Address, gseq, gstate)
		assertOrderState(t, orderapp, depl.Address, gseq, oseq, ostate)
		assertFulfillmentState(t, fulfillmentapp, depl.Address, gseq, oseq, prov.Address, fstate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN, types.Order_OPEN, types.Fulfillment_OPEN)

	testutil.CloseDeployment(t, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED, types.Order_CLOSED, types.Fulfillment_CLOSED)
}

func TestCloseTx_4(t *testing.T) {

	const (
		gseq  = 1
		oseq  = 3
		price = 0
	)

	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)
	depl, _ := testutil.CreateDeployment(t, app, account, key, nonce)

	orderapp, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	testutil.CreateOrder(t, orderapp, account, key, depl.Address, gseq, oseq)

	providerapp, err := provider.NewApp(state, testutil.Logger())
	prov := testutil.CreateProvider(t, providerapp, account, key, nonce)

	fulfillmentapp, err := fulfillment.NewApp(state, testutil.Logger())
	testutil.CreateFulfillment(t, fulfillmentapp, prov.Address, key, depl.Address, gseq, oseq, price)

	leaseapp, err := lease.NewApp(state, testutil.Logger())
	testutil.CreateLease(t, leaseapp, prov.Address, key, depl.Address, gseq, oseq, price)

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState,
		ostate types.Order_OrderState,
		fstate types.Fulfillment_FulfillmentState,
		lstate types.Lease_LeaseState) {
		assertDeploymentState(t, app, depl.Address, dstate)
		assertDeploymentGroupState(t, app, depl.Address, gseq, gstate)
		assertOrderState(t, orderapp, depl.Address, gseq, oseq, ostate)
		assertFulfillmentState(t, fulfillmentapp, depl.Address, gseq, oseq, prov.Address, fstate)
		assertLeaseState(t, leaseapp, depl.Address, gseq, oseq, prov.Address, lstate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN, types.Order_MATCHED, types.Fulfillment_OPEN, types.Lease_ACTIVE)

	testutil.CloseDeployment(t, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED, types.Order_CLOSED, types.Fulfillment_CLOSED, types.Lease_CLOSED)
}

// check deployment and group query & status
func assertDeploymentState(
	t *testing.T,
	app apptypes.Application,
	daddr []byte,
	dstate types.Deployment_DeploymentState) {

	path := query.DeploymentPath(daddr)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	dep := new(types.Deployment)
	require.NoError(t, dep.Unmarshal(resp.Value))

	assert.Equal(t, dstate, dep.State)
}

// check deployment and group query & status
func assertDeploymentGroupState(
	t *testing.T,
	app apptypes.Application,
	daddr []byte,
	gseq uint64,
	gstate types.DeploymentGroup_DeploymentGroupState) {

	path := query.DeploymentGroupPath(daddr, gseq)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	group := new(types.DeploymentGroup)
	require.NoError(t, group.Unmarshal(resp.Value))

	assert.Equal(t, gstate, group.State)
}

func assertOrderState(
	t *testing.T,
	app apptypes.Application,
	daddr []byte,
	gseq uint64,
	oseq uint64,
	ostate types.Order_OrderState) {

	path := query.OrderPath(daddr, gseq, oseq)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	order := new(types.Order)
	require.NoError(t, order.Unmarshal(resp.Value))
	assert.Equal(t, ostate, order.State)
}

func assertFulfillmentState(
	t *testing.T,
	app apptypes.Application,
	daddr []byte,
	gseq uint64,
	oseq uint64,
	paddr []byte,
	state types.Fulfillment_FulfillmentState) {

	path := query.FulfillmentPath(daddr, gseq, oseq, paddr)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	obj := new(types.Fulfillment)
	require.NoError(t, obj.Unmarshal(resp.Value))
	assert.Equal(t, state, obj.State)
}

func assertLeaseState(
	t *testing.T,
	app apptypes.Application,
	daddr []byte,
	gseq uint64,
	oseq uint64,
	paddr []byte,
	state types.Lease_LeaseState) {

	// check fulfillment state
	path := query.LeasePath(daddr, gseq, oseq, paddr)
	resp := app.Query(tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	obj := new(types.Lease)
	require.NoError(t, obj.Unmarshal(resp.Value))
	assert.Equal(t, state, obj.State)
}
