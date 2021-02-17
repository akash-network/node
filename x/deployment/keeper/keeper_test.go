package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/testutil/state"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

func Test_Create(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	deployment := testutil.Deployment(t)
	groups := testutil.DeploymentGroups(t, deployment.ID(), 0)

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	// assert event emitted
	assert.Len(t, ctx.EventManager().Events(), 1)

	t.Run("deployment written", func(t *testing.T) {
		result, ok := keeper.GetDeployment(ctx, deployment.ID())
		assert.True(t, ok)
		assert.Equal(t, deployment, result)
	})

	t.Run("one deployment exists", func(t *testing.T) {
		count := 0
		keeper.WithDeployments(ctx, func(d types.Deployment) bool {
			if assert.Equal(t, deployment.ID(), d.ID()) {
				count++
			}
			return false
		})
		assert.Equal(t, 1, count)
	})

	// write more data.
	{
		deployment := testutil.Deployment(t)
		groups := testutil.DeploymentGroups(t, deployment.ID(), 0)
		assert.NoError(t, keeper.Create(ctx, deployment, groups))
	}

	t.Run("groups written - read all", func(t *testing.T) {
		result := keeper.GetGroups(ctx, deployment.ID())
		assert.Equal(t, groups, result)
	})

	// assert groups written - read single
	for i := 0; i < len(groups); i++ {
		result, ok := keeper.GetGroup(ctx, groups[i].ID())
		assert.True(t, ok)
		assert.Equal(t, groups[i], result)
	}

	t.Run("non-existent group", func(t *testing.T) {
		_, ok := keeper.GetGroup(ctx, testutil.GroupID(t))
		assert.False(t, ok)
	})
}

func Test_Create_dupe(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	deployment := testutil.Deployment(t)
	groups := testutil.DeploymentGroups(t, deployment.ID(), 0)

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	err = keeper.Create(ctx, deployment, groups)
	require.Error(t, err)
}

func Test_Create_badgroups(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	deployment := testutil.Deployment(t)
	groups := testutil.DeploymentGroups(t, testutil.DeploymentID(t), 0)

	err := keeper.Create(ctx, deployment, groups)
	require.Error(t, err)

	// no events if not created
	assert.Empty(t, ctx.EventManager().Events())
}

func Test_UpdateDeployment(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	deployment := testutil.Deployment(t)
	groups := testutil.DeploymentGroups(t, deployment.ID(), 0)

	err := keeper.UpdateDeployment(ctx, deployment)
	require.Error(t, err)

	err = keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	deployment.Version = []byte{5, 6, 7, 8}

	err = keeper.UpdateDeployment(ctx, deployment)
	require.NoError(t, err)

	result, ok := keeper.GetDeployment(ctx, deployment.ID())
	require.True(t, ok)
	require.Equal(t, deployment, result)
}

func Test_OnEscrowAccountClosed_overdrawn(t *testing.T) {
	t.Skip("Hooks Refactor")
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	did := groups[0].ID().DeploymentID()

	// eid := types.EscrowAccountForDeployment(did)

	// eobj := etypes.Account{
	// 	ID:    eid,
	// 	State: etypes.AccountOverdrawn,
	// }

	// keeper.OnEscrowAccountClosed(ctx, eobj)

	{
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupInsufficientFunds, group.State)
	}

	{
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupInsufficientFunds, group.State)
	}

	{
		deployment, ok := keeper.GetDeployment(ctx, did)
		assert.True(t, ok)
		assert.Equal(t, types.DeploymentClosed, deployment.State)
	}
}

func Test_OnBidClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	err := keeper.OnBidClosed(ctx, groups[0].ID())
	require.NoError(t, err)

	t.Run("target group changed", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupPaused, group.State)
	})

	t.Run("non-target group state unchanged", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupOpen, group.State)
	})
}

func Test_CloseGroup(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	groups := createActiveDeployment(t, ctx, keeper)

	t.Run("assert group 0 state closed", func(t *testing.T) {
		assert.NoError(t, keeper.OnCloseGroup(ctx, groups[0], types.GroupClosed))
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupClosed, group.State)

		assert.Equal(t, types.GroupClosed, group.State)
	})
	t.Run("group 1 matched-state orderable", func(t *testing.T) {
		group := groups[1]
		assert.Equal(t, types.GroupOpen, group.State)
	})
}

func Test_Empty_CloseGroup(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	group := types.Group{
		GroupID: testutil.GroupID(t),
	}

	t.Run("assert non-existent group returns error", func(t *testing.T) {
		err := keeper.OnCloseGroup(ctx, group, types.GroupClosed)
		assert.Error(t, err, "'group not found' error should be returned")
	})
}

func createActiveDeployment(t testing.TB, ctx sdk.Context, keeper keeper.IKeeper) []types.Group {
	t.Helper()

	deployment := testutil.Deployment(t)
	groups := []types.Group{
		testutil.DeploymentGroup(t, deployment.ID(), 0),
		testutil.DeploymentGroup(t, deployment.ID(), 1),
	}
	for i := range groups {
		groups[i].State = types.GroupOpen
	}

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	return groups
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.IKeeper) {
	t.Helper()

	suite := state.SetupTestSuite(t)

	return suite.Context(), suite.DeploymentKeeper()
}
