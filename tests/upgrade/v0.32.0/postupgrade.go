//go:build e2e.upgrade

// Package v0_32_0
// nolint revive
package v0_32_0

import (
	"context"
	"testing"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	cltypes "github.com/akash-network/akash-api/go/node/client/types"
	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	"pkg.akt.dev/akashd/app"
	"pkg.akt.dev/akashd/client"
	uttypes "pkg.akt.dev/akashd/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v0.32.0", &postUpgrade{})
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
		WithKeyringDir(params.Home)

	kr, err := sdkclient.NewKeyringFromBackend(cctx, params.KeyringBackend)
	require.NoError(t, err)

	info, err := kr.Key(params.From)
	require.NoError(t, err)

	cctx = cctx.WithFromName(info.GetName()).
		WithFromAddress(info.GetAddress()).
		WithKeyring(kr)

	cl, err := client.DiscoverClient(ctx, cctx, cltypes.WithGasPrices("0.0025uakt"), cltypes.WithGas(flags.GasSetting{Simulate: false, Gas: 100000}))
	require.NoError(t, err)
	require.NotNil(t, cl)

	cmsg := &ptypes.MsgCreateProvider{
		Owner:   cctx.GetFromAddress().String(),
		HostURI: "https://example.com:443",
		Info:    ptypes.ProviderInfo{},
		Attributes: types.Attributes{
			{
				Key:   "test1",
				Value: "test1",
			},
		},
	}

	err = cmsg.ValidateBasic()
	require.NoError(t, err)

	resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{cmsg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)

	txResp := resp.(*sdk.TxResponse)

	require.Equal(t, uint32(0), txResp.Code)

	pmsg := &ptypes.MsgUpdateProvider{
		Owner:   cctx.GetFromAddress().String(),
		HostURI: "https://example.com:443",
		Info:    ptypes.ProviderInfo{},
		Attributes: types.Attributes{
			{
				Key:   "test1",
				Value: "test1",
			},
		},
	}

	err = cmsg.ValidateBasic()
	require.NoError(t, err)

	resp, err = cl.Tx().Broadcast(ctx, []sdk.Msg{pmsg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)

	txResp = resp.(*sdk.TxResponse)

	require.Equal(t, uint32(0), txResp.Code)
	require.LessOrEqual(t, txResp.GasUsed, int64(100000))
}
