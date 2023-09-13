//go:build e2e.upgrade

// Package v0_26_0
// nolint revive
package v0_26_0

import (
	"context"
	"fmt"
	"strings"
	"testing"

	// v1beta2dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta2"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astaking "github.com/akash-network/akash-api/go/node/staking/v1beta3"

	"github.com/akash-network/node/app"
	uttypes "github.com/akash-network/node/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v0.26.0", &postUpgrade{})
}

type postUpgrade struct{}

var _ uttypes.TestWorker = (*postUpgrade)(nil)

func (pu *postUpgrade) Run(ctx context.Context, t *testing.T, params uttypes.TestParams) {
	encodingConfig := app.MakeEncodingConfig()

	rpcClient, err := client.NewClientFromNode(params.Node)
	require.NoError(t, err)

	cctx := client.Context{}.
		WithCodec(encodingConfig.Marshaler).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastBlock).
		WithHomeDir(params.Home).
		WithChainID(params.ChainID).
		WithNodeURI(params.Node).
		WithClient(rpcClient)

	kr, err := client.NewKeyringFromBackend(cctx, params.KeyringBackend)
	require.NoError(t, err)

	cctx = cctx.WithKeyring(kr)

	validateValidatorsCommission(ctx, cctx, t)
	validateDeploymentAuthz(ctx, cctx, t)
}

func validateDeploymentAuthz(ctx context.Context, cctx client.Context, t *testing.T) {
	authQc := authtypes.NewQueryClient(cctx)
	authzQc := authz.NewQueryClient(cctx)

	var pkey []byte

	accs := make([]sdk.AccAddress, 0, 100000)

	for {
		var pgn *query.PageRequest
		if pkey != nil {
			pgn = &query.PageRequest{
				Key: pkey,
			}
		}

		result, err := authQc.Accounts(ctx, &authtypes.QueryAccountsRequest{
			Pagination: pgn,
		})

		require.NoError(t, err)

		for _, accI := range result.Accounts {
			var acc authtypes.AccountI
			err := cctx.Codec.UnpackAny(accI, &acc)
			require.NoError(t, err)

			accs = append(accs, acc.GetAddress())
		}

		if pg := result.Pagination; pg != nil && len(pg.NextKey) > 0 {
			pkey = pg.NextKey
		} else {
			break
		}
	}

	for _, acc := range accs {
		result, err := authzQc.GranterGrants(ctx, &authz.QueryGranterGrantsRequest{Granter: acc.String()})
		require.NoError(t, err)

		if len(result.Grants) == 0 {
			continue
		}

		for _, grant := range result.Grants {
			assert.NotEqual(t,
				"/akash.deployment.v1beta2.DepositDeploymentAuthorization",
				grant.Authorization.GetTypeUrl(),
				"detected non-migrated v1beta2.DepositDeploymentAuthorization. granter: (%s), grantee: (%s)", grant.Granter, grant.Grantee)
			// authzOld := &v1beta2dtypes.DepositDeploymentAuthorization{}
			//
			// err := cctx.Codec.UnpackAny(grant.Authorization, authzOld)
			// assert.Error(t, err, "detected non-migrated v1beta2.DepositDeploymentAuthorization. granter: (%s), grantee: (%s)", grant.Granter, grant.Grantee)
		}
	}
}

func validateValidatorsCommission(ctx context.Context, cctx client.Context, t *testing.T) {
	pqc := proposal.NewQueryClient(cctx)
	res, err := pqc.Params(ctx, &proposal.QueryParamsRequest{
		Subspace: stakingtypes.ModuleName,
		Key:      string(stakingtypes.KeyMaxValidators),
	})
	require.NoError(t, err)

	require.NoError(t, err)

	res, err = pqc.Params(ctx, &proposal.QueryParamsRequest{
		Subspace: astaking.ModuleName,
		Key:      string(astaking.KeyMinCommissionRate),
	})
	require.NoError(t, err)

	minCommission, err := sdk.NewDecFromStr(strings.Trim(res.Param.Value, "\""))
	require.NoError(t, err)

	sqc := stakingtypes.NewQueryClient(cctx)

	var pkey []byte

	for {
		var pgn *query.PageRequest
		if pkey != nil {
			pgn = &query.PageRequest{
				Key: pkey,
			}
		}

		result, err := sqc.Validators(ctx, &stakingtypes.QueryValidatorsRequest{
			Pagination: pgn,
		})
		require.NoError(t, err)

		for _, validator := range result.Validators {
			assert.True(t, validator.Commission.Rate.GTE(minCommission),
				fmt.Sprintf("invalid commission Rate for validator (%s). (%s%%) < (%s%%)MinCommission",
					validator.OperatorAddress, validator.Commission.Rate.String(), minCommission.String()))

			assert.True(t, validator.Commission.MaxRate.GTE(minCommission),
				fmt.Sprintf("invalid commission MaxRate for validator (%s). (%s%%) < (%s%%)MinCommission",
					validator.OperatorAddress, validator.Commission.MaxRate.String(), minCommission.String()))
		}

		if pg := result.Pagination; pg != nil && len(pg.NextKey) > 0 {
			pkey = pg.NextKey
		} else {
			break
		}
	}
}
