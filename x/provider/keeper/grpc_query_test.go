package keeper_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/provider/v1beta4"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/provider/keeper"
)

type grpcTestSuite struct {
	t      *testing.T
	app    *app.AkashApp
	ctx    sdk.Context
	keeper keeper.IKeeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &grpcTestSuite{
		t:      t,
		app:    ssuite.App(),
		ctx:    ssuite.Context(),
		keeper: ssuite.ProviderKeeper(),
	}

	querier := suite.keeper.NewQuerier()
	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = types.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryProvider(t *testing.T) {
	suite := setupTest(t)

	// creating provider
	provider := testutil.Provider(t)
	err := suite.keeper.Create(suite.ctx, provider)
	require.NoError(t, err)

	var (
		req         *types.QueryProviderRequest
		expProvider types.Provider
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryProviderRequest{}
			},
			false,
		},
		{
			"provider not found",
			func() {
				req = &types.QueryProviderRequest{Owner: testutil.AccAddress(t).String()}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryProviderRequest{Owner: provider.Owner}
				expProvider = provider
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

			res, err := suite.queryClient.Provider(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expProvider, res.Provider)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func TestGRPCQueryProviders(t *testing.T) {
	suite := setupTest(t)

	// creating providers
	provider := testutil.Provider(t)
	err := suite.keeper.Create(suite.ctx, provider)
	require.NoError(t, err)

	provider2 := testutil.Provider(t)
	err = suite.keeper.Create(suite.ctx, provider2)
	require.NoError(t, err)

	var req *types.QueryProvidersRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query all providers without pagination",
			func() {
				req = &types.QueryProvidersRequest{}
			},
			2,
		},
		{
			"query orders with pagination",
			func() {
				req = &types.QueryProvidersRequest{Pagination: &sdkquery.PageRequest{Limit: 1, Offset: 1}}
			},
			1,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

			res, err := suite.queryClient.Providers(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Providers))
		})
	}
}

func TestGRPCQueryProviderRegistration(t *testing.T) {
	suite := setupTest(t)
	provider := testutil.Provider(t)
	err := suite.keeper.Create(suite.ctx, provider)
	require.NoError(t, err)

	res, err := suite.queryClient.Registration(suite.ctx, &types.QueryRegistrationRequest{
		Provider: provider.Owner,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, provider.Owner, res.Registration.Owner)
	require.Equal(t, suite.ctx.BlockTime(), res.Registration.RegisteredAt)
}

func TestGRPCQueryProviderMaintenances(t *testing.T) {
	suite := setupTest(t)
	provider := testutil.Provider(t)
	err := suite.keeper.Create(suite.ctx, provider)
	require.NoError(t, err)

	record := types.ProviderMaintenanceRecord{
		ID:              suite.keeper.AllocateMaintenanceID(suite.ctx),
		Provider:        provider.Owner,
		MaintenanceType: types.ProviderMaintenanceType_provider_maintenance_type_planned,
		StartsAt:        suite.ctx.BlockTime().Add(time.Hour),
		ExpectedEndsAt:  suite.ctx.BlockTime().Add(2 * time.Hour),
		OpenedAt:        suite.ctx.BlockTime(),
	}
	err = suite.keeper.SetMaintenance(suite.ctx, record)
	require.NoError(t, err)

	res, err := suite.queryClient.ProviderMaintenance(suite.ctx, &types.QueryProviderMaintenanceRequest{
		Provider:      provider.Owner,
		MaintenanceId: record.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, record, res.Maintenance.Record)
	require.Equal(t, types.ProviderMaintenanceStatus_provider_maintenance_status_scheduled, res.Maintenance.Status)

	list, err := suite.queryClient.ProviderMaintenances(suite.ctx, &types.QueryProviderMaintenancesRequest{
		Provider:     provider.Owner,
		StatusFilter: types.ProviderMaintenanceStatus_provider_maintenance_status_scheduled,
	})
	require.NoError(t, err)
	require.NotNil(t, list)
	require.Len(t, list.Maintenance, 1)
	require.Equal(t, record, list.Maintenance[0].Record)
}
