//go:build e2e.upgrade

package upgrade

import (
	"context"
	"fmt"
	"os"
	"testing"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/require"

	"github.com/CosmWasm/wasmd/x/wasm/ioutils"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	client "pkg.akt.dev/go/node/client/discovery"
	cltypes "pkg.akt.dev/go/node/client/types"
	clt "pkg.akt.dev/go/node/client/v1beta3"

	cflags "pkg.akt.dev/go/cli/flags"
	arpcclient "pkg.akt.dev/go/node/client"
	"pkg.akt.dev/go/sdkutil"

	akash "pkg.akt.dev/node/v2/app"
	uttypes "pkg.akt.dev/node/v2/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v2.0.0", &postUpgrade{})
}

type postUpgrade struct {
	cl arpcclient.Client
}

var _ uttypes.TestWorker = (*postUpgrade)(nil)

func (pu *postUpgrade) Run(ctx context.Context, t *testing.T, params uttypes.TestParams) {
	encCfg := sdkutil.MakeEncodingConfig()
	akash.ModuleBasics().RegisterInterfaces(encCfg.InterfaceRegistry)
	rpcClient, err := arpcclient.NewClient(ctx, params.Node)
	require.NoError(t, err)

	cctx := sdkclient.Context{}.
		WithCodec(encCfg.Codec).
		WithInterfaceRegistry(encCfg.InterfaceRegistry).
		WithTxConfig(encCfg.TxConfig).
		WithLegacyAmino(encCfg.Amino).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(cflags.BroadcastBlock).
		WithHomeDir(params.Home).
		WithChainID(params.ChainID).
		WithNodeURI(params.Node).
		WithClient(rpcClient).
		WithSkipConfirmation(true).
		WithFrom(params.From).
		WithKeyringDir(params.Home).
		WithSignModeStr("direct")

	kr, err := sdkclient.NewKeyringFromBackend(cctx, params.KeyringBackend)
	require.NoError(t, err)

	cctx = cctx.WithKeyring(kr)

	info, err := kr.Key(params.From)
	require.NoError(t, err)

	mainAddr, err := info.GetAddress()
	require.NoError(t, err)

	mainCctx := cctx.WithFromName(info.Name).
		WithFromAddress(mainAddr)

	opts := []cltypes.ClientOption{
		cltypes.WithGasPrices("0.025uakt"),
		cltypes.WithGas(cltypes.GasSetting{Simulate: false, Gas: 1000000}),
		cltypes.WithGasAdjustment(2),
	}

	mcl, err := client.DiscoverClient(ctx, mainCctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	// should not be able to deploy smart contract directly
	wasm, err := os.ReadFile(fmt.Sprintf("%s/tests/upgrade/testdata/hackatom.wasm", params.SourceDir))
	require.NoError(t, err)

	// gzip the wasm file
	if ioutils.IsWasm(wasm) {
		wasm, err = ioutils.GzipIt(wasm)
		require.NoError(t, err)
	} else {
		require.True(t, ioutils.IsGzip(wasm))
	}

	msg := &wasmtypes.MsgStoreCode{
		Sender:                mainAddr.String(),
		WASMByteCode:          wasm,
		InstantiatePermission: &wasmtypes.AllowNobody,
	}

	err = msg.ValidateBasic()
	require.NoError(t, err)

	resp, err := mcl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
	require.Error(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	require.ErrorIs(t, err, sdkerrors.ErrUnauthorized)

	govMsg, err := govv1.NewMsgSubmitProposal([]sdk.Msg{msg}, sdk.Coins{sdk.NewInt64Coin("uakt", 1000000000)}, mainCctx.GetFromAddress().String(), "", "test wasm store", "test wasm store", false)
	require.NoError(t, err)

	// sending contract via gov with sender not as the gov module account should fail as well
	resp, err = mcl.Tx().BroadcastMsgs(ctx, []sdk.Msg{govMsg})
	require.Error(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)

	qResp, err := mcl.Query().Auth().ModuleAccountByName(ctx, &authtypes.QueryModuleAccountByNameRequest{Name: "gov"})
	require.NoError(t, err)
	require.NotNil(t, qResp)

	var acc sdk.AccountI
	err = encCfg.InterfaceRegistry.UnpackAny(qResp.Account, &acc)
	require.NoError(t, err)
	macc, ok := acc.(sdk.ModuleAccountI)
	require.True(t, ok)

	err = encCfg.InterfaceRegistry.UnpackAny(qResp.Account, &macc)
	require.NoError(t, err)
	msg.Sender = macc.GetAddress().String()

	govMsg, err = govv1.NewMsgSubmitProposal([]sdk.Msg{msg}, sdk.Coins{sdk.NewInt64Coin("uakt", 1000000000)}, mainCctx.GetFromAddress().String(), "", "test wasm store", "test wasm store", false)
	require.NoError(t, err)

	// sending contract via gov with sender as the gov module account shall pass
	resp, err = mcl.Tx().BroadcastMsgs(ctx, []sdk.Msg{govMsg}, clt.WithGas(cltypes.GasSetting{Simulate: true}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
}
