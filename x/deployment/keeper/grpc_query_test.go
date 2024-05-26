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

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"

	"pkg.akt.dev/akashd/app"
	"pkg.akt.dev/akashd/testutil"
	"pkg.akt.dev/akashd/testutil/state"
	"pkg.akt.dev/akashd/x/deployment/keeper"
	ekeeper "pkg.akt.dev/akashd/x/escrow/keeper"
)

type grpcTestSuite struct {
	t       *testing.T
	app     *app.AkashApp
	ctx     sdk.Context
	keeper  keeper.IKeeper
	ekeeper ekeeper.Keeper

	queryClient v1beta4.QueryClient
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
	v1beta4.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = v1beta4.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryDeployment(t *testing.T) {
	suite := setupTest(t)

	// creating deployment
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)

	eid := suite.createEscrowAccount(deployment.ID)

	var (
		req           *v1beta4.QueryDeploymentRequest
		expDeployment v1beta4.QueryDeploymentResponse
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &v1beta4.QueryDeploymentRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &v1beta4.QueryDeploymentRequest{ID: v1.DeploymentID{}}
			},
			false,
		},
		{
			"deployment not found",
			func() {
				req = &v1beta4.QueryDeploymentRequest{ID: v1.DeploymentID{
					Owner: testutil.AccAddress(t).String(),
					DSeq:  32,
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &v1beta4.QueryDeploymentRequest{ID: deployment.ID}
				expDeployment = v1beta4.QueryDeploymentResponse{
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
	suite.createEscrowAccount(deployment.ID)

	deployment2, groups2 := suite.createDeployment()
	deployment2.State = v1.DeploymentClosed
	err = suite.keeper.Create(suite.ctx, deployment2, groups2)
	require.NoError(t, err)
	suite.createEscrowAccount(deployment2.ID)

	var req *v1beta4.QueryDeploymentsRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query deployments without any filters and pagination",
			func() {
				req = &v1beta4.QueryDeploymentsRequest{}
			},
			2,
		},
		{
			"query deployments with filters having non existent data",
			func() {
				req = &v1beta4.QueryDeploymentsRequest{
					Filters: v1beta4.DeploymentFilters{
						DSeq:  37,
						State: v1.DeploymentClosed.String(),
					}}
			},
			0,
		},
		{
			"query deployments with state filter",
			func() {
				req = &v1beta4.QueryDeploymentsRequest{Filters: v1beta4.DeploymentFilters{State: v1.DeploymentClosed.String()}}
			},
			1,
		},
		{
			"query deployments with pagination",
			func() {
				req = &v1beta4.QueryDeploymentsRequest{Pagination: &sdkquery.PageRequest{Limit: 1}}
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
	f         func(leaseID v1.DeploymentID, filter v1beta4.DeploymentFilters) v1beta4.DeploymentFilters
	getField  func(leaseID v1.DeploymentID) interface{}
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

	deps := []v1.DeploymentID{
		depA,
		depB,
		depC,
	}

	modifiers := []deploymentFilterModifier{
		{
			"owner",
			func(depID v1.DeploymentID, filter v1beta4.DeploymentFilters) v1beta4.DeploymentFilters {
				filter.Owner = depID.GetOwner()
				return filter
			},
			func(depID v1.DeploymentID) interface{} {
				return depID.Owner
			},
		},
		{
			"dseq",
			func(depID v1.DeploymentID, filter v1beta4.DeploymentFilters) v1beta4.DeploymentFilters {
				filter.DSeq = depID.DSeq
				return filter
			},
			func(depID v1.DeploymentID) interface{} {
				return depID.DSeq
			},
		},
	}

	ctx := sdk.WrapSDKContext(suite.ctx)

	for _, depID := range deps {
		for _, m := range modifiers {
			req := &v1beta4.QueryDeploymentsRequest{
				Filters: m.f(depID, v1beta4.DeploymentFilters{}),
			}

			res, err := suite.queryClient.Deployments(ctx, req)

			require.NoError(t, err, "testing %v", m.fieldName)
			require.NotNil(t, res, "testing %v", m.fieldName)
			// At least 1 result
			assert.GreaterOrEqual(t, len(res.Deployments), 1, "testing %v", m.fieldName)

			for _, dep := range res.Deployments {
				assert.Equal(t, m.getField(depID), m.getField(dep.Deployment.ID), "testing %v", m.fieldName)
			}
		}
	}

	limit := int(math.Pow(2, float64(len(modifiers))))

	// Use an order ID that matches absolutely nothing in any field
	bogusOrderID := v1.DeploymentID{
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
			filter := v1beta4.DeploymentFilters{}
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

			req := &v1beta4.QueryDeploymentsRequest{
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
					require.Equal(t, m.getField(orderID), m.getField(dep.Deployment.ID), "testing %v", m.fieldName)
				}
			}
		}

		filter := v1beta4.DeploymentFilters{}
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

		req := &v1beta4.QueryDeploymentsRequest{
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
		req := &v1beta4.QueryDeploymentsRequest{
			Filters: v1beta4.DeploymentFilters{
				Owner: depID.Owner,
			},
		}

		res, err := suite.queryClient.Deployments(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// Just 1 result
		require.Len(t, res.Deployments, 1)
		depResult := res.Deployments[0]
		require.Equal(t, depID, depResult.GetDeployment().ID)

		// Query with valid DSeq
		req = &v1beta4.QueryDeploymentsRequest{
			Filters: v1beta4.DeploymentFilters{
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
		require.Equal(t, depID, depResult.Deployment.ID)

		// Query with a bogus DSeq
		req = &v1beta4.QueryDeploymentsRequest{
			Filters: v1beta4.DeploymentFilters{
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
		req           *v1beta4.QueryGroupRequest
		expDeployment v1beta4.Group
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &v1beta4.QueryGroupRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &v1beta4.QueryGroupRequest{ID: v1.GroupID{}}
			},
			false,
		},
		{
			"group not found",
			func() {
				req = &v1beta4.QueryGroupRequest{ID: v1.GroupID{
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
				req = &v1beta4.QueryGroupRequest{ID: groups[0].ID}
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

func (suite *grpcTestSuite) createDeployment() (v1.Deployment, v1beta4.Groups) {
	suite.t.Helper()

	deployment := testutil.Deployment(suite.t)
	group := testutil.DeploymentGroup(suite.t, deployment.ID, 0)
	group.GroupSpec.Resources = v1beta4.ResourceUnits{
		{
			Resources: testutil.ResourceUnits(suite.t),
			Count:     1,
			Price:     testutil.DecCoin(suite.t),
		},
	}
	groups := []v1beta4.Group{
		group,
	}

	for i := range groups {
		groups[i].State = v1beta4.GroupOpen
	}

	return deployment, groups
}

func (suite *grpcTestSuite) createEscrowAccount(id v1.DeploymentID) etypes.AccountID {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	require.NoError(suite.t, err)

	eid := v1beta4.EscrowAccountForDeployment(id)
	defaultDeposit, err := v1beta4.DefaultParams().MinDepositFor("uakt")
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
