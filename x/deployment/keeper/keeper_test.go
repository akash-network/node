package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	dbm "github.com/tendermint/tm-db"
)

func Test_Create(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	deployment := testutil.Deployment(t)
	groups := testutil.DeploymentGroups(t, deployment.ID(), 0)

	err := keeper.Create(ctx, deployment, groups)
	require.NoError(t, err)

	// assert event emitted
	assert.Len(t, ctx.EventManager().Events(), 1)

	// assert deployment written
	{
		result, ok := keeper.GetDeployment(ctx, deployment.ID())
		assert.True(t, ok)
		assert.Equal(t, deployment, result)
	}

	// assert one deployment exists
	{
		count := 0
		keeper.WithDeployments(ctx, func(d types.Deployment) bool {
			if assert.Equal(t, deployment.ID(), d.ID()) {
				count++
			}
			return false
		})
		assert.Equal(t, 1, count)
	}

	// write more data.
	{
		deployment := testutil.Deployment(t)
		groups := testutil.DeploymentGroups(t, deployment.ID(), 0)
		keeper.Create(ctx, deployment, groups)
	}

	// assert groups written - read all
	{
		result := keeper.GetGroups(ctx, deployment.ID())
		assert.Equal(t, groups, result)
	}

	// assert groups written - read single
	for i := 0; i < len(groups); i++ {
		result, ok := keeper.GetGroup(ctx, groups[i].ID())
		assert.True(t, ok)
		assert.Equal(t, groups[i], result)
	}

	// check non-existent group
	{
		_, ok := keeper.GetGroup(ctx, testutil.GroupID(t))
		assert.False(t, ok)
	}

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

	{
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupInsufficientFunds, group.State)
	}

	{
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupMatched, group.State)
	}
}

func Test_OnLeaseClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	keeper.OnLeaseClosed(ctx, groups[0].ID())

	{
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupOpen, group.State)
	}

	{
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupMatched, group.State)
	}
}

func Test_OnDeploymentClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)

	groups := createActiveDeployment(t, ctx, keeper)

	keeper.OnDeploymentClosed(ctx, groups[0])

	{
		group, ok := keeper.GetGroup(ctx, groups[0].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupClosed, group.State)
	}

	{
		group, ok := keeper.GetGroup(ctx, groups[1].ID())
		assert.True(t, ok)
		assert.Equal(t, types.GroupMatched, group.State)
	}
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

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper) {
	t.Helper()
	key := sdk.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(key, sdk.StoreTypeIAVL, db)
	ms.LoadLatestVersion()
	ctx := sdk.NewContext(ms, abci.Header{Time: time.Unix(0, 0)}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(app.MakeCodec(), key)
}
