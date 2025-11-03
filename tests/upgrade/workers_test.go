//go:build e2e.upgrade

package upgrade

import (
	"context"
	"fmt"
	"os"
	"testing"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CosmWasm/wasmd/x/wasm/ioutils"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	cflags "pkg.akt.dev/go/cli/flags"
	arpcclient "pkg.akt.dev/go/node/client"
	client "pkg.akt.dev/go/node/client/discovery"
	cltypes "pkg.akt.dev/go/node/client/types"
	clt "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
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

	// verify deployment object migrations
	pu.verifyDeploymentMigrations(ctx, t, mcl)
}

// allowedDenoms are the only denoms that should appear in deployment objects post-upgrade.
// uact: migrated from axlUSDC (immediate) or newly created deployments.
// uakt: deferred migration (pending oracle availability).
var allowedDenoms = map[string]bool{
	sdkutil.DenomUact: true,
	sdkutil.DenomUakt: true,
}

func assertDenomMigrated(t *testing.T, denom, context string) {
	t.Helper()
	assert.Truef(t, allowedDenoms[denom], "unexpected denom %q in %s", denom, context)
}

func (pu *postUpgrade) verifyDeploymentMigrations(ctx context.Context, t *testing.T, mcl clt.Client) {
	t.Helper()

	var (
		uactCount int
		uaktCount int
	)

	// 1. Query active deployments and verify group/escrow denoms
	depResp, err := mcl.Query().Deployment().Deployments(ctx, &dvbeta.QueryDeploymentsRequest{
		Filters:    dvbeta.DeploymentFilters{State: dv1.DeploymentActive.String()},
		Pagination: &sdkquery.PageRequest{Limit: 1000},
	})
	require.NoError(t, err)

	t.Logf("found %d active deployments", len(depResp.Deployments))

	for _, dep := range depResp.Deployments {
		did := dep.Deployment.ID
		denomSeen := ""

		for _, group := range dep.Groups {
			for _, res := range group.GroupSpec.Resources {
				if !res.Price.IsZero() {
					assertDenomMigrated(t, res.Price.Denom, fmt.Sprintf("deployment %s group %d resource price", did, group.ID.GSeq))
					denomSeen = res.Price.Denom
				}
			}
		}

		// Check escrow account
		eacc := dep.EscrowAccount
		for _, f := range eacc.State.Funds {
			assertDenomMigrated(t, f.Denom, fmt.Sprintf("deployment %s escrow funds", did))
		}
		for _, d := range eacc.State.Deposits {
			assertDenomMigrated(t, d.Balance.Denom, fmt.Sprintf("deployment %s escrow deposit", did))
		}

		switch denomSeen {
		case sdkutil.DenomUact:
			uactCount++
		case sdkutil.DenomUakt:
			uaktCount++
		}
	}

	// 2. Query active orders
	ordersResp, err := mcl.Query().Market().Orders(ctx, &mvbeta.QueryOrdersRequest{
		Filters:    mvbeta.OrderFilters{State: mvbeta.OrderActive.String()},
		Pagination: &sdkquery.PageRequest{Limit: 1000},
	})
	require.NoError(t, err)

	for _, order := range ordersResp.Orders {
		for _, res := range order.Spec.Resources {
			if !res.Price.IsZero() {
				assertDenomMigrated(t, res.Price.Denom, fmt.Sprintf("order %s resource price", order.ID))
			}
		}
	}

	// 3. Query active bids
	bidsResp, err := mcl.Query().Market().Bids(ctx, &mvbeta.QueryBidsRequest{
		Filters:    mvbeta.BidFilters{State: mvbeta.BidActive.String()},
		Pagination: &sdkquery.PageRequest{Limit: 1000},
	})
	require.NoError(t, err)

	for _, bidResp := range bidsResp.Bids {
		if !bidResp.Bid.Price.IsZero() {
			assertDenomMigrated(t, bidResp.Bid.Price.Denom, fmt.Sprintf("bid %s price", bidResp.Bid.ID))
		}
	}

	// 4. Query active leases
	leasesResp, err := mcl.Query().Market().Leases(ctx, &mvbeta.QueryLeasesRequest{
		Filters:    mv1.LeaseFilters{State: mv1.LeaseActive.String()},
		Pagination: &sdkquery.PageRequest{Limit: 1000},
	})
	require.NoError(t, err)

	for _, leaseResp := range leasesResp.Leases {
		if !leaseResp.Lease.Price.IsZero() {
			assertDenomMigrated(t, leaseResp.Lease.Price.Denom, fmt.Sprintf("lease %s price", leaseResp.Lease.ID))
		}
	}

	// 5. Query open escrow payments
	paymentsResp, err := mcl.Query().Escrow().Payments(ctx, &ev1.QueryPaymentsRequest{
		State:      "open",
		Pagination: &sdkquery.PageRequest{Limit: 1000},
	})
	require.NoError(t, err)

	for _, pmnt := range paymentsResp.Payments {
		if !pmnt.State.Rate.IsZero() {
			assertDenomMigrated(t, pmnt.State.Rate.Denom, fmt.Sprintf("payment %s rate", pmnt.ID))
		}
		if !pmnt.State.Balance.IsZero() {
			assertDenomMigrated(t, pmnt.State.Balance.Denom, fmt.Sprintf("payment %s balance", pmnt.ID))
		}
		if !pmnt.State.Withdrawn.IsZero() {
			assertDenomMigrated(t, pmnt.State.Withdrawn.Denom, fmt.Sprintf("payment %s withdrawn", pmnt.ID))
		}
	}

	// 6. Verify uakt deployments still exist (deferred, not prematurely migrated)
	t.Logf("deployment migration summary: %d uact (migrated), %d uakt (deferred)", uactCount, uaktCount)
	assert.Greater(t, uactCount+uaktCount, 0, "expected at least one active deployment")
}
