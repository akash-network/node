package handler_test

import (
	"crypto/sha256"
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

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

	suite.ctx = sdk.NewContext(suite.ms, tmproto.Header{}, true, testutil.Logger(t))

	suite.mkeeper = mkeeper.NewKeeper(types.ModuleCdc, mKey)
	suite.dkeeper = keeper.NewKeeper(types.ModuleCdc, dKey)

	suite.handler = handler.NewHandler(suite.dkeeper, suite.mkeeper)

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.handler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCreateDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &types.MsgCreateDeployment{
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

	msg := &types.MsgCreateDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrInvalidGroups))
}

func TestUpdateDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &types.MsgUpdateDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentNotFound.Error())
}

func TestUpdateDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &types.MsgCreateDeployment{
		ID:      deployment.ID(),
		Groups:  make([]types.GroupSpec, 0, len(groups)),
		Version: testutil.DefaultDeploymentVersion[:],
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("assert deployment version", func(t *testing.T) {
		d, ok := suite.dkeeper.GetDeployment(suite.ctx, deployment.DeploymentID)
		require.True(t, ok)
		assert.Equal(t, d.Version, testutil.DefaultDeploymentVersion[:])
	})

	depSum := sha256.Sum256(testutil.DefaultDeploymentVersion[:])

	msgUpdate := &types.MsgUpdateDeployment{
		ID:      deployment.ID(),
		Version: depSum[:],
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
	t.Run("assert version updated", func(t *testing.T) {
		d, ok := suite.dkeeper.GetDeployment(suite.ctx, deployment.DeploymentID)
		require.True(t, ok)
		assert.Equal(t, d.Version, depSum[:])
	})
}

func TestCloseDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &types.MsgCloseDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentNotFound.Error())
}

func TestCloseDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &types.MsgCreateDeployment{
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

	msgClose := &types.MsgCloseDeployment{
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
	group.GroupSpec.Resources = []types.Resource{
		{
			Resources: testutil.ResourceUnits(st.t),
			Count:     1,
			Price:     testutil.Coin(st.t),
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
