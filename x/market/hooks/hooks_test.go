package hooks_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/id/v1"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"
	cmocks "pkg.akt.dev/node/testutil/cosmos/mocks"
	"pkg.akt.dev/node/testutil/state"
	dkeeper "pkg.akt.dev/node/x/deployment/keeper"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
	"pkg.akt.dev/node/x/market/hooks"
	mkeeper "pkg.akt.dev/node/x/market/keeper"
)

type testSuite struct {
	t       testing.TB
	ctx     sdk.Context
	ekeeper ekeeper.Keeper
	dkeeper dkeeper.IKeeper
	mkeeper mkeeper.IKeeper
	bkeeper *cmocks.BankKeeper
}

type testSeedData struct {
	did dv1.DeploymentID
	aid ev1.Account
}

type testInput struct {
	accountState    etypes.AccountState
	deploymentState dv1.Deployment_State
	groupState      dtypes.Group_State
}

func setupTestSuite(t *testing.T) *testSuite {
	ssuite := state.SetupTestSuite(t)

	suite := &testSuite{
		t:       t,
		ctx:     ssuite.Context(),
		ekeeper: ssuite.EscrowKeeper(),
		dkeeper: ssuite.DeploymentKeeper(),
		mkeeper: ssuite.MarketKeeper(),
		bkeeper: ssuite.BankKeeper(),
	}

	return suite
}

func TestEscrowAccountClose(t *testing.T) {
	suite := setupTestSuite(t)

	tests := []struct {
		description             string
		testInput               testInput
		expectedDeploymentState dv1.Deployment_State
		expectedGroupState      dtypes.Group_State
	}{
		{
			"Overdrawn account when deployment is active",
			testInput{
				etypes.AccountState{
					State: etypes.StateOverdrawn,
				},
				dv1.DeploymentActive,
				dtypes.GroupOpen,
			},
			dv1.DeploymentActive,
			dtypes.GroupPaused,
		},
		{
			"Overdrawn account when deployment is closed",
			testInput{
				etypes.AccountState{
					State: etypes.StateOverdrawn,
				},
				dv1.DeploymentClosed,
				dtypes.GroupClosed,
			},
			dv1.DeploymentClosed,
			dtypes.GroupClosed,
		},
		{
			"Account in good standing when deployment is active",
			testInput{
				etypes.AccountState{
					State: etypes.StateOpen,
				},
				dv1.DeploymentActive,
				dtypes.GroupOpen,
			},
			dv1.DeploymentClosed,
			dtypes.GroupClosed,
		},
		{
			"Account in good standing when deployment is closed",
			testInput{
				etypes.AccountState{
					State: etypes.StateOpen,
				},
				dv1.DeploymentClosed,
				dtypes.GroupClosed,
			},
			dv1.DeploymentClosed,
			dtypes.GroupClosed,
		},
		{
			"Account already closed",
			testInput{
				etypes.AccountState{
					State: etypes.StateClosed,
				},
				dv1.DeploymentActive,
				dtypes.GroupOpen,
			},
			dv1.DeploymentClosed,
			dtypes.GroupClosed,
		},
		{
			"Account is in an invalid state",
			testInput{
				etypes.AccountState{
					State: etypes.StateInvalid,
				},
				dv1.DeploymentActive,
				dtypes.GroupOpen,
			},
			dv1.DeploymentClosed,
			dtypes.GroupClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			ctx := suite.ctx
			hooks := hooks.New(suite.dkeeper, suite.mkeeper)

			seedData := setupSeedData(t, suite, tt.testInput)

			hooks.OnEscrowAccountClosed(
				ctx,
				etypes.Account{
					ID:    seedData.aid,
					State: tt.testInput.accountState,
				})

			deployment, found := suite.dkeeper.GetDeployment(ctx, seedData.did)
			assert.NotNil(t, deployment)
			assert.True(t, found)

			assert.Equal(t, tt.expectedDeploymentState, deployment.State)

			groups := suite.dkeeper.GetGroups(ctx, seedData.did)

			for _, g := range groups {
				assert.Equal(t, tt.expectedGroupState, g.State)
			}
		})
	}
}

func setupSeedData(t testing.TB, suite *testSuite, ti testInput) testSeedData {
	t.Helper()

	ctx := suite.ctx

	did := testutil.DeploymentID(t)
	aid := did.ToEscrowAccountID()

	deployment := dv1.Deployment{
		ID:    did,
		State: ti.deploymentState,
	}

	groupCount := 3

	groups := make([]dtypes.Group, groupCount)
	for i := range groups {
		groups[i] = testutil.DeploymentGroup(t, did, uint32(i)+1)
		groups[i].State = ti.groupState
	}

	require.NoError(t, suite.dkeeper.Create(ctx, deployment, groups))

	return testSeedData{
		did: did,
		aid: aid,
	}
}
