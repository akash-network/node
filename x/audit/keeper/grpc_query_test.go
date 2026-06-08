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

	"pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/x/audit/keeper"
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

	suite.app = app.Setup(app.WithHome(t.TempDir()), app.WithGenesis(app.GenesisStateWithValSet))

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

	expByOwner := map[string]types.AuditedProvider{
		provider.Owner:  provider,
		provider2.Owner: provider2,
	}

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

			for _, got := range res.Providers {
				// Regression guard: Owner must be a valid bech32 address, not raw
				// protobuf bytes from a mis-unmarshaled value.
				_, err := sdk.AccAddressFromBech32(got.Owner)
				require.NoErrorf(t, err, "owner %q is not a valid bech32 address", got.Owner)

				exp, ok := expByOwner[got.Owner]
				require.Truef(t, ok, "unexpected owner %q", got.Owner)
				require.Equal(t, exp.Auditor, got.Auditor)
				require.Equal(t, exp.Attributes, got.Attributes)
			}
		})
	}
}

func TestGRPCQueryAuditorAttributes(t *testing.T) {
	suite := setupTest(t)

	// Two providers under the same auditor.
	auditor := testutil.AccAddress(t)

	_, provider1 := testutil.AuditedProvider(t)
	id1 := types.ProviderID{Owner: testutil.AccAddress(t), Auditor: auditor}
	provider1.Owner = id1.Owner.String()
	provider1.Auditor = id1.Auditor.String()
	err := suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, id1, provider1.Attributes)
	require.NoError(t, err)

	_, provider2 := testutil.AuditedProvider(t)
	id2 := types.ProviderID{Owner: testutil.AccAddress(t), Auditor: auditor}
	provider2.Owner = id2.Owner.String()
	provider2.Auditor = id2.Auditor.String()
	err = suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, id2, provider2.Attributes)
	require.NoError(t, err)

	// A third provider under a different auditor, which must be filtered out.
	idOther, providerOther := testutil.AuditedProvider(t)
	err = suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, idOther, providerOther.Attributes)
	require.NoError(t, err)

	res, err := suite.queryClient.AuditorAttributes(suite.ctx, &types.QueryAuditorAttributesRequest{
		Auditor: auditor.String(),
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Providers, 2)

	expByOwner := map[string]types.AuditedProvider{
		provider1.Owner: provider1,
		provider2.Owner: provider2,
	}

	for _, got := range res.Providers {
		require.Equal(t, auditor.String(), got.Auditor)
		exp, ok := expByOwner[got.Owner]
		require.Truef(t, ok, "unexpected owner %q returned for auditor", got.Owner)
		require.Equal(t, exp.Attributes, got.Attributes)
	}
}

func TestGRPCQueryProviderAttributes(t *testing.T) {
	suite := setupTest(t)

	id, provider := testutil.AuditedProvider(t)
	err := suite.keeper.CreateOrUpdateProviderAttributes(suite.ctx, id, provider.Attributes)
	require.NoError(t, err)

	res, err := suite.queryClient.ProviderAttributes(suite.ctx, &types.QueryProviderAttributesRequest{
		Owner: provider.Owner,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Providers, 1)
	require.Equal(t, provider, res.Providers[0])
}
