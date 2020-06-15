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
	"github.com/ovrclk/akash/x/provider/handler"

	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

type testuite struct {
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	keeper  keeper.Keeper
	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testuite {
	suite := &testuite{}

	keyProvider := sdk.NewTransientStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db)
	suite.ms.MountStoreWithDB(keyProvider, sdk.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, abci.Header{}, true, testutil.Logger(t))

	suite.keeper = keeper.NewKeeper(app.MakeCodec(), keyProvider)
	suite.handler = handler.NewHandler(suite.keeper)

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	_, err := suite.handler(suite.ctx, sdk.NewTestMsg())
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestProviderCreate(t *testing.T) {
	suite := setupTestSuite(t)

	msg := types.MsgCreateProvider{
		Owner:   testutil.AccAddress(t),
		HostURI: testutil.Hostname(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	res, err = suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrProviderExists))
}

func TestProviderUpdateExisting(t *testing.T) {
	suite := setupTestSuite(t)

	addr := testutil.AccAddress(t)

	createMsg := types.MsgCreateProvider{
		Owner:   addr,
		HostURI: testutil.Hostname(t),
	}

	updateMsg := types.MsgUpdateProvider{
		Owner:   addr,
		HostURI: testutil.Hostname(t),
	}

	err := suite.keeper.Create(suite.ctx, types.Provider(createMsg))
	require.NoError(t, err)

	res, err := suite.handler(suite.ctx, updateMsg)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestProviderUpdateNotExisting(t *testing.T) {
	suite := setupTestSuite(t)
	msg := types.MsgUpdateProvider{
		Owner:   testutil.AccAddress(t),
		HostURI: testutil.Hostname(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Error(t, err)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}

func TestProviderDeleteExisting(t *testing.T) {
	suite := setupTestSuite(t)

	addr := testutil.AccAddress(t)

	createMsg := types.MsgCreateProvider{
		Owner:   addr,
		HostURI: testutil.Hostname(t),
	}

	deleteMsg := types.MsgDeleteProvider{
		Owner: addr,
	}

	err := suite.keeper.Create(suite.ctx, types.Provider(createMsg))
	require.NoError(t, err)

	res, err := suite.handler(suite.ctx, deleteMsg)
	require.Nil(t, res)
	require.EqualError(t, err, handler.ErrInternal.Error()+": NOTIMPLEMENTED")
	require.True(t, errors.Is(err, handler.ErrInternal))
}

func TestProviderDeleteNonExisting(t *testing.T) {
	suite := setupTestSuite(t)
	msg := types.MsgDeleteProvider{
		Owner: testutil.AccAddress(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Error(t, err)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}
