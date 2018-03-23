package deployment_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/akash/app/deployment"
	"github.com/ovrclk/akash/app/fulfillment"
	"github.com/ovrclk/akash/app/lease"
	"github.com/ovrclk/akash/app/order"
	"github.com/ovrclk/akash/app/provider"
	apptypes "github.com/ovrclk/akash/app/types"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestAcceptQuery(t *testing.T) {
	state := testutil.NewState(t, nil)

	account, _ := testutil.CreateAccount(t, state)
	address := account.Address

	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	{
		path := fmt.Sprintf("%v%X", pstate.DeploymentPath, address)
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: path}))
	}

	{
		path := fmt.Sprintf("%v%X", "/foo/", address)
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

	depl, groups := testutil.CreateDeployment(t, app, account, &key, nonce)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, depl.Tenant, dep.Tenant)
		assert.Equal(t, depl.Address, dep.Address)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		grps := new(types.DeploymentGroup)
		require.NoError(t, grps.Unmarshal(resp.Value))

		assert.Equal(t, grps.Requirements, groups.GetItems()[0].Requirements)
		assert.Equal(t, grps.Resources, groups.GetItems()[0].Resources)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: pstate.DeploymentPath})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: pstate.DeploymentPath + hex.EncodeToString(pstate.DeploymentGroupID(depl.Address, 1))})
		assert.NotEmpty(t, resp.Log)
		require.False(t, resp.IsOK())
	}

	{
		path := pstate.DeploymentGroupPath + hex.EncodeToString(pstate.DeploymentGroupID(depl.Address, 1))
		resp := app.Query(tmtypes.RequestQuery{Path: path})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())
	}

	{
		path := pstate.DeploymentGroupPath + hex.EncodeToString(pstate.DeploymentGroupID(depl.Address, 0))
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
	tx := testutil.ProviderTx(account, &key, 10)
	ctx := apptypes.NewContext(tx)
	assert.False(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(ctx, tx.Payload.Payload)
	assert.False(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, tx.Payload.Payload)
	assert.False(t, dresp.IsOK())
}

func TestCloseTx_1(t *testing.T) {
	const groupseq = 1
	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl, groups := testutil.CreateDeployment(t, app, account, &key, nonce)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		println("resp", resp.String())
		println("\ndep\n", dep.String())

		assert.Equal(t, types.Deployment_ACTIVE, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, groups.GetItems()[0].State)
		assert.Equal(t, group.State, types.DeploymentGroup_OPEN)
	}

	testutil.CloseDeployment(t, app, &depl.Address, &key)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSING, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSING)
	}

	testutil.DeploymentClosed(t, app, &depl.Address, &key)

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSED, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSED)
	}
}

func TestCloseTx_2(t *testing.T) {

	const (
		groupseq = 1
		orderseq = 3
	)

	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	depl, groups := testutil.CreateDeployment(t, app, account, &key, nonce)

	orderapp, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	testutil.CreateOrder(t, orderapp, account, &key, depl.Address, groupseq, orderseq)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, dep.State, depl.State)
		assert.Equal(t, dep.State, types.Deployment_ACTIVE)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, groups.GetItems()[0].State)
		assert.Equal(t, group.State, types.DeploymentGroup_OPEN)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_OPEN, o.State)
	}

	testutil.CloseDeployment(t, app, &depl.Address, &key)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSING, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSING)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_CLOSING, o.State)
	}

	testutil.DeploymentClosed(t, app, &depl.Address, &key)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSED, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSED)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_CLOSED, o.State)
	}
}

func TestCloseTx_3(t *testing.T) {

	const (
		groupseq = 1
		orderseq = 3
		price    = 0
	)

	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)
	depl, groups := testutil.CreateDeployment(t, app, account, &key, nonce)

	orderapp, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	testutil.CreateOrder(t, orderapp, account, &key, depl.Address, groupseq, orderseq)

	providerapp, err := provider.NewApp(state, testutil.Logger())
	prov := testutil.CreateProvider(t, providerapp, account, &key, nonce)

	fulfillmentapp, err := fulfillment.NewApp(state, testutil.Logger())
	testutil.CreateFulfillment(t, fulfillmentapp, prov.Address, &key, depl.Address, groupseq, orderseq, price)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, dep.State, depl.State)
		assert.Equal(t, dep.State, types.Deployment_ACTIVE)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, groups.GetItems()[0].State)
		assert.Equal(t, group.State, types.DeploymentGroup_OPEN)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_OPEN, o.State)
	}

	{
		// check fulfillment state
		resp := fulfillmentapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.FulfillmentPath, pstate.FulfillmentID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Fulfillment)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Fulfillment_OPEN, o.State)
	}

	testutil.CloseDeployment(t, app, &depl.Address, &key)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSING, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSING)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_CLOSING, o.State)
	}

	{
		// check fulfillment state
		resp := fulfillmentapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.FulfillmentPath, pstate.FulfillmentID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Fulfillment)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Fulfillment_CLOSING, o.State)
	}

	testutil.DeploymentClosed(t, app, &depl.Address, &key)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSED, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSED)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_CLOSED, o.State)
	}

	{
		// check fulfillment state
		resp := fulfillmentapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.FulfillmentPath, pstate.FulfillmentID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Fulfillment)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Fulfillment_CLOSED, o.State)
	}
}

func TestCloseTx_4(t *testing.T) {

	const (
		groupseq = 1
		orderseq = 3
		price    = 0
	)

	state := testutil.NewState(t, nil)
	app, err := deployment.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)
	depl, groups := testutil.CreateDeployment(t, app, account, &key, nonce)

	orderapp, err := order.NewApp(state, testutil.Logger())
	require.NoError(t, err)
	testutil.CreateOrder(t, orderapp, account, &key, depl.Address, groupseq, orderseq)

	providerapp, err := provider.NewApp(state, testutil.Logger())
	prov := testutil.CreateProvider(t, providerapp, account, &key, nonce)

	fulfillmentapp, err := fulfillment.NewApp(state, testutil.Logger())
	testutil.CreateFulfillment(t, fulfillmentapp, prov.Address, &key, depl.Address, groupseq, orderseq, price)

	leaseapp, err := lease.NewApp(state, testutil.Logger())
	testutil.CreateLease(t, leaseapp, prov.Address, &key, depl.Address, groupseq, orderseq, price)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, dep.State, depl.State)
		assert.Equal(t, dep.State, types.Deployment_ACTIVE)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, groups.GetItems()[0].State)
		assert.Equal(t, group.State, types.DeploymentGroup_OPEN)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_MATCHED, o.State)
	}

	{
		// check fulfillment state
		resp := fulfillmentapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.FulfillmentPath, pstate.FulfillmentID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Fulfillment)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Fulfillment_OPEN, o.State)
	}

	{
		// check lease state
		resp := leaseapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.LeasePath, pstate.LeaseID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Lease)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Lease_ACTIVE, o.State)
	}

	testutil.CloseDeployment(t, app, &depl.Address, &key)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSING, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSING)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_CLOSING, o.State)
	}

	{
		// check fulfillment state
		resp := fulfillmentapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.FulfillmentPath, pstate.FulfillmentID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Fulfillment)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Fulfillment_CLOSING, o.State)
	}

	{
		// check lease state
		resp := leaseapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.LeasePath, pstate.LeaseID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Lease)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Lease_CLOSING, o.State)
	}

	testutil.DeploymentClosed(t, app, &depl.Address, &key)

	{
		// check deployment state
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentPath, depl.Address)})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		dep := new(types.Deployment)
		require.NoError(t, dep.Unmarshal(resp.Value))

		assert.Equal(t, types.Deployment_CLOSED, dep.State)
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.DeploymentGroupPath, pstate.DeploymentGroupID(depl.Address, groupseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		group := new(types.DeploymentGroup)
		require.NoError(t, group.Unmarshal(resp.Value))

		assert.Equal(t, group.State, types.DeploymentGroup_CLOSED)
	}

	{
		// check order state
		resp := orderapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.OrderPath, pstate.OrderID(depl.Address, groupseq, orderseq))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Order)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Order_CLOSED, o.State)
	}

	{
		// check fulfillment state
		resp := fulfillmentapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.FulfillmentPath, pstate.FulfillmentID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Fulfillment)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Fulfillment_CLOSED, o.State)
	}

	{
		// check lease state
		resp := leaseapp.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%X", pstate.LeasePath, pstate.LeaseID(depl.Address, groupseq, orderseq, prov.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		o := new(types.Lease)
		require.NoError(t, o.Unmarshal(resp.Value))

		assert.Equal(t, types.Lease_CLOSED, o.State)
	}
}
