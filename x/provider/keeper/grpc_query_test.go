package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	"github.com/akash-network/node/x/provider/keeper"
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
			ctx := sdk.WrapSDKContext(suite.ctx)

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
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Providers(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Providers))
		})
	}
}
