package handler_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/provider/handler"

	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

type testContext struct {
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	keeper  keeper.Keeper
	handler sdk.Handler
}

func setupTestContext(t *testing.T) *testContext {
	testCtx := &testContext{}
	testutil.AccAddress(t)
	keyProvider := sdk.NewTransientStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	testCtx.ms = store.NewCommitMultiStore(db)
	testCtx.ms.MountStoreWithDB(keyProvider, sdk.StoreTypeIAVL, db)

	err := testCtx.ms.LoadLatestVersion()
	require.NoError(t, err)

	testCtx.ctx = sdk.NewContext(testCtx.ms, abci.Header{}, true, log.NewNopLogger())

	testCtx.keeper = keeper.NewKeeper(app.MakeCodec(), keyProvider)
	testCtx.handler = handler.NewHandler(testCtx.keeper)

	return testCtx
}

func TestProviderCreate(t *testing.T) {
	testCtx := setupTestContext(t)

	msg := types.MsgCreateProvider{
		Owner:   testutil.AccAddress(t),
		HostURI: "host",
	}

	_, err := testCtx.handler(testCtx.ctx, msg)
	require.NoError(t, err)

	_, err = testCtx.handler(testCtx.ctx, msg)
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrProviderExists))
}

func TestProviderUpdateExisting(t *testing.T) {
	testCtx := setupTestContext(t)

	addr := testutil.AccAddress(t)

	createMsg := types.MsgCreateProvider{
		Owner:   addr,
		HostURI: "host",
	}

	updateMsg := types.MsgUpdateProvider{
		Owner:   addr,
		HostURI: "host",
	}

	err := testCtx.keeper.Create(testCtx.ctx, types.Provider(createMsg))
	require.NoError(t, err)

	_, err = testCtx.handler(testCtx.ctx, updateMsg)
	require.NoError(t, err)
}

func TestProviderUpdateNotExisting(t *testing.T) {
	testCtx := setupTestContext(t)
	msg := types.MsgUpdateProvider{
		Owner:   testutil.AccAddress(t),
		HostURI: "host",
	}

	_, err := testCtx.handler(testCtx.ctx, msg)
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}

func TestProviderDeleteExisting(t *testing.T) {
	testCtx := setupTestContext(t)

	addr := testutil.AccAddress(t)

	createMsg := types.MsgCreateProvider{
		Owner:   addr,
		HostURI: "host",
	}

	deleteMsg := types.MsgDeleteProvider{
		Owner: addr,
	}

	err := testCtx.keeper.Create(testCtx.ctx, types.Provider(createMsg))
	require.NoError(t, err)

	_, err = testCtx.handler(testCtx.ctx, deleteMsg)
	require.Error(t, err)
}

func TestProviderDeleteNonExisting(t *testing.T) {
	testCtx := setupTestContext(t)
	msg := types.MsgDeleteProvider{
		Owner: testutil.AccAddress(t),
	}

	_, err := testCtx.handler(testCtx.ctx, msg)
	require.Error(t, err)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}
