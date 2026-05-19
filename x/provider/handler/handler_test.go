package handler_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/provider/v1beta4"
	akashtypes "pkg.akt.dev/go/node/types/attributes/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
	mkeeper "pkg.akt.dev/node/v2/x/market/keeper"
	"pkg.akt.dev/node/v2/x/provider/handler"
	"pkg.akt.dev/node/v2/x/provider/keeper"
)

const (
	emailValid = "test@example.com"
)

type testSuite struct {
	t       testing.TB
	ctx     sdk.Context
	keeper  keeper.IKeeper
	mkeeper mkeeper.IKeeper
	handler baseapp.MsgServiceHandler
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
		testutil.EnsureEvent(t, res.Events, &types.EventProviderCreated{Owner: msg.Owner})
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
		Info: types.Info{
			EMail:   emailValid,
			Website: testutil.Hostname(t),
		},
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		testutil.EnsureEvent(t, res.Events, &types.EventProviderCreated{Owner: msg.Owner})
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
		testutil.EnsureEvent(t, res.Events, &types.EventProviderUpdated{Owner: updateMsg.Owner})
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

	updateMsg.Attributes = nil
	res, err := suite.handler(suite.ctx, updateMsg)
	require.NoError(t, err)
	require.NotNil(t, res)
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

	t.Run("ensure event created", func(_ *testing.T) {
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

func TestProviderUpdateParams(t *testing.T) {
	suite := setupTestSuite(t)

	params := keeper.DefaultParams()
	params.MaintenanceMaxDuration = 24 * time.Hour

	msg := &types.MsgUpdateParams{
		Authority: suite.keeper.GetAuthority(),
		Params:    params,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, params, suite.keeper.GetParams(suite.ctx))

	msg.Authority = testutil.AccAddress(t).String()
	res, err = suite.handler(suite.ctx, msg)
	require.Error(t, err)
	require.Nil(t, res)
}

func TestProviderMaintenanceLifecycle(t *testing.T) {
	suite := setupTestSuite(t)
	suite.ctx = suite.ctx.WithBlockTime(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	prov := testutil.Provider(t)
	err := suite.keeper.Create(suite.ctx, prov)
	require.NoError(t, err)

	openMsg := &types.MsgOpenProviderMaintenance{
		Provider:        prov.Owner,
		MaintenanceType: types.ProviderMaintenanceType_provider_maintenance_type_planned,
		StartsAt:        suite.ctx.BlockTime().Add(time.Hour),
		ExpectedEndsAt:  suite.ctx.BlockTime().Add(2 * time.Hour),
		MetadataHash:    []byte("hash"),
	}

	res, err := suite.handler(suite.ctx, openMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	record, found := suite.keeper.GetMaintenance(suite.ctx, 1)
	require.True(t, found)
	require.Equal(t, openMsg.Provider, record.Provider)
	require.Equal(t, openMsg.MaintenanceType, record.MaintenanceType)
	require.Equal(t, openMsg.StartsAt, record.StartsAt)
	require.Equal(t, openMsg.ExpectedEndsAt, record.ExpectedEndsAt)
	require.Equal(t, openMsg.MetadataHash, record.MetadataHash)

	provider, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)
	activeID, found := suite.keeper.GetActiveMaintenanceID(suite.ctx, provider)
	require.True(t, found)
	require.Equal(t, uint64(1), activeID)

	res, err = suite.handler(suite.ctx, openMsg)
	require.Error(t, err)
	require.Nil(t, res)

	closeMsg := &types.MsgCloseProviderMaintenance{
		Provider:      prov.Owner,
		MaintenanceID: 1,
	}
	closeCtx := suite.ctx.WithBlockTime(suite.ctx.BlockTime().Add(90 * time.Minute))
	res, err = suite.handler(closeCtx, closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	record, found = suite.keeper.GetMaintenance(closeCtx, 1)
	require.True(t, found)
	require.NotNil(t, record.ClosedAt)
	require.Equal(t, closeCtx.BlockTime(), *record.ClosedAt)

	_, found = suite.keeper.GetActiveMaintenanceID(closeCtx, provider)
	require.False(t, found)
}

func TestProviderMaintenanceOpenValidation(t *testing.T) {
	suite := setupTestSuite(t)
	suite.ctx = suite.ctx.WithBlockTime(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	prov := testutil.Provider(t)
	err := suite.keeper.Create(suite.ctx, prov)
	require.NoError(t, err)

	valid := &types.MsgOpenProviderMaintenance{
		Provider:        prov.Owner,
		MaintenanceType: types.ProviderMaintenanceType_provider_maintenance_type_planned,
		StartsAt:        suite.ctx.BlockTime().Add(time.Hour),
		ExpectedEndsAt:  suite.ctx.BlockTime().Add(2 * time.Hour),
	}

	cases := []struct {
		name string
		msg  *types.MsgOpenProviderMaintenance
	}{
		{
			name: "unknown provider",
			msg: &types.MsgOpenProviderMaintenance{
				Provider:        testutil.AccAddress(t).String(),
				MaintenanceType: valid.MaintenanceType,
				StartsAt:        valid.StartsAt,
				ExpectedEndsAt:  valid.ExpectedEndsAt,
			},
		},
		{
			name: "unspecified type",
			msg: &types.MsgOpenProviderMaintenance{
				Provider:       valid.Provider,
				StartsAt:       valid.StartsAt,
				ExpectedEndsAt: valid.ExpectedEndsAt,
			},
		},
		{
			name: "bad time order",
			msg: &types.MsgOpenProviderMaintenance{
				Provider:        valid.Provider,
				MaintenanceType: valid.MaintenanceType,
				StartsAt:        valid.ExpectedEndsAt,
				ExpectedEndsAt:  valid.StartsAt,
			},
		},
		{
			name: "duration too long",
			msg: &types.MsgOpenProviderMaintenance{
				Provider:        valid.Provider,
				MaintenanceType: valid.MaintenanceType,
				StartsAt:        valid.StartsAt,
				ExpectedEndsAt:  valid.StartsAt.Add(8 * 24 * time.Hour),
			},
		},
		{
			name: "lookahead too far",
			msg: &types.MsgOpenProviderMaintenance{
				Provider:        valid.Provider,
				MaintenanceType: valid.MaintenanceType,
				StartsAt:        suite.ctx.BlockTime().Add(91 * 24 * time.Hour),
				ExpectedEndsAt:  suite.ctx.BlockTime().Add(91*24*time.Hour + time.Hour),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := suite.handler(suite.ctx, tc.msg)
			require.Error(t, err)
			require.Nil(t, res)
		})
	}
}
