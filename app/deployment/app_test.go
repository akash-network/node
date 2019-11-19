package deployment_test

import (
	"fmt"
	"testing"

	"github.com/ovrclk/akash/app/deployment"
	"github.com/ovrclk/akash/app/fulfillment"
	"github.com/ovrclk/akash/app/order"
	"github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/query"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	address := testutil.DeploymentAddress(t)

	app, err := deployment.NewApp(testutil.Logger())
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
	_, cacheState := testutil.NewState(t, nil)
	app, err := deployment.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	nonce := uint64(1)

	depl, groups := testutil.CreateDeployment(t, cacheState, app, account, key, nonce)

	{
		path := query.DeploymentPath(depl.Address)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, depl.Tenant, dep.Tenant)
		assert.Equal(t, depl.Address, dep.Address)
		assert.Equal(t, depl.Version, dep.Version)
	}

	{
		path := query.DeploymentGroupPath(groups.Items[0].DeploymentGroupID)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		grps := new(types.DeploymentGroup)
		require.NoError(t, grps.Unmarshal(resp.Value))

		assert.Equal(t, grps.Requirements, groups.GetItems()[0].Requirements)
		assert.Equal(t, grps.Resources, groups.GetItems()[0].Resources)
	}

	{
		path := pstate.DeploymentPath
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	badgroup := types.DeploymentGroupID{
		Deployment: depl.Address,
		Seq:        2,
	}

	goodgroup := groups.GetItems()[0].DeploymentGroupID

	{
		path := fmt.Sprintf("%v%v", pstate.DeploymentPath, badgroup)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		path := query.DeploymentGroupPath(goodgroup)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		path := query.DeploymentGroupPath(badgroup)
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		grps, err := cacheState.DeploymentGroup().ForDeployment(depl.Address)
		require.NoError(t, err)
		require.Len(t, grps, 1)

		assert.Equal(t, grps[0].Requirements, groups.GetItems()[0].Requirements)
		assert.Equal(t, grps[0].Resources, groups.GetItems()[0].Resources)
	}
}

func TestUpdateTx(t *testing.T) {
	const groupseq = 1
	_, cacheState := testutil.NewState(t, nil)
	app, err := deployment.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	nonce := uint64(1)

	depl, _ := testutil.CreateDeployment(t, cacheState, app, account, key, nonce)
	nonce++

	path := query.DeploymentPath(depl.Address)

	{
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, depl.Tenant, dep.Tenant)
		assert.Equal(t, depl.Address, dep.Address)
		assert.Equal(t, depl.Version, dep.Version)
	}

	itx := testutil.UpdateDeployment(t, cacheState, app, key, nonce, depl.Address)

	{
		resp := app.Query(cacheState, tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, depl.Tenant, dep.Tenant)
		assert.Equal(t, depl.Address, dep.Address)
		assert.Equal(t, itx.Version, dep.Version)
	}
}

func TestTx_BadTxType(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)
	app, err := deployment.NewApp(testutil.Logger())
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

func TestCloseTx_1(t *testing.T) {
	_, cacheState := testutil.NewState(t, nil)
	app, err := deployment.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	nonce := uint64(1)

	depl, groups := testutil.CreateDeployment(t, cacheState, app, account, key, nonce)

	group := groups.Items[0]

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState) {
		assertDeploymentState(t, cacheState, app, depl.Address, dstate)
		assertDeploymentGroupState(t, cacheState, app, group.DeploymentGroupID, gstate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN)

	testutil.CloseDeployment(t, cacheState, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED)
}

func TestCloseTx_2(t *testing.T) {

	const (
		oseq = 3
	)

	_, cacheState := testutil.NewState(t, nil)
	app, err := deployment.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	nonce := uint64(1)

	depl, groups := testutil.CreateDeployment(t, cacheState, app, account, key, nonce)
	group := groups.Items[0]

	oapp, err := order.NewApp(testutil.Logger())
	require.NoError(t, err)

	order := testutil.CreateOrder(t, cacheState, oapp, account, key, depl.Address, group.Seq, oseq)

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState,
		ostate types.Order_OrderState) {
		assertDeploymentState(t, cacheState, app, depl.Address, dstate)
		assertDeploymentGroupState(t, cacheState, app, order.GroupID(), gstate)
		assertOrderState(t, cacheState, oapp, order.OrderID, ostate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN, types.Order_OPEN)

	testutil.CloseDeployment(t, cacheState, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED, types.Order_CLOSED)
}

func TestCloseTx_3(t *testing.T) {

	const (
		oseq  = 3
		price = 1
	)

	_, cacheState := testutil.NewState(t, nil)
	app, err := deployment.NewApp(testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, cacheState)
	nonce := uint64(1)
	depl, groups := testutil.CreateDeployment(t, cacheState, app, account, key, nonce)
	group := groups.Items[0]

	orderapp, err := order.NewApp(testutil.Logger())
	require.NoError(t, err)
	order := testutil.CreateOrder(t, cacheState, orderapp, account, key, depl.Address, group.Seq, oseq)

	providerapp, err := provider.NewApp(testutil.Logger())
	require.NoError(t, err)
	prov := testutil.CreateProvider(t, cacheState, providerapp, account, key, nonce)

	fulfillmentapp, err := fulfillment.NewApp(testutil.Logger())
	require.NoError(t, err)
	fulfillment := testutil.CreateFulfillment(t, cacheState, fulfillmentapp, prov.Address, key, depl.Address, group.Seq, order.Seq, price)

	check := func(
		dstate types.Deployment_DeploymentState,
		gstate types.DeploymentGroup_DeploymentGroupState,
		ostate types.Order_OrderState,
		fstate types.Fulfillment_FulfillmentState) {
		assertDeploymentState(t, cacheState, app, depl.Address, dstate)
		assertDeploymentGroupState(t, cacheState, app, group.DeploymentGroupID, gstate)
		assertOrderState(t, cacheState, orderapp, order.OrderID, ostate)
		assertFulfillmentState(t, cacheState, fulfillmentapp, fulfillment.FulfillmentID, fstate)
	}

	check(types.Deployment_ACTIVE, types.DeploymentGroup_OPEN, types.Order_OPEN, types.Fulfillment_OPEN)

	testutil.CloseDeployment(t, cacheState, app, &depl.Address, key)

	check(types.Deployment_CLOSED, types.DeploymentGroup_CLOSED, types.Order_CLOSED, types.Fulfillment_CLOSED)
}

// check deployment and group query & status
func assertDeploymentState(
	t *testing.T,
	state pstate.State,
	app apptypes.Application,
	daddr []byte,
	dstate types.Deployment_DeploymentState) {

	path := query.DeploymentPath(daddr)
	resp := app.Query(state, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	dep := new(types.Deployment)
	require.NoError(t, dep.Unmarshal(resp.Value))

	assert.Equal(t, dstate, dep.State)
}

// check deployment and group query & status
func assertDeploymentGroupState(
	t *testing.T,
	state pstate.State,
	app apptypes.Application,
	id types.DeploymentGroupID,
	gstate types.DeploymentGroup_DeploymentGroupState) {

	path := query.DeploymentGroupPath(id)
	resp := app.Query(state, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	group := new(types.DeploymentGroup)
	require.NoError(t, group.Unmarshal(resp.Value))

	assert.Equal(t, gstate, group.State)
}

func assertOrderState(
	t *testing.T,
	state pstate.State,
	app apptypes.Application,
	id types.OrderID,
	ostate types.Order_OrderState) {

	path := query.OrderPath(id)
	resp := app.Query(state, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	order := new(types.Order)
	require.NoError(t, order.Unmarshal(resp.Value))
	assert.Equal(t, ostate, order.State)
}

func assertFulfillmentState(
	t *testing.T,
	st pstate.State,
	app apptypes.Application,
	id types.FulfillmentID,
	state types.Fulfillment_FulfillmentState) {

	path := query.FulfillmentPath(id)
	resp := app.Query(st, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	obj := new(types.Fulfillment)
	require.NoError(t, obj.Unmarshal(resp.Value))
	assert.Equal(t, state, obj.State)
}

func assertLeaseState(
	t *testing.T,
	st pstate.State,
	app apptypes.Application,
	id types.LeaseID,
	state types.Lease_LeaseState) {

	// check fulfillment state
	path := query.LeasePath(id)
	resp := app.Query(st, tmtypes.RequestQuery{Path: path})
	assert.Empty(t, resp.Log)
	require.True(t, resp.IsOK())

	obj := new(types.Lease)
	require.NoError(t, obj.Unmarshal(resp.Value))
	assert.Equal(t, state, obj.State)
}
