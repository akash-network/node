//go:build e2e.upgrade

package upgrade

import (
	"context"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/cli/flags"
	cltypes "pkg.akt.dev/go/node/client/types"
	"pkg.akt.dev/go/node/client/v1beta3"

	"pkg.akt.dev/node/app"
	uttypes "pkg.akt.dev/node/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v1.0.0", &postUpgrade{})
}

type postUpgrade struct {
	cl v1beta3.Client
}

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

	pu.cl, err = cli.DiscoverClient(ctx, cctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, pu.cl)

	pu.testGov(ctx, t)

	pu.testStaking(ctx, t)
}

func (pu *postUpgrade) testGov(ctx context.Context, t *testing.T) {
	t.Logf("testing gov module")
	cctx := pu.cl.ClientContext()

	paramsResp, err := pu.cl.Query().Gov().Params(ctx, &govtypes.QueryParamsRequest{ParamsType: "deposit"})
	require.NoError(t, err)
	require.NotNil(t, paramsResp)

	// paramsResp.Params.ExpeditedMinDeposit.
	require.Equal(t, sdk.Coins{sdk.NewCoin("uakt", sdk.NewInt(2000000000))}.String(), sdk.Coins(paramsResp.Params.ExpeditedMinDeposit).String(), "ExpeditedMinDeposit must have 2000AKT")
	require.Equal(t, paramsResp.Params.MinInitialDepositRatio, sdk.NewDecWithPrec(40, 2).String(), "MinInitialDepositRatio must be 40%")

	opAddr := sdk.ValAddress(cctx.FromAddress)

	comVal := sdk.NewDecWithPrec(4, 2)

	valResp, err := pu.cl.Query().Staking().Validator(ctx, &stakingtypes.QueryValidatorRequest{ValidatorAddr: opAddr.String()})
	require.NoError(t, err)

	tx := stakingtypes.NewMsgEditValidator(opAddr, valResp.Validator.Description, &comVal)
	broadcastResp, err := pu.cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{tx})
	require.Error(t, err)
	require.NotNil(t, broadcastResp)

	require.IsType(t, &sdk.TxResponse{}, broadcastResp)
	txResp := broadcastResp.(*sdk.TxResponse)
	require.NotEqual(t, uint32(0), txResp.Code, "update validator commission should fail if new value is < 5%")
}

func (pu *postUpgrade) testStaking(ctx context.Context, t *testing.T) {
	t.Logf("testing staking module")

	cctx := pu.cl.ClientContext()

	paramsResp, err := pu.cl.Query().Staking().Params(ctx, &stakingtypes.QueryParamsRequest{})
	require.NoError(t, err)
	require.NotNil(t, paramsResp)

	require.True(t, paramsResp.Params.MinCommissionRate.GTE(sdk.NewDecWithPrec(5, 2)), "per upgrade v1.0.0 MinCommissionRate should be 5%")

	opAddr := sdk.ValAddress(cctx.FromAddress)

	comVal := sdk.NewDecWithPrec(4, 2)

	valResp, err := pu.cl.Query().Staking().Validator(ctx, &stakingtypes.QueryValidatorRequest{ValidatorAddr: opAddr.String()})
	require.NoError(t, err)

	tx := stakingtypes.NewMsgEditValidator(opAddr, valResp.Validator.Description, &comVal)
	broadcastResp, err := pu.cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{tx})
	require.Error(t, err)
	require.NotNil(t, broadcastResp)

	require.IsType(t, &sdk.TxResponse{}, broadcastResp)
	txResp := broadcastResp.(*sdk.TxResponse)
	require.NotEqual(t, uint32(0), txResp.Code, "update validator commission should fail if new value is < 5%")
}
