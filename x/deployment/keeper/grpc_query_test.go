package keeper_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	"github.com/akash-network/node/x/deployment/keeper"
	ekeeper "github.com/akash-network/node/x/escrow/keeper"
)

type grpcTestSuite struct {
	t       *testing.T
	app     *app.AkashApp
	ctx     sdk.Context
	keeper  keeper.IKeeper
	ekeeper ekeeper.Keeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &grpcTestSuite{
		t:       t,
		app:     ssuite.App(),
		ctx:     ssuite.Context(),
		keeper:  ssuite.DeploymentKeeper(),
		ekeeper: ssuite.EscrowKeeper(),
	}

	querier := suite.keeper.NewQuerier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = types.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryDeployment(t *testing.T) {
	suite := setupTest(t)

	// creating deployment
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)

	eid := suite.createEscrowAccount(deployment.ID())

	var (
		req           *types.QueryDeploymentRequest
		expDeployment types.QueryDeploymentResponse
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryDeploymentRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &types.QueryDeploymentRequest{ID: types.DeploymentID{}}
			},
			false,
		},
		{
			"deployment not found",
			func() {
				req = &types.QueryDeploymentRequest{ID: types.DeploymentID{
					Owner: testutil.AccAddress(t).String(),
					DSeq:  32,
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryDeploymentRequest{ID: deployment.DeploymentID}
				expDeployment = types.QueryDeploymentResponse{
					Deployment: deployment,
					Groups:     groups,
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Deployment(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expDeployment.Deployment, res.Deployment)
				require.Equal(t, expDeployment.Groups, res.Groups)
				require.Equal(t, eid, res.EscrowAccount.ID)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func TestGRPCQueryDeployments(t *testing.T) {
	suite := setupTest(t)

	// creating deployments with different states
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)
	suite.createEscrowAccount(deployment.ID())

	deployment2, groups2 := suite.createDeployment()
	deployment2.State = types.DeploymentClosed
	err = suite.keeper.Create(suite.ctx, deployment2, groups2)
	require.NoError(t, err)
	suite.createEscrowAccount(deployment2.ID())

	var req *types.QueryDeploymentsRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query deployments without any filters and pagination",
			func() {
				req = &types.QueryDeploymentsRequest{}
			},
			2,
		},
		{
			"query deployments with filters having non existent data",
			func() {
				req = &types.QueryDeploymentsRequest{
					Filters: types.DeploymentFilters{
						DSeq:  37,
						State: types.DeploymentClosed.String(),
					}}
			},
			0,
		},
		{
			"query deployments with state filter",
			func() {
				req = &types.QueryDeploymentsRequest{Filters: types.DeploymentFilters{State: types.DeploymentClosed.String()}}
			},
			1,
		},
		{
			"query deployments with pagination",
			func() {
				req = &types.QueryDeploymentsRequest{Pagination: &sdkquery.PageRequest{Limit: 1}}
			},
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Deployments(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Deployments))
		})
	}
}

type deploymentFilterModifier struct {
	fieldName string
	f         func(leaseID types.DeploymentID, filter types.DeploymentFilters) types.DeploymentFilters
	getField  func(leaseID types.DeploymentID) interface{}
}

func TestGRPCQueryDeploymentsWithFilter(t *testing.T) {
	suite := setupTest(t)

	// creating orders with different states
	depA, _ := createActiveDeployment(t, suite.ctx, suite.keeper)
	depB, _ := createActiveDeployment(t, suite.ctx, suite.keeper)
	depC, _ := createActiveDeployment(t, suite.ctx, suite.keeper)

	suite.createEscrowAccount(depA)
	suite.createEscrowAccount(depB)
	suite.createEscrowAccount(depC)

	deps := []types.DeploymentID{
		depA,
		depB,
		depC,
	}

	modifiers := []deploymentFilterModifier{
		{
			"owner",
			func(depID types.DeploymentID, filter types.DeploymentFilters) types.DeploymentFilters {
				filter.Owner = depID.GetOwner()
				return filter
			},
			func(depID types.DeploymentID) interface{} {
				return depID.Owner
			},
		},
		{
			"dseq",
			func(depID types.DeploymentID, filter types.DeploymentFilters) types.DeploymentFilters {
				filter.DSeq = depID.DSeq
				return filter
			},
			func(depID types.DeploymentID) interface{} {
				return depID.DSeq
			},
		},
	}

	ctx := sdk.WrapSDKContext(suite.ctx)

	for _, depID := range deps {
		for _, m := range modifiers {
			req := &types.QueryDeploymentsRequest{
				Filters: m.f(depID, types.DeploymentFilters{}),
			}

			res, err := suite.queryClient.Deployments(ctx, req)

			require.NoError(t, err, "testing %v", m.fieldName)
			require.NotNil(t, res, "testing %v", m.fieldName)
			// At least 1 result
			assert.GreaterOrEqual(t, len(res.Deployments), 1, "testing %v", m.fieldName)

			for _, dep := range res.Deployments {
				assert.Equal(t, m.getField(depID), m.getField(dep.Deployment.DeploymentID), "testing %v", m.fieldName)
			}
		}
	}

	limit := int(math.Pow(2, float64(len(modifiers))))

	// Use an order ID that matches absolutely nothing in any field
	bogusOrderID := types.DeploymentID{
		Owner: testutil.AccAddress(t).String(),
		DSeq:  9999999,
	}

	for i := 0; i != limit; i++ {
		modifiersToUse := make([]bool, len(modifiers))

		for j := 0; j != len(modifiers); j++ {
			mask := int(math.Pow(2, float64(j)))
			modifiersToUse[j] = (mask & i) != 0
		}

		for _, orderID := range deps {
			filter := types.DeploymentFilters{}
			msg := strings.Builder{}
			msg.WriteString("testing filtering on: ")
			for k, useModifier := range modifiersToUse {
				if !useModifier {
					continue
				}
				modifier := modifiers[k]
				filter = modifier.f(orderID, filter)
				msg.WriteString(modifier.fieldName)
				msg.WriteString(", ")
			}

			req := &types.QueryDeploymentsRequest{
				Filters: filter,
			}

			res, err := suite.queryClient.Deployments(ctx, req)

			require.NoError(t, err, msg.String())
			require.NotNil(t, res, msg.String())
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Deployments), 1, msg.String())

			for _, dep := range res.Deployments {
				for k, useModifier := range modifiersToUse {
					if !useModifier {
						continue
					}
					m := modifiers[k]
					require.Equal(t, m.getField(orderID), m.getField(dep.Deployment.DeploymentID), "testing %v", m.fieldName)
				}
			}
		}

		filter := types.DeploymentFilters{}
		msg := strings.Builder{}
		msg.WriteString("testing filtering on (using non matching ID): ")
		for k, useModifier := range modifiersToUse {
			if !useModifier {
				continue
			}
			modifier := modifiers[k]
			filter = modifier.f(bogusOrderID, filter)
			msg.WriteString(modifier.fieldName)
			msg.WriteString(", ")
		}

		req := &types.QueryDeploymentsRequest{
			Filters: filter,
		}

		res, err := suite.queryClient.Deployments(ctx, req)

		require.NoError(t, err, msg.String())
		require.NotNil(t, res, msg.String())
		expected := 0
		if i == 0 {
			expected = len(deps)
		}
		require.Len(t, res.Deployments, expected, msg.String())
	}

	for _, depID := range deps {
		// Query by owner
		req := &types.QueryDeploymentsRequest{
			Filters: types.DeploymentFilters{
				Owner: depID.Owner,
			},
		}

		res, err := suite.queryClient.Deployments(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// Just 1 result
		require.Len(t, res.Deployments, 1)
		depResult := res.Deployments[0]
		require.Equal(t, depID, depResult.GetDeployment().DeploymentID)

		// Query with valid DSeq
		req = &types.QueryDeploymentsRequest{
			Filters: types.DeploymentFilters{
				Owner: depID.Owner,
				DSeq:  depID.DSeq,
			},
		}

		res, err = suite.queryClient.Deployments(ctx, req)

		// Expect the same match
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Deployments, 1)
		depResult = res.Deployments[0]
		require.Equal(t, depID, depResult.Deployment.DeploymentID)

		// Query with a bogus DSeq
		req = &types.QueryDeploymentsRequest{
			Filters: types.DeploymentFilters{
				Owner: depID.Owner,
				DSeq:  depID.DSeq + 1,
			},
		}

		res, err = suite.queryClient.Deployments(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// expect nothing matches
		require.Len(t, res.Deployments, 0)
	}
}

func TestGRPCQueryGroup(t *testing.T) {
	suite := setupTest(t)

	// creating group
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)

	var (
		req           *types.QueryGroupRequest
		expDeployment types.Group
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryGroupRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &types.QueryGroupRequest{ID: types.GroupID{}}
			},
			false,
		},
		{
			"group not found",
			func() {
				req = &types.QueryGroupRequest{ID: types.GroupID{
					Owner: testutil.AccAddress(t).String(),
					DSeq:  32,
					GSeq:  45,
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryGroupRequest{ID: groups[0].GroupID}
				expDeployment = groups[0]
			},
			true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Group(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expDeployment, res.Group)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func (suite *grpcTestSuite) createDeployment() (types.Deployment, []types.Group) {
	suite.t.Helper()

	deployment := testutil.Deployment(suite.t)
	group := testutil.DeploymentGroup(suite.t, deployment.ID(), 0)
	group.GroupSpec.Resources = []types.Resource{
		{
			Resources: testutil.ResourceUnits(suite.t),
			Count:     1,
			Price:     testutil.DecCoin(suite.t),
		},
	}
	groups := []types.Group{
		group,
	}

	for i := range groups {
		groups[i].State = types.GroupOpen
	}

	return deployment, groups
}

func (suite *grpcTestSuite) createEscrowAccount(id types.DeploymentID) etypes.AccountID {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	require.NoError(suite.t, err)

	eid := types.EscrowAccountForDeployment(id)
	defaultDeposit, err := types.DefaultParams().MinDepositFor("uakt")
	require.NoError(suite.t, err)

	err = suite.ekeeper.AccountCreate(suite.ctx,
		eid,
		owner,
		owner,
		defaultDeposit,
	)
	require.NoError(suite.t, err)
	return eid
}
