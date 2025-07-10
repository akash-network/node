package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/audit/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/app"
	"pkg.akt.dev/node/x/audit/keeper"
)

type grpcTestSuite struct {
	t      *testing.T
	app    *app.AkashApp
	ctx    sdk.Context
	keeper keeper.Keeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	suite := &grpcTestSuite{
		t: t,
	}

	suite.app = app.Setup(app.WithGenesis(app.GenesisStateWithValSet))

	suite.ctx, suite.keeper = setupKeeper(t)
	querier := keeper.Querier{Keeper: suite.keeper}

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = types.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryProvider(t *testing.T) {
	suite := setupTest(t)

	// creating provider
	id, provider := testutil.AuditedProvider(t)
	err := suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, id, provider.Attributes)
	require.NoError(t, err)

	var req *types.QueryProviderAuditorRequest
	var expProvider types.AuditedProvider

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryProviderAuditorRequest{}
			},
			false,
		},
		{
			"provider not found",
			func() {
				req = &types.QueryProviderAuditorRequest{
					Owner:   testutil.AccAddress(t).String(),
					Auditor: testutil.AccAddress(t).String(),
				}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryProviderAuditorRequest{
					Auditor: provider.Auditor,
					Owner:   provider.Owner,
				}
				expProvider = provider
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

			res, err := suite.queryClient.ProviderAuditorAttributes(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expProvider, res.Providers[0])
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
	id1, provider := testutil.AuditedProvider(t)
	err := suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, id1, provider.Attributes)
	require.NoError(t, err)

	id2, provider2 := testutil.AuditedProvider(t)
	err = suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, id2, provider2.Attributes)
	require.NoError(t, err)

	var req *types.QueryAllProvidersAttributesRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query all providers without pagination",
			func() {
				req = &types.QueryAllProvidersAttributesRequest{}
			},
			2,
		},
		{
			"query orders with pagination",
			func() {
				req = &types.QueryAllProvidersAttributesRequest{
					Pagination: &sdkquery.PageRequest{
						Limit:  1,
						Offset: 1,
					},
				}
			},
			1,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := suite.ctx

			res, err := suite.queryClient.AllProvidersAttributes(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Providers))
		})
	}
}
