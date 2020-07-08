package handler_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/handler"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	mkeeper "github.com/ovrclk/akash/x/market/keeper"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type testSuite struct {
	t       *testing.T
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	mkeeper mkeeper.Keeper
	dkeeper keeper.Keeper
	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testSuite {
	suite := &testSuite{
		t: t,
	}

	dKey := sdk.NewKVStoreKey(types.StoreKey)
	mKey := sdk.NewKVStoreKey(mtypes.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db)
	suite.ms.MountStoreWithDB(dKey, sdk.StoreTypeIAVL, db)
	suite.ms.MountStoreWithDB(mKey, sdk.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, abci.Header{}, true, testutil.Logger(t))

	suite.mkeeper = mkeeper.NewKeeper(app.MakeCodec(), mKey)
	suite.dkeeper = keeper.NewKeeper(app.MakeCodec(), dKey)

	suite.handler = handler.NewHandler(suite.dkeeper, suite.mkeeper)

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.handler(suite.ctx, sdk.NewTestMsg())
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCreateDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := types.MsgCreateDeployment{
		ID:     deployment.ID(),
		Groups: make([]types.GroupSpec, 0, len(groups)),
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseDeploymentEvent(t, res.Events)
		require.IsType(t, types.EventDeploymentCreated{}, iev)

		dev := iev.(types.EventDeploymentCreated)

		require.Equal(t, msg.ID, dev.ID)
	})

	_, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID())
	require.True(t, exists)

	res, err = suite.handler(suite.ctx, msg)
	require.EqualError(t, err, types.ErrDeploymentExists.Error())
	require.Nil(t, res)
}

func TestCreateDeploymentEmptyGroups(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := types.MsgCreateDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrInvalidGroups))
}

func TestUpdateDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := types.MsgUpdateDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentNotFound.Error())
}

func TestUpdateDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := types.MsgCreateDeployment{
		ID:     deployment.ID(),
		Groups: make([]types.GroupSpec, 0, len(groups)),
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	msgUpdate := types.MsgUpdateDeployment{
		ID: deployment.ID(),
	}

	res, err = suite.handler(suite.ctx, msgUpdate)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseDeploymentEvent(t, res.Events[1:])
		require.IsType(t, types.EventDeploymentUpdated{}, iev)

		dev := iev.(types.EventDeploymentUpdated)

		require.Equal(t, msg.ID, dev.ID)
	})
}

func TestCloseDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := types.MsgCloseDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentNotFound.Error())
}

func TestCloseDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := types.MsgCreateDeployment{
		ID:     deployment.ID(),
		Groups: make([]types.GroupSpec, 0, len(groups)),
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseDeploymentEvent(t, res.Events)
		require.IsType(t, types.EventDeploymentCreated{}, iev)

		dev := iev.(types.EventDeploymentCreated)

		require.Equal(t, msg.ID, dev.ID)
	})

	msgClose := types.MsgCloseDeployment{
		ID: deployment.ID(),
	}

	res, err = suite.handler(suite.ctx, msgClose)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event updated", func(t *testing.T) {
		iev := testutil.ParseDeploymentEvent(t, res.Events[1:2])
		require.IsType(t, types.EventDeploymentUpdated{}, iev)

		dev := iev.(types.EventDeploymentUpdated)

		require.Equal(t, msg.ID, dev.ID)
	})

	t.Run("ensure event close", func(t *testing.T) {
		iev := testutil.ParseDeploymentEvent(t, res.Events[2:])
		require.IsType(t, types.EventDeploymentClosed{}, iev)

		dev := iev.(types.EventDeploymentClosed)

		require.Equal(t, msg.ID, dev.ID)
	})

	res, err = suite.handler(suite.ctx, msgClose)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentClosed.Error())
}

func (st *testSuite) createDeployment() (types.Deployment, []types.Group) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	group := testutil.DeploymentGroup(st.t, deployment.ID(), 0)
	group.Resources = []types.Resource{
		{
			Unit:  testutil.Unit(st.t),
			Count: 1,
			Price: testutil.Coin(st.t),
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

func (st *testSuite) createActiveDeployment() (types.Deployment, []types.Group) {
	st.t.Helper()

	deployment, groups := st.createDeployment()

	err := st.dkeeper.Create(st.ctx, deployment, groups)
	require.NoError(st.t, err)

	return deployment, groups
}
