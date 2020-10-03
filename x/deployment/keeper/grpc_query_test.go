package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

type grpcTestSuite struct {
	t      *testing.T
	app    *app.AkashApp
	ctx    sdk.Context
	keeper keeper.Keeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	suite := &grpcTestSuite{
		t: t,
	}

	suite.app = app.Setup(false)
	suite.ctx, suite.keeper = setupKeeper(t)
	querier := keeper.Querier{Keeper: suite.keeper}

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

	var (
		req           *types.QueryDeploymentRequest
		expDeployment types.DeploymentResponse
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
					Owner: testutil.AccAddress(t),
					DSeq:  32,
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryDeploymentRequest{ID: deployment.DeploymentID}
				expDeployment = types.DeploymentResponse{
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
				require.Equal(t, expDeployment, res.Deployment)
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

	deployment2, groups2 := suite.createDeployment()
	deployment2.State = types.DeploymentClosed
	err = suite.keeper.Create(suite.ctx, deployment2, groups2)
	require.NoError(t, err)

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
						State: "closed",
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
					Owner: testutil.AccAddress(t),
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
			Price:     testutil.Coin(suite.t),
		},
	}
	groups := []types.Group{
		group,
	}

	for i := range groups {
		groups[i].State = types.GroupMatched
	}

	return deployment, groups
}
