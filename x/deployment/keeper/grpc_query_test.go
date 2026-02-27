package keeper_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"

	"pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	eid "pkg.akt.dev/go/node/escrow/id/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/deployment/keeper"
	ekeeper "pkg.akt.dev/node/v2/x/escrow/keeper"
)

type grpcTestSuite struct {
	*state.TestSuite
	t           *testing.T
	app         *app.AkashApp
	ctx         sdk.Context
	keeper      keeper.IKeeper
	ekeeper     ekeeper.Keeper
	authzKeeper ekeeper.AuthzKeeper
	bankKeeper  ekeeper.BankKeeper

	queryClient dvbeta.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &grpcTestSuite{
		TestSuite:   ssuite,
		t:           t,
		app:         ssuite.App(),
		ctx:         ssuite.Context(),
		keeper:      ssuite.DeploymentKeeper(),
		ekeeper:     ssuite.EscrowKeeper(),
		authzKeeper: ssuite.AuthzKeeper(),
		bankKeeper:  ssuite.BankKeeper(),
	}

	querier := suite.keeper.NewQuerier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	dvbeta.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = dvbeta.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryDeployment(t *testing.T) {
	suite := setupTest(t)

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)

		bkeeper.On("BurnCoins", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

	// creating deployment
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)

	eid := suite.createEscrowAccount(deployment.ID)

	var req *dvbeta.QueryDeploymentRequest
	var expDeployment dvbeta.QueryDeploymentResponse

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &dvbeta.QueryDeploymentRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &dvbeta.QueryDeploymentRequest{ID: v1.DeploymentID{}}
			},
			false,
		},
		{
			"deployment not found",
			func() {
				req = &dvbeta.QueryDeploymentRequest{ID: v1.DeploymentID{
					Owner: testutil.AccAddress(t).String(),
					DSeq:  32,
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &dvbeta.QueryDeploymentRequest{ID: deployment.ID}
				expDeployment = dvbeta.QueryDeploymentResponse{
					Deployment: deployment,
					Groups:     groups,
				}
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

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
	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)

		bkeeper.On("BurnCoins", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

	// creating deployments with different states
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)
	suite.createEscrowAccount(deployment.ID)

	deployment2, groups2 := suite.createDeployment()
	deployment2.State = v1.DeploymentActive
	err = suite.keeper.Create(suite.ctx, deployment2, groups2)
	require.NoError(t, err)
	suite.createEscrowAccount(deployment2.ID)

	deployment3, groups3 := suite.createDeployment()
	deployment3.State = v1.DeploymentClosed
	err = suite.keeper.Create(suite.ctx, deployment3, groups3)
	require.NoError(t, err)
	suite.createEscrowAccount(deployment3.ID)

	var req *dvbeta.QueryDeploymentsRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query deployments without any filters and pagination",
			func() {
				req = &dvbeta.QueryDeploymentsRequest{}
			},
			3,
		},
		{
			"query deployments with state filter",
			func() {
				req = &dvbeta.QueryDeploymentsRequest{
					Filters: dvbeta.DeploymentFilters{
						State: v1.DeploymentActive.String(),
					},
				}
			},
			2,
		},
		{
			"query deployments with filters having non existent data",
			func() {
				req = &dvbeta.QueryDeploymentsRequest{
					Filters: dvbeta.DeploymentFilters{
						DSeq:  37,
						State: v1.DeploymentClosed.String(),
					}}
			},
			0,
		},
		{
			"query deployments with state filter",
			func() {
				req = &dvbeta.QueryDeploymentsRequest{Filters: dvbeta.DeploymentFilters{State: v1.DeploymentClosed.String()}}
			},
			1,
		},
		{
			"query deployments with pagination",
			func() {
				req = &dvbeta.QueryDeploymentsRequest{Pagination: &sdkquery.PageRequest{Limit: 1}}
			},
			1,
		},
		{
			"query deployments with pagination next key",
			func() {
				req = &dvbeta.QueryDeploymentsRequest{
					Filters:    dvbeta.DeploymentFilters{State: v1.DeploymentActive.String()},
					Pagination: &sdkquery.PageRequest{Limit: 1},
				}
			},
			1,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

			res, err := suite.queryClient.Deployments(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.expLen, len(res.Deployments))
		})
	}

	// Validate offset pagination returns different records
	t.Run("offset pagination returns distinct deployments", func(t *testing.T) {
		page0, err := suite.queryClient.Deployments(suite.ctx, &dvbeta.QueryDeploymentsRequest{
			Filters:    dvbeta.DeploymentFilters{State: v1.DeploymentActive.String()},
			Pagination: &sdkquery.PageRequest{Offset: 0, Limit: 1},
		})
		require.NoError(t, err)
		require.Len(t, page0.Deployments, 1)

		page1, err := suite.queryClient.Deployments(suite.ctx, &dvbeta.QueryDeploymentsRequest{
			Filters:    dvbeta.DeploymentFilters{State: v1.DeploymentActive.String()},
			Pagination: &sdkquery.PageRequest{Offset: 1, Limit: 1},
		})
		require.NoError(t, err)
		require.Len(t, page1.Deployments, 1)

		require.NotEqual(t, page0.Deployments[0].Deployment.ID,
			page1.Deployments[0].Deployment.ID,
			"offset pagination must return different deployments")
	})
}

type deploymentFilterModifier struct {
	fieldName string
	f         func(leaseID v1.DeploymentID, filter dvbeta.DeploymentFilters) dvbeta.DeploymentFilters
	getField  func(leaseID v1.DeploymentID) interface{}
}

func TestGRPCQueryDeploymentsWithFilter(t *testing.T) {
	suite := setupTest(t)
	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)

		bkeeper.On("BurnCoins", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

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
			func(depID v1.DeploymentID, filter dvbeta.DeploymentFilters) dvbeta.DeploymentFilters {
				filter.Owner = depID.GetOwner()
				return filter
			},
			func(depID v1.DeploymentID) interface{} {
				return depID.Owner
			},
		},
		{
			"dseq",
			func(depID v1.DeploymentID, filter dvbeta.DeploymentFilters) dvbeta.DeploymentFilters {
				filter.DSeq = depID.DSeq
				return filter
			},
			func(depID v1.DeploymentID) interface{} {
				return depID.DSeq
			},
		},
	}

	ctx := suite.ctx

	for _, depID := range deps {
		for _, m := range modifiers {
			req := &dvbeta.QueryDeploymentsRequest{
				Filters: m.f(depID, dvbeta.DeploymentFilters{}),
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
			filter := dvbeta.DeploymentFilters{}
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

			req := &dvbeta.QueryDeploymentsRequest{
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

		filter := dvbeta.DeploymentFilters{}
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

		req := &dvbeta.QueryDeploymentsRequest{
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
		req := &dvbeta.QueryDeploymentsRequest{
			Filters: dvbeta.DeploymentFilters{
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
		req = &dvbeta.QueryDeploymentsRequest{
			Filters: dvbeta.DeploymentFilters{
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
		req = &dvbeta.QueryDeploymentsRequest{
			Filters: dvbeta.DeploymentFilters{
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

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

	// creating group
	deployment, groups := suite.createDeployment()
	err := suite.keeper.Create(suite.ctx, deployment, groups)
	require.NoError(t, err)

	var (
		req           *dvbeta.QueryGroupRequest
		expDeployment dvbeta.Group
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &dvbeta.QueryGroupRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &dvbeta.QueryGroupRequest{ID: v1.GroupID{}}
			},
			false,
		},
		{
			"group not found",
			func() {
				req = &dvbeta.QueryGroupRequest{ID: v1.GroupID{
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
				req = &dvbeta.QueryGroupRequest{ID: groups[0].GetID()}
				expDeployment = groups[0]
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

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

func (suite *grpcTestSuite) createDeployment() (v1.Deployment, dvbeta.Groups) {
	suite.t.Helper()

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

	deployment := testutil.Deployment(suite.t)
	group := testutil.DeploymentGroup(suite.t, deployment.ID, 0)
	group.GroupSpec.Resources = dvbeta.ResourceUnits{
		{
			Resources: testutil.ResourceUnits(suite.t),
			Count:     1,
			Price:     testutil.DecCoin(suite.t),
		},
	}
	groups := []dvbeta.Group{
		group,
	}

	for i := range groups {
		groups[i].State = dvbeta.GroupOpen
	}

	return deployment, groups
}

func (suite *grpcTestSuite) createEscrowAccount(id v1.DeploymentID) eid.Account {
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	require.NoError(suite.t, err)

	eid := id.ToEscrowAccountID()
	defaultDeposit, err := dvbeta.DefaultParams().MinDepositFor("uact")
	require.NoError(suite.t, err)

	msg := &dvbeta.MsgCreateDeployment{
		ID: id,
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	deposits, err := suite.ekeeper.AuthorizeDeposits(suite.ctx, msg)
	require.NoError(suite.t, err)

	err = suite.ekeeper.AccountCreate(suite.ctx, eid, owner, deposits)
	require.NoError(suite.t, err)
	return eid
}
