//go:build e2e.upgrade

// Package v0_26_0
// nolint revive
package v0_26_0

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
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

	pqc := proposal.NewQueryClient(cctx)
	res, err := pqc.Params(ctx, &proposal.QueryParamsRequest{
		Subspace: stakingtypes.ModuleName,
		Key:      string(stakingtypes.KeyMaxValidators),
	})
	require.NoError(t, err)

	maxValidators, err := strconv.ParseInt(strings.Trim(res.Param.Value, "\""), 10, 32)
	require.NoError(t, err)

	res, err = pqc.Params(ctx, &proposal.QueryParamsRequest{
		Subspace: astaking.ModuleName,
		Key:      string(astaking.KeyMinCommissionRate),
	})
	require.NoError(t, err)

	minCommission, err := sdk.NewDecFromStr(strings.Trim(res.Param.Value, "\""))
	require.NoError(t, err)

	qc := stakingtypes.NewQueryClient(cctx)

	var pkey []byte

	validators := make(stakingtypes.Validators, 0, maxValidators)

	for {
		var pgn *query.PageRequest
		if pkey != nil {
			pgn = &query.PageRequest{
				Key: pkey,
			}
		}

		result, err := qc.Validators(ctx, &stakingtypes.QueryValidatorsRequest{
			Pagination: pgn,
		})
		require.NoError(t, err)

		validators = append(validators, result.Validators...)

		if pg := result.Pagination; pg != nil && len(pg.NextKey) > 0 {
			pkey = pg.NextKey
		} else {
			break
		}
	}

	for _, validator := range validators {
		assert.True(t, validator.Commission.Rate.GTE(minCommission),
			fmt.Sprintf("invalid commission Rate for validator (%s). (%s%%) < (%s%%)MinCommission",
				validator.OperatorAddress, validator.Commission.Rate.String(), minCommission.String()))

		assert.True(t, validator.Commission.MaxRate.GTE(minCommission),
			fmt.Sprintf("invalid commission MaxRate for validator (%s). (%s%%) < (%s%%)MinCommission",
				validator.OperatorAddress, validator.Commission.MaxRate.String(), minCommission.String()))
	}
}
