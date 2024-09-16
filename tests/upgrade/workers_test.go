//go:build e2e.upgrade

package upgrade

import (
	"context"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/cli/flags"
	cltypes "pkg.akt.dev/go/node/client/types"

	"pkg.akt.dev/node/app"
	uttypes "pkg.akt.dev/node/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v1.0.0", &postUpgrade{})
}

type postUpgrade struct{}

var _ uttypes.TestWorker = (*postUpgrade)(nil)

func (pu *postUpgrade) Run(ctx context.Context, t *testing.T, params uttypes.TestParams) {
	encodingConfig := app.MakeEncodingConfig()

	rpcClient, err := sdkclient.NewClientFromNode(params.Node)
	require.NoError(t, err)

	cctx := sdkclient.Context{}.
		WithCodec(encodingConfig.Marshaler).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastBlock).
		WithHomeDir(params.Home).
		WithChainID(params.ChainID).
		WithNodeURI(params.Node).
		WithClient(rpcClient).
		WithSkipConfirmation(true).
		WithFrom(params.From).
		WithFromName(params.From).
		WithFromAddress(params.FromAddress).
		WithKeyringDir(params.Home).
		WithSignModeStr(flags.SignModeDirect).
		WithSimulation(false)

	kr, err := sdkclient.NewKeyringFromBackend(cctx, params.KeyringBackend)
	require.NoError(t, err)

	cctx = cctx.WithKeyring(kr)

	opts := []cltypes.ClientOption{
		cltypes.WithGasPrices("0.025uakt"),
		cltypes.WithGas(cltypes.GasSetting{Simulate: false, Gas: 1000000}),
		cltypes.WithGasAdjustment(2),
	}

	mcl, err := cli.DiscoverClient(ctx, cctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	paramsResp, err := mcl.Query().Staking().Params(ctx, &stakingtypes.QueryParamsRequest{})
	require.NoError(t, err)
	require.NotNil(t, paramsResp)

	require.True(t, paramsResp.Params.MinCommissionRate.GTE(sdk.NewDecWithPrec(5, 2)), "per upgrade v1.0.0 MinCommissionRate should be 5%")

	// operator address is taken from testnetify
	opAddr, err := sdk.AccAddressFromHexUnsafe("20DDEBCF73B805ACDC88277B472382FC9DEA8CBC")
	require.NoError(t, err)

	comVal := sdk.NewDecWithPrec(4, 2)

	valResp, err := mcl.Query().Staking().Validator(ctx, &stakingtypes.QueryValidatorRequest{ValidatorAddr: sdk.ValAddress(opAddr).String()})
	require.NoError(t, err)

	tx := stakingtypes.NewMsgEditValidator(sdk.ValAddress(opAddr), valResp.Validator.Description, &comVal)
	broadcastResp, err := mcl.Tx().BroadcastMsgs(ctx, []sdk.Msg{tx})
	require.Error(t, err)
	require.NotNil(t, broadcastResp)

	require.IsType(t, &sdk.TxResponse{}, broadcastResp)
	txResp := broadcastResp.(*sdk.TxResponse)
	require.NotEqual(t, uint32(0), txResp.Code, "update validator commission should fail if new value is < 5%")

	comVal = sdk.NewDecWithPrec(6, 2)

	tx = stakingtypes.NewMsgEditValidator(sdk.ValAddress(opAddr), valResp.Validator.Description, &comVal)

	broadcastResp, err = mcl.Tx().BroadcastMsgs(ctx, []sdk.Msg{tx})
	require.NoError(t, err)
	require.NotNil(t, broadcastResp)

	require.IsType(t, &sdk.TxResponse{}, broadcastResp)
	txResp = broadcastResp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "update validator commission should pass if new value is >= 5%")
}
