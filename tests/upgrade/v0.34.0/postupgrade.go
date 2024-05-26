//go:build e2e.upgrade

// Package v0_34_0
// nolint revive
package v0_34_0

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/go-bip39"

	cltypes "github.com/akash-network/akash-api/go/node/client/types"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"pkg.akt.dev/akashd/app"
	"pkg.akt.dev/akashd/client"
	"pkg.akt.dev/akashd/cmd/common"
	"pkg.akt.dev/akashd/sdl"
	uttypes "pkg.akt.dev/akashd/tests/upgrade/types"
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
	uttypes.RegisterPostUpgradeWorker("v0.34.0", &postUpgrade{})
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

	deplAcc, err := kr.NewAccount("depl", mnemonic, "", hdPath, algo)
	require.NoError(t, err)

	hdPath = hd.CreateHDPath(coinType, 0, 1).String()
	authz1, err := kr.NewAccount("authz1", mnemonic, "", hdPath, algo)
	require.NoError(t, err)

	hdPath = hd.CreateHDPath(coinType, 0, 2).String()
	authz2, err := kr.NewAccount("authz2", mnemonic, "", hdPath, algo)
	require.NoError(t, err)

	mcctx := cctx.WithFromName(info.GetName()).
		WithFromAddress(info.GetAddress())

	dcctx := cctx.WithFromName(deplAcc.GetName()).
		WithFromAddress(deplAcc.GetAddress())

	a1cctx := cctx.WithFromName(authz1.GetName()).
		WithFromAddress(authz1.GetAddress())

	a2cctx := cctx.WithFromName(authz2.GetName()).
		WithFromAddress(authz2.GetAddress())

	opts := []cltypes.ClientOption{
		cltypes.WithGasPrices("0.025uakt"),
		cltypes.WithGas(flags.GasSetting{Simulate: false, Gas: 1000000}),
		cltypes.WithGasAdjustment(2),
	}

	mcl, err := client.DiscoverClient(ctx, mcctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	msgs := []sdk.Msg{
		&banktypes.MsgSend{
			FromAddress: info.GetAddress().String(),
			ToAddress:   deplAcc.GetAddress().String(),
			Amount:      sdk.NewCoins(sdk.NewCoin("uakt", sdk.NewInt(100000000))),
		},
		&banktypes.MsgSend{
			FromAddress: info.GetAddress().String(),
			ToAddress:   authz1.GetAddress().String(),
			Amount:      sdk.NewCoins(sdk.NewCoin("uakt", sdk.NewInt(100000000))),
		},
		&banktypes.MsgSend{
			FromAddress: info.GetAddress().String(),
			ToAddress:   authz2.GetAddress().String(),
			Amount:      sdk.NewCoins(sdk.NewCoin("uakt", sdk.NewInt(100000000))),
		},
	}

	// fund all new accounts with some tokens
	resp, err := mcl.Tx().Broadcast(ctx, msgs)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp := resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	dcl, err := client.DiscoverClient(ctx, dcctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	a1cl, err := client.DiscoverClient(ctx, a1cctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	a2cl, err := client.DiscoverClient(ctx, a2cctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, mcl)

	// give deployment key deployment deposit authorization from two accounts
	spendLimit := sdk.NewCoin("uakt", sdk.NewInt(10000000))
	authorization := dtypes.NewDepositDeploymentAuthorization(spendLimit)

	msg, err := authz.NewMsgGrant(a1cctx.FromAddress, deplAcc.GetAddress(), authorization, time.Now().AddDate(1, 0, 0))
	require.NoError(t, err)
	require.NotNil(t, msg)

	resp, err = a1cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	msg, err = authz.NewMsgGrant(a2cctx.FromAddress, deplAcc.GetAddress(), authorization, time.Now().AddDate(1, 0, 0))
	require.NoError(t, err)
	require.NotNil(t, msg)

	resp, err = a2cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	tsdl, err := sdl.Read([]byte(testSDL))
	require.NoError(t, err)
	require.NotNil(t, tsdl)

	syncInfo, err := dcl.Node().SyncInfo(ctx)
	require.NoError(t, err)
	require.NotNil(t, syncInfo)
	require.False(t, syncInfo.CatchingUp)

	dID := dtypes.DeploymentID{
		Owner: deplAcc.GetAddress().String(),
		DSeq:  uint64(syncInfo.LatestBlockHeight),
	}

	dVersion, err := tsdl.Version()
	require.NoError(t, err)

	dGroups, err := tsdl.DeploymentGroups()
	require.NoError(t, err)

	deposit, err := common.DetectDeposit(ctx, &pflag.FlagSet{}, dcl.Query(), "deployment", "MinDeposits")
	require.NoError(t, err)

	// create deployment with deposit from owner
	// should not have any errors
	deploymentMsg := &dtypes.MsgCreateDeployment{
		ID:        dID,
		Version:   dVersion,
		Groups:    make([]dtypes.GroupSpec, 0, len(dGroups)),
		Deposit:   deposit,
		Depositor: deplAcc.GetAddress().String(),
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

	// should be able to fund escrow with owner as depositor
	depositMsg := &dtypes.MsgDepositDeployment{
		ID:        dID,
		Amount:    deposit,
		Depositor: deplAcc.GetAddress().String(),
	}

	resp, err = dcl.Tx().Broadcast(ctx, []sdk.Msg{depositMsg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	// should be able to fund escrow with depositor from authz1
	depositMsg = &dtypes.MsgDepositDeployment{
		ID:        dID,
		Amount:    deposit,
		Depositor: authz1.GetAddress().String(),
	}

	resp, err = dcl.Tx().Broadcast(ctx, []sdk.Msg{depositMsg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	// should not be able to fund escrow with depositor from authz2
	depositMsg = &dtypes.MsgDepositDeployment{
		ID:        dID,
		Amount:    deposit,
		Depositor: authz2.GetAddress().String(),
	}

	resp, err = dcl.Tx().Broadcast(ctx, []sdk.Msg{depositMsg})
	require.Error(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.NotEqual(t, uint32(0), txResp.Code)

	dCloseMsg := &dtypes.MsgCloseDeployment{ID: dID}

	resp, err = dcl.Tx().Broadcast(ctx, []sdk.Msg{dCloseMsg})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &sdk.TxResponse{}, resp)
	txResp = resp.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code)

	grants, err := dcl.Query().Authz().Grants(ctx, &authz.QueryGrantsRequest{
		Granter:    authz1.GetAddress().String(),
		Grantee:    deplAcc.GetAddress().String(),
		MsgTypeUrl: dtypes.DepositDeploymentAuthorization{}.MsgTypeURL(),
	})
	require.NoError(t, err)
	require.Len(t, grants.Grants, 1)

	var auth authz.Authorization
	err = cctx.Codec.UnpackAny(grants.Grants[0].Authorization, &auth)
	require.NoError(t, err)

	grant, valid := auth.(*dtypes.DepositDeploymentAuthorization)
	require.True(t, valid)
	require.Equal(t, spendLimit, grant.SpendLimit)
}
