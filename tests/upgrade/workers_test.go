//go:build e2e.upgrade

package upgrade

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"pkg.akt.dev/go/cli/flags"
	arpcclient "pkg.akt.dev/go/node/client"
	aclient "pkg.akt.dev/go/node/client/discovery"
	cltypes "pkg.akt.dev/go/node/client/types"
	"pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"
	restypes "pkg.akt.dev/go/node/types/resources/v1beta4"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/app"
	uttypes "pkg.akt.dev/node/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v1.2.0", &postUpgrade{})
}

type postUpgrade struct {
	cl v1beta3.Client
}

var _ uttypes.TestWorker = (*postUpgrade)(nil)

func (pu *postUpgrade) Run(ctx context.Context, t *testing.T, params uttypes.TestParams) {
	encodingConfig := sdkutil.MakeEncodingConfig()
	app.ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	rpcClient, err := arpcclient.NewClient(ctx, params.Node)
	require.NoError(t, err)

	cctx := sdkclient.Context{}.
		WithCodec(encodingConfig.Codec).
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

	pu.cl, err = aclient.DiscoverClient(ctx, cctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, pu.cl)

	pu.testMarket(ctx, t)
	pu.testDeployment(ctx, t)
	pu.testLeaseClosedReason(ctx, t, params, kr, cctx, opts)
}

func (pu *postUpgrade) testMarket(ctx context.Context, t *testing.T) {
	// Query orders — verify migrated data is accessible
	ordersResp, err := pu.cl.Query().Market().Orders(ctx, &mtypes.QueryOrdersRequest{})
	require.NoError(t, err)
	require.NotNil(t, ordersResp)
	require.NotEmpty(t, ordersResp.Orders, "expected orders from network")

	for _, order := range ordersResp.Orders {
		require.NotEqual(t, int32(0), int32(order.State), "order state must not be invalid")
	}

	// Query bids — verify migrated data is accessible
	bidsResp, err := pu.cl.Query().Market().Bids(ctx, &mtypes.QueryBidsRequest{})
	require.NoError(t, err)
	require.NotNil(t, bidsResp)
	require.NotEmpty(t, bidsResp.Bids, "expected bids from network")

	for _, bidResp := range bidsResp.Bids {
		require.NotEqual(t, int32(0), int32(bidResp.Bid.State), "bid state must not be invalid")
	}

	// Query all leases — verify migrated data is accessible
	leasesResp, err := pu.cl.Query().Market().Leases(ctx, &mtypes.QueryLeasesRequest{})
	require.NoError(t, err)
	require.NotNil(t, leasesResp)
	require.NotEmpty(t, leasesResp.Leases, "expected leases from network")

	// Query leases with state filter — confirms state index works on new collections.IndexedMap
	activeLeasesResp, err := pu.cl.Query().Market().Leases(ctx, &mtypes.QueryLeasesRequest{
		Filters: mv1.LeaseFilters{State: "active"},
	})
	require.NoError(t, err)
	require.NotNil(t, activeLeasesResp)
	for _, lr := range activeLeasesResp.Leases {
		require.Equal(t, mv1.LeaseActive, lr.Lease.State, "filtered active lease has wrong state")
	}

	closedLeasesResp, err := pu.cl.Query().Market().Leases(ctx, &mtypes.QueryLeasesRequest{
		Filters: mv1.LeaseFilters{State: "closed"},
	})
	require.NoError(t, err)
	require.NotNil(t, closedLeasesResp)
	for _, lr := range closedLeasesResp.Leases {
		require.Equal(t, mv1.LeaseClosed, lr.Lease.State, "filtered closed lease has wrong state")
	}

	// Query leases with pagination — verify pagination works on migrated data
	pagedLeasesResp, err := pu.cl.Query().Market().Leases(ctx, &mtypes.QueryLeasesRequest{
		Pagination: &sdkquery.PageRequest{
			Limit: 1,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, pagedLeasesResp)
	require.NotNil(t, pagedLeasesResp.Pagination)
	require.Len(t, pagedLeasesResp.Leases, 1, "expected exactly 1 lease with Limit=1")
}

func (pu *postUpgrade) testDeployment(ctx context.Context, t *testing.T) {
	// Verify deployment params are set correctly
	paramsResp, err := pu.cl.Query().Deployment().Params(ctx, &dtypes.QueryParamsRequest{})
	require.NoError(t, err)
	require.NotNil(t, paramsResp)
	require.Contains(t, paramsResp.Params.MinDeposits, sdk.NewCoin("uakt", sdkmath.NewInt(500000)),
		"MinDeposits must contain 500000uakt")

	// Query all deployments — verify migrated data is accessible
	deploymentsResp, err := pu.cl.Query().Deployment().Deployments(ctx, &dtypes.QueryDeploymentsRequest{})
	require.NoError(t, err)
	require.NotNil(t, deploymentsResp)
	require.NotEmpty(t, deploymentsResp.Deployments, "expected deployments from network")

	// Query deployments with state filter — confirms state index works on new collections.IndexedMap
	activeResp, err := pu.cl.Query().Deployment().Deployments(ctx, &dtypes.QueryDeploymentsRequest{
		Filters: dtypes.DeploymentFilters{State: "active"},
	})
	require.NoError(t, err)
	require.NotNil(t, activeResp)
	require.NotEmpty(t, activeResp.Deployments, "expected active deployments from network")
	for _, dr := range activeResp.Deployments {
		require.Equal(t, dv1.DeploymentActive, dr.Deployment.State, "filtered active deployment has wrong state")
	}

	// Query deployments with pagination
	pagedResp, err := pu.cl.Query().Deployment().Deployments(ctx, &dtypes.QueryDeploymentsRequest{
		Pagination: &sdkquery.PageRequest{
			Limit: 1,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, pagedResp)
	require.NotNil(t, pagedResp.Pagination)
	require.Len(t, pagedResp.Deployments, 1, "expected exactly 1 deployment with Limit=1")
}

func (pu *postUpgrade) testLeaseClosedReason(
	ctx context.Context,
	t *testing.T,
	params uttypes.TestParams,
	kr keyring.Keyring,
	cctx sdkclient.Context,
	opts []cltypes.ClientOption,
) {

	// Step 1: Create provider account and fund it
	kinfo, _, err := kr.NewMnemonic("provider", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	providerAddr, err := kinfo.GetAddress()
	require.NoError(t, err)

	fundMsg := banktypes.NewMsgSend(
		params.FromAddress,
		providerAddr,
		sdk.NewCoins(sdk.NewCoin("uakt", sdkmath.NewInt(10000000))),
	)

	res, err := pu.cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{fundMsg})
	require.NoError(t, err)
	require.NotNil(t, res)
	txResp := res.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "fund provider failed: %s", txResp.RawLog)

	// Step 2: Create provider client
	providerCctx := cctx.
		WithFromAddress(providerAddr).
		WithFromName("provider").
		WithFrom("provider")

	providerCl, err := aclient.DiscoverClient(ctx, providerCctx, opts...)
	require.NoError(t, err)
	require.NotNil(t, providerCl)

	// Step 3: Register provider on chain
	registerMsg := &ptypes.MsgCreateProvider{
		Owner:   providerAddr.String(),
		HostURI: "https://test-provider.example.com",
	}

	res, err = providerCl.Tx().BroadcastMsgs(ctx, []sdk.Msg{registerMsg})
	require.NoError(t, err)
	require.NotNil(t, res)
	txResp = res.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "register provider failed: %s", txResp.RawLog)

	// Step 4: Create deployment (as owner)
	status, err := pu.cl.Query().ClientContext().Client.Status(ctx)
	require.NoError(t, err)
	dseq := uint64(status.SyncInfo.LatestBlockHeight)

	hash := sha256.Sum256([]byte("test-deployment"))
	groupSpec := dtypes.GroupSpec{
		Name: "test-group",
		Resources: dtypes.ResourceUnits{
			{
				Resources: restypes.Resources{
					ID: 1,
					CPU: &restypes.CPU{
						Units: restypes.NewResourceValue(10),
					},
					GPU: &restypes.GPU{
						Units: restypes.NewResourceValue(0),
					},
					Memory: &restypes.Memory{
						Quantity: restypes.NewResourceValue(1073741824),
					},
					Storage: restypes.Volumes{
						{Quantity: restypes.NewResourceValue(1073741824)},
					},
				},
				Count: 1,
				Price: sdk.NewDecCoin("uakt", sdkmath.NewInt(100)),
			},
		},
	}

	deployMsg := &dtypes.MsgCreateDeployment{
		ID: dv1.DeploymentID{
			Owner: params.FromAddress.String(),
			DSeq:  dseq,
		},
		Groups: dtypes.GroupSpecs{groupSpec},
		Hash:   hash[:],
		Deposit: depositv1.Deposit{
			Amount:  sdk.NewCoin("uakt", sdkmath.NewInt(5000000)),
			Sources: depositv1.Sources{depositv1.SourceBalance},
		},
	}

	res, err = pu.cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{deployMsg})
	require.NoError(t, err)
	require.NotNil(t, res)
	txResp = res.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "create deployment failed: %s", txResp.RawLog)

	// Step 5: Create bid (as provider)
	orderID := mv1.OrderID{
		Owner: params.FromAddress.String(),
		DSeq:  dseq,
		GSeq:  1,
		OSeq:  1,
	}
	bidID := mv1.MakeBidID(orderID, providerAddr)

	bidMsg := &mtypes.MsgCreateBid{
		ID:    bidID,
		Price: sdk.NewDecCoin("uakt", sdkmath.NewInt(1)),
		Deposit: depositv1.Deposit{
			Amount:  mtypes.DefaultBidMinDeposit,
			Sources: depositv1.Sources{depositv1.SourceBalance},
		},
	}

	res, err = providerCl.Tx().BroadcastMsgs(ctx, []sdk.Msg{bidMsg})
	require.NoError(t, err)
	require.NotNil(t, res)
	txResp = res.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "create bid failed: %s", txResp.RawLog)

	// Step 6: Create lease (as owner — accepts bid)
	leaseMsg := &mtypes.MsgCreateLease{
		BidID: bidID,
	}

	res, err = pu.cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{leaseMsg})
	require.NoError(t, err)
	require.NotNil(t, res)
	txResp = res.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "create lease failed: %s", txResp.RawLog)

	// Step 7: Close bid with reason Unstable (as provider)
	closeBidMsg := mtypes.NewMsgCloseBid(bidID, mv1.LeaseClosedReasonUnstable)

	res, err = providerCl.Tx().BroadcastMsgs(ctx, []sdk.Msg{closeBidMsg})
	require.NoError(t, err)
	require.NotNil(t, res)
	txResp = res.(*sdk.TxResponse)
	require.Equal(t, uint32(0), txResp.Code, "close bid failed: %s", txResp.RawLog)

	// Step 8: Verify closed lease has correct reason
	leaseID := mv1.MakeLeaseID(bidID)
	leaseResp, err := pu.cl.Query().Market().Lease(ctx, &mtypes.QueryLeaseRequest{ID: leaseID})
	require.NoError(t, err)
	require.NotNil(t, leaseResp)
	require.Equal(t, mv1.LeaseClosed, leaseResp.Lease.State, "lease must be closed")
	require.Equal(t, mv1.LeaseClosedReasonUnstable, leaseResp.Lease.Reason, "lease close reason must be Unstable")
	t.Logf("verified lease %s closed with reason: %s", leaseID, leaseResp.Lease.Reason)
}
