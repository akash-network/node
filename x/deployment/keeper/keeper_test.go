package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
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
	t.Run("one active deployment exists", func(t *testing.T) {
		count := 0
		keeper.WithDeploymentsActive(ctx, func(d types.Deployment) bool {
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
		keeper.Create(ctx, deployment, groups)
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

func Test_OnOrderCreated(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	deployment := testutil.Deployment(t)
	groups := []types.Group{
		testutil.DeploymentGroup(t, deployment.ID(), 0),
		testutil.DeploymentGroup(t, deployment.ID(), 1),
	}

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	keeper.OnOrderCreated(ctx, groups[0])

	group, ok := keeper.GetGroup(ctx, groups[0].ID())
	assert.True(t, ok)
	assert.Equal(t, types.GroupOrdered, group.State)
}

func Test_OnLeaseCreated(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	deployment := testutil.Deployment(t)
	groups := testutil.DeploymentGroups(t, deployment.ID(), 0)

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	keeper.OnLeaseCreated(ctx, groups[0].ID())
	group, ok := keeper.GetGroup(ctx, groups[0].ID())
	assert.True(t, ok)
	assert.Equal(t, types.GroupMatched, group.State)
}

func Test_OnInsufficientFunds(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	keeper.OnLeaseInsufficientFunds(ctx, groups[0].ID())

	t.Run("target group changed", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupInsufficientFunds, group.State)
	})

	t.Run("non-target group state unchanged", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupMatched, group.State)
	})
}

func Test_OnLeaseClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	keeper.OnLeaseClosed(ctx, groups[0].ID())

	t.Run("target group changed", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupOpen, group.State)
	})

	t.Run("non-target group state unchanged", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupMatched, group.State)
	})
}

func Test_OnDeploymentClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	keeper.OnDeploymentClosed(ctx, groups[0])

	t.Run("target group changed", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupClosed, group.State)
	})

	t.Run("non-target group state unchanged", func(t *testing.T) {
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupMatched, group.State)
	})
}

func Test_CloseGroup(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	groups := createActiveDeployment(t, ctx, keeper)

	t.Run("assert group 0 state closed", func(t *testing.T) {
		assert.NoError(t, keeper.OnCloseGroup(ctx, groups[0]))
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupClosed, group.State)

		keeper.OnDeploymentClosed(ctx, group) // coverage: catch an additional code path
		assert.Equal(t, types.GroupClosed, group.State)
	})
	t.Run("group 1 matched-state not-orderable", func(t *testing.T) {
		group := groups[1]
		assert.Equal(t, group.ValidateOrderable(), types.ErrGroupNotOpen, group.State.String())
	})
}

func Test_CloseOpenGroups(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	groups := createDeploymentsWithState(t, ctx, types.GroupOpen, keeper)

	t.Run("assert open groups", func(t *testing.T) {
		og := keeper.GetGroups(ctx, groups[0].ID().DeploymentID())
		assert.Len(t, og, 2)
		for _, g := range og {
			assert.Equal(t, g.State, types.GroupOpen)
		}
	})

	t.Run("assert group 0 state closed", func(t *testing.T) {
		assert.NoError(t, keeper.OnCloseGroup(ctx, groups[0]))
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupClosed, group.State)
	})
	t.Run("group 1 matched-state still orderable", func(t *testing.T) {
		group := groups[1]
		assert.Equal(t, group.ValidateOrderable(), nil, group.State.String())
	})
}

func Test_CloseOpenGroupsCheckIndexes(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	groups := createDeploymentsWithState(t, ctx, types.GroupOpen, keeper)

	t.Run("assert iterating over one group", func(t *testing.T) {
		i := 0
		keeper.WithOpenGroups(ctx, func(g types.Group) bool {
			i++
			return true // aborts the iteration after one group is read and increments i
		})
		assert.Equal(t, i, 1)
	})

	t.Run("assert groups open", func(t *testing.T) {
		openCnt := 0
		keeper.WithOpenGroups(ctx, func(g types.Group) bool {
			openCnt++
			return false
		})
		assert.Equal(t, 2, openCnt)
	})

	t.Run("assert group 0 state closed", func(t *testing.T) {
		assert.NoError(t, keeper.OnCloseGroup(ctx, groups[0]))
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupClosed, group.State)
	})
	t.Run("group 1 matched-state still orderable", func(t *testing.T) {
		group := groups[1]
		assert.Equal(t, group.ValidateOrderable(), nil, group.State.String())
	})
}

func Test_Empty_CloseGroup(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	group := types.Group{
		GroupID: testutil.GroupID(t),
	}

	t.Run("assert non-existent group returns error", func(t *testing.T) {
		err := keeper.OnCloseGroup(ctx, group)
		assert.Error(t, err, "'group not found' error should be returned")
	})
}

func createActiveDeployment(t testing.TB, ctx sdk.Context, keeper keeper.Keeper) []types.Group {
	t.Helper()

	deployment := testutil.Deployment(t)
	groups := []types.Group{
		testutil.DeploymentGroup(t, deployment.ID(), 0),
		testutil.DeploymentGroup(t, deployment.ID(), 1),
	}
	for i := range groups {
		groups[i].State = types.GroupMatched
	}

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	return groups
}

func createDeploymentsWithState(t testing.TB, ctx sdk.Context, gState types.GroupState, keeper keeper.Keeper) []types.Group {
	t.Helper()

	deployment := testutil.Deployment(t)
	groups := []types.Group{
		testutil.DeploymentGroup(t, deployment.ID(), 0),
		testutil.DeploymentGroup(t, deployment.ID(), 1),
	}
	for i := range groups {
		groups[i].State = gState
	}

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	return groups
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper) {
	t.Helper()
	key := sdk.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(key, sdk.StoreTypeIAVL, db)
	err := ms.LoadLatestVersion()
	require.NoError(t, err)
	ctx := sdk.NewContext(ms, abci.Header{Time: time.Unix(0, 0)}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(app.MakeCodec(), key)
}
