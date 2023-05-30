package handler_test

import (
	"errors"
	"testing"

	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"

	types "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	akashtypes "github.com/akash-network/akash-api/go/node/types/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	mkeeper "github.com/akash-network/node/x/market/keeper"
	"github.com/akash-network/node/x/provider/handler"
	"github.com/akash-network/node/x/provider/keeper"
)

const (
	emailValid = "test@example.com"
)

type testSuite struct {
	t       testing.TB
	ctx     sdk.Context
	keeper  keeper.IKeeper
	mkeeper mkeeper.IKeeper
	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &testSuite{
		t:       t,
		ctx:     ssuite.Context(),
		keeper:  ssuite.ProviderKeeper(),
		mkeeper: ssuite.MarketKeeper(),
	}

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
		HostURI: testutil.ProviderHostname(t),
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

func TestProviderCreateWithInfo(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &types.MsgCreateProvider{
		Owner:   testutil.AccAddress(t).String(),
		HostURI: testutil.ProviderHostname(t),
		Info: types.ProviderInfo{
			EMail:   emailValid,
			Website: testutil.Hostname(t),
		},
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
		HostURI:    testutil.ProviderHostname(t),
		Attributes: testutil.Attributes(t),
	}

	msg.Attributes = append(msg.Attributes, msg.Attributes[0])

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, akashtypes.ErrAttributesDuplicateKeys.Error())
}

func TestProviderUpdateWithDuplicated(t *testing.T) {
	suite := setupTestSuite(t)

	createMsg := &types.MsgCreateProvider{
		Owner:      testutil.AccAddress(t).String(),
		HostURI:    testutil.ProviderHostname(t),
		Attributes: testutil.Attributes(t),
	}

	updateMsg := &types.MsgUpdateProvider{
		Owner:      createMsg.Owner,
		HostURI:    testutil.ProviderHostname(t),
		Attributes: createMsg.Attributes,
	}

	updateMsg.Attributes = append(updateMsg.Attributes, updateMsg.Attributes[0])

	err := suite.keeper.Create(suite.ctx, types.Provider(*createMsg))
	require.NoError(t, err)

	res, err := suite.handler(suite.ctx, updateMsg)
	require.Nil(t, res)
	require.EqualError(t, err, akashtypes.ErrAttributesDuplicateKeys.Error())
}

func TestProviderUpdateExisting(t *testing.T) {
	suite := setupTestSuite(t)

	addr := testutil.AccAddress(t)

	createMsg := &types.MsgCreateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.ProviderHostname(t),
		Attributes: testutil.Attributes(t),
	}

	updateMsg := &types.MsgUpdateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.ProviderHostname(t),
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
		HostURI: testutil.ProviderHostname(t),
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
		HostURI:    testutil.ProviderHostname(t),
		Attributes: testutil.Attributes(t),
	}

	updateMsg := &types.MsgUpdateProvider{
		Owner:      addr.String(),
		HostURI:    testutil.ProviderHostname(t),
		Attributes: createMsg.Attributes,
	}

	err := suite.keeper.Create(suite.ctx, types.Provider(*createMsg))
	require.NoError(t, err)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)

	group.GroupSpec.Resources = testutil.Resources(t)
	group.GroupSpec.Requirements = akashtypes.PlacementRequirements{
		Attributes: createMsg.Attributes,
	}

	order, err := suite.mkeeper.CreateOrder(suite.ctx, group.ID(), group.GroupSpec)
	require.NoError(t, err)

	price := testutil.DecCoin(t)

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
		HostURI: testutil.ProviderHostname(t),
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
