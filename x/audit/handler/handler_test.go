package handler_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	types "github.com/akash-network/akash-api/go/node/audit/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/x/audit/handler"
	"github.com/akash-network/node/x/audit/keeper"
)

type testSuite struct {
	t       testing.TB
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	keeper  keeper.Keeper
	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testSuite {
	suite := &testSuite{
		t: t,
	}

	aKey := sdk.NewTransientStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db)
	suite.ms.MountStoreWithDB(aKey, sdk.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, tmproto.Header{}, true, testutil.Logger(t))

	suite.keeper = keeper.NewKeeper(types.ModuleCdc, aKey)

	suite.handler = handler.NewHandler(suite.keeper)

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	_, err := suite.handler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestProviderSignNew(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)

	msg := &types.MsgSignProviderAttributes{
		Owner:      owner.String(),
		Auditor:    auditor.String(),
		Attributes: testutil.Attributes(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	prov, exists := suite.keeper.GetProviderAttributes(suite.ctx, owner)
	require.True(t, exists)
	require.Equal(t, prov, msgSignProviderAttributesToResponse(msg))
}

func TestProviderSignAndUpdate(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)
	auditor := testutil.AccAddress(t)
	originAttr := testutil.Attributes(t)

	msg := &types.MsgSignProviderAttributes{
		Owner:      owner.String(),
		Auditor:    auditor.String(),
		Attributes: originAttr,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	msg.Attributes = testutil.Attributes(t)
	res, err = suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)
	prov, exists := suite.keeper.GetProviderAttributes(suite.ctx, owner)

	msg.Attributes = append(msg.Attributes, originAttr...)
	// add some more attributes.
	// if part below starts to fail it is due to testutil.Attributes
	// may produce same attributes between multiple calls to it
	require.True(t, exists)
	require.Equal(t, prov, msgSignProviderAttributesToResponse(msg))
}

func TestProviderDeleteNonExisting(t *testing.T) {
	suite := setupTestSuite(t)
	msg := &types.MsgDeleteProviderAttributes{
		Auditor: testutil.AccAddress(t).String(),
		Owner:   testutil.AccAddress(t).String(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Error(t, err)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}

func TestProviderDeleteFull(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &types.MsgSignProviderAttributes{
		Owner:      testutil.AccAddress(t).String(),
		Auditor:    testutil.AccAddress(t).String(),
		Attributes: testutil.Attributes(t),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	res, err = suite.handler(suite.ctx, &types.MsgDeleteProviderAttributes{
		Auditor: msg.Auditor,
		Owner:   msg.Owner,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	res, err = suite.handler(suite.ctx, &types.MsgDeleteProviderAttributes{
		Auditor: msg.Auditor,
		Owner:   msg.Owner,
	})

	require.Error(t, err)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrProviderNotFound))
}

func TestProviderDeleteAttribute(t *testing.T) {
	suite := setupTestSuite(t)

	owner := testutil.AccAddress(t)

	msg := &types.MsgSignProviderAttributes{
		Owner:      owner.String(),
		Auditor:    testutil.AccAddress(t).String(),
		Attributes: testutil.Attributes(t),
	}

	// add one more attribute in case prev call to testutil.Attributes generated only one entry
	msg.Attributes = append(msg.Attributes, testutil.Attribute(t))

	sort.SliceStable(msg.Attributes, func(i, j int) bool {
		return msg.Attributes[i].Key < msg.Attributes[j].Key
	})

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	res, err = suite.handler(suite.ctx, &types.MsgDeleteProviderAttributes{
		Auditor: msg.Auditor,
		Owner:   msg.Owner,
		Keys:    []string{msg.Attributes[0].Key}, // remove first attribute
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	msg.Attributes = msg.Attributes[1:]
	prov, exists := suite.keeper.GetProviderAttributes(suite.ctx, owner)
	require.True(t, exists)
	require.Equal(t, prov, msgSignProviderAttributesToResponse(msg))
}

func msgSignProviderAttributesToResponse(msg *types.MsgSignProviderAttributes) types.Providers {
	// create handler sorts attributes, so do we to ensure same order

	sort.SliceStable(msg.Attributes, func(i, j int) bool {
		return msg.Attributes[i].Key < msg.Attributes[j].Key
	})

	return types.Providers{
		{
			Owner:      msg.Owner,
			Auditor:    msg.Auditor,
			Attributes: msg.Attributes,
		},
	}
}
