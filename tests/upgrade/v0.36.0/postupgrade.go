//go:build e2e.upgrade

// Package v0_36_0
// nolint revive
package v0_36_0

import (
	"context"
	"testing"
	"time"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/cosmos/go-bip39"

	cltypes "github.com/akash-network/akash-api/go/node/client/types"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/client"
	"github.com/akash-network/node/cmd/common"
	"github.com/akash-network/node/sdl"
	uttypes "github.com/akash-network/node/tests/upgrade/types"
)

const (
	mnemonicEntropySize = 256
)

const (
	testSDL = `
---
version: "2.0"
services:
  web:
    image: nginx
    expose:
      - port: 80
        accept:
          - ahostname.com
        to:
          - global: true
      - port: 12345
        to:
          - global: true
        proto: udp
profiles:
  compute:
    web:
      resources:
        cpu:
          units: "100m"
        memory:
          size: "128Mi"
        storage:
          size: "1Gi"
  placement:
    westcoast:
      attributes:
        region: us-west
      signedBy:
        anyOf:
          - 1
          - 2
        allOf:
          - 3
          - 4
      pricing:
        web:
          denom: uakt
          amount: 50
deployment:
  web:
    westcoast:
      profile: web
      count: 2
`
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v0.36.0", &postUpgrade{})
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

	cctx = cctx.WithKeyring(kr)

	info, err := kr.Key(params.From)
	require.NoError(t, err)

	entropySeed, err := bip39.NewEntropy(mnemonicEntropySize)
	require.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropySeed)
	require.NoError(t, err)

	keyringAlgos, _ := kr.SupportedAlgorithms()

	algo, err := keyring.NewSigningAlgoFromString(string(hd.Secp256k1Type), keyringAlgos)

	coinType := sdk.GetConfig().GetCoinType()
	hdPath := hd.CreateHDPath(coinType, 0, 0).String()

	testAcc, err := kr.NewAccount("depl", mnemonic, "", hdPath, algo)
	require.NoError(t, err)

	mainCctx := cctx.WithFromName(info.GetName()).
		WithFromAddress(info.GetAddress())

	testCctx := cctx.WithFromName(testAcc.GetName()).
		WithFromAddress(testAcc.GetAddress()).
		WithFeeGranterAddress(info.GetAddress())

	opts := []cltypes.ClientOption{
		cltypes.WithGasPrices("0.025uakt"),
		cltypes.WithGas(flags.GasSetting{Simulate: false, Gas: 1000000}),
		cltypes.WithGasAdjustment(2),
	}

	mcl, err := client.DiscoverClient(ctx, mainCctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	basic := &feegrant.BasicAllowance{
		SpendLimit: sdk.Coins{sdk.NewCoin("uakt", sdk.NewInt(500000))},
	}

	fmsg, err := feegrant.NewMsgGrantAllowance(basic, info.GetAddress(), testAcc.GetAddress())
	require.NoError(t, err)

	// give test key deployment deposit authorization
	spendLimit := sdk.NewCoin("uakt", sdk.NewInt(10000000))
	authorization := dtypes.NewDepositDeploymentAuthorization(spendLimit)
	dmsg, err := authz.NewMsgGrant(info.GetAddress(), testAcc.GetAddress(), authorization, time.Now().AddDate(1, 0, 0))

	msgs := []sdk.Msg{
		fmsg,
		dmsg,
	}

	// authorize test account with feegrant
	resp, err := mcl.Tx().Broadcast(ctx, msgs)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp := resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	dcl, err := client.DiscoverClient(ctx, testCctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	tsdl, err := sdl.Read([]byte(testSDL))
	require.NoError(t, err)
	require.NotNil(t, tsdl)

	syncInfo, err := dcl.Node().SyncInfo(ctx)
	require.NoError(t, err)
	require.NotNil(t, syncInfo)
	require.False(t, syncInfo.CatchingUp)

	dID := dtypes.DeploymentID{
		Owner: testAcc.GetAddress().String(),
		DSeq:  uint64(syncInfo.LatestBlockHeight),
	}

	dVersion, err := tsdl.Version()
	require.NoError(t, err)

	dGroups, err := tsdl.DeploymentGroups()
	require.NoError(t, err)

	deposit, err := common.DetectDeposit(ctx, &pflag.FlagSet{}, dcl.Query(), "deployment", "MinDeposits")
	require.NoError(t, err)

	qresp, err := dcl.Query().Bank().Balance(ctx, &banktypes.QueryBalanceRequest{
		Denom:   "uakt",
		Address: testAcc.GetAddress().String(),
	})
	require.NoError(t, err)
	require.True(t, qresp.Balance.IsZero())

	// create deployment with deposit with both fee&deposit grants
	// should not have any errors
	deploymentMsg := &dtypes.MsgCreateDeployment{
		ID:        dID,
		Version:   dVersion,
		Groups:    make([]dtypes.GroupSpec, 0, len(dGroups)),
		Deposit:   deposit,
		Depositor: info.GetAddress().String(),
	}

	for _, group := range dGroups {
		deploymentMsg.Groups = append(deploymentMsg.Groups, *group)
	}

	resp, err = dcl.Tx().Broadcast(ctx, []sdk.Msg{deploymentMsg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)
}
