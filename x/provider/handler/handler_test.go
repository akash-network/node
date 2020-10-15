package handler_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/testutil"
	mkeeper "github.com/ovrclk/akash/x/market/keeper"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/ovrclk/akash/x/provider/handler"

	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

type testSuite struct {
	t       testing.TB
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	keeper  keeper.Keeper
	mkeeper mkeeper.Keeper
	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testSuite {
	suite := &testSuite{
		t: t,
	}

	pKey := sdk.NewTransientStoreKey(types.StoreKey)
	mKey := sdk.NewTransientStoreKey(mtypes.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db)
	suite.ms.MountStoreWithDB(pKey, sdk.StoreTypeIAVL, db)
	suite.ms.MountStoreWithDB(mKey, sdk.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, tmproto.Header{}, true, testutil.Logger(t))

	suite.keeper = keeper.NewKeeper(types.ModuleCdc, pKey)
	suite.mkeeper = mkeeper.NewKeeper(types.ModuleCdc, mKey)

	suite.handler = handler.NewHandler(suite.keeper, suite.mkeeper)

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	_, err := suite.handler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestProviderCreate(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &types.MsgCreateProvider{
		Owner:   testutil.AccAddress(t).String(),
		HostURI: testutil.Hostname(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {

		iev := testutil.ParseProviderEvent(t, res.Events)
		require.IsType(t, types.EventProviderCreated{}, iev)

		dev := iev.(types.EventProviderCreated)

		require.Equal(t, msg.Owner, dev.Owner.String())
	})

	res, err = suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrProviderExists))
}

func TestProviderCreateWithDuplicated(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &types.MsgCreateProvider{
		Owner:      testutil.AccAddress(t).String(),
		HostURI:    testutil.Hostname(t),
		Attributes: testutil.Attributes(t),
	}

	msg.Attributes = append(msg.Attributes, msg.Attributes[0])

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDuplicateAttributes.Error())
}

func TestProviderUpdateWithDuplicated(t *testing.T) {
	suite := setupTestSuite(t)

	createMsg := &types.MsgCreateProvider{
		Owner:      testutil.AccAddress(t).String(),
		HostURI:    testutil.Hostname(t),
		Attributes: testutil.Attributes(t),
	}

	updateMsg := &types.MsgUpdateProvider{
		Owner:      createMsg.Owner,
		HostURI:    testutil.Hostname(t),
		Attributes: createMsg.Attributes,
	}

	updateMsg.Attributes = append(updateMsg.Attributes, updateMsg.Attributes[0])

	err := suite.keeper.Create(suite.ctx, types.Provider(*createMsg))
	require.NoError(t, err)

	res, err := suite.handler(suite.ctx, updateMsg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDuplicateAttributes.Error())
}

func TestProviderUpdateExisting(t *testing.T) {
	suite := setupTestSuite(t)

	addr := testutil.AccAddress(t)

	createMsg := &types.MsgCreateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.Hostname(t),
		Attributes: testutil.Attributes(t),
	}

	updateMsg := &types.MsgUpdateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.Hostname(t),
		Attributes: createMsg.Attributes,
	}

	err := suite.keeper.Create(suite.ctx, types.Provider(*createMsg))
	require.NoError(t, err)

	res, err := suite.handler(suite.ctx, updateMsg)

	t.Run("ensure event created", func(t *testing.T) {

		iev := testutil.ParseProviderEvent(t, res.Events[1:])
		require.IsType(t, types.EventProviderUpdated{}, iev)

		dev := iev.(types.EventProviderUpdated)

		require.Equal(t, updateMsg.Owner, dev.Owner.String())
	})

	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestProviderUpdateNotExisting(t *testing.T) {
	suite := setupTestSuite(t)
	msg := &types.MsgUpdateProvider{
		Owner:   testutil.AccAddress(t).String(),
		HostURI: testutil.Hostname(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Error(t, err)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}

func TestProviderUpdateAttributes(t *testing.T) {
	suite := setupTestSuite(t)

	addr := testutil.AccAddress(t)

	createMsg := &types.MsgCreateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.Hostname(t),
		Attributes: testutil.Attributes(t),
	}

	updateMsg := &types.MsgUpdateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.Hostname(t),
		Attributes: createMsg.Attributes,
	}

	err := suite.keeper.Create(suite.ctx, types.Provider(*createMsg))
	require.NoError(t, err)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)

	group.GroupSpec.Resources = testutil.Resources(t)
	group.GroupSpec.Requirements = createMsg.Attributes

	order, err := suite.mkeeper.CreateOrder(suite.ctx, group.ID(), group.GroupSpec)
	require.NoError(t, err)

	price := testutil.Coin(t)

	bid, err := suite.mkeeper.CreateBid(suite.ctx, order.ID(), addr, price)
	require.NoError(t, err)

	suite.mkeeper.CreateLease(suite.ctx, bid)

	res, err := suite.handler(suite.ctx, updateMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {

		iev := testutil.ParseProviderEvent(t, res.Events[4:])
		require.IsType(t, types.EventProviderUpdated{}, iev)

		dev := iev.(types.EventProviderUpdated)

		require.Equal(t, updateMsg.Owner, dev.Owner.String())
	})

	updateMsg.Attributes = nil
	res, err = suite.handler(suite.ctx, updateMsg)
	require.Error(t, err, types.ErrIncompatibleAttributes.Error())
	require.Nil(t, res)
}

func TestProviderDeleteExisting(t *testing.T) {
	suite := setupTestSuite(t)

	addr := testutil.AccAddress(t)

	createMsg := &types.MsgCreateProvider{
		Owner:   addr.String(),
		HostURI: testutil.Hostname(t),
	}

	deleteMsg := &types.MsgDeleteProvider{
		Owner: addr.String(),
	}

	err := suite.keeper.Create(suite.ctx, types.Provider(*createMsg))
	require.NoError(t, err)

	res, err := suite.handler(suite.ctx, deleteMsg)
	require.Nil(t, res)
	require.EqualError(t, err, "NOTIMPLEMENTED: "+handler.ErrInternal.Error())
	require.True(t, errors.Is(err, handler.ErrInternal))

	t.Run("ensure event created", func(t *testing.T) {
		// TODO: this should emit a ProviderDelete
	})
}

func TestProviderDeleteNonExisting(t *testing.T) {
	suite := setupTestSuite(t)
	msg := &types.MsgDeleteProvider{
		Owner: testutil.AccAddress(t).String(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Error(t, err)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}
