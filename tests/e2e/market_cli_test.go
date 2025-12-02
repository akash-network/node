//go:build e2e.integration

package e2e

import (
	"path/filepath"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	types "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	"pkg.akt.dev/node/testutil"
)

type marketIntegrationTestSuite struct {
	*testutil.NetworkTestSuite

	cctx         client.Context
	keyDeployer  *keyring.Record
	keyProvider  *keyring.Record
	addrDeployer sdk.AccAddress
	addrProvider sdk.AccAddress
}

func (s *marketIntegrationTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	ctx := s.CLIContext()

	kb := s.Network().Validators[0].ClientCtx.Keyring

	_, _, err := kb.NewMnemonic("keyDeployer", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	_, _, err = kb.NewMnemonic("keyProvider", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	s.cctx = s.CLIClientContext()
	cctx := s.cctx

	s.keyDeployer, err = s.cctx.Keyring.Key("keyDeployer")
	s.Require().NoError(err)

	s.keyProvider, err = s.cctx.Keyring.Key("keyProvider")
	s.Require().NoError(err)

	s.addrDeployer, err = s.keyDeployer.GetAddress()
	s.Require().NoError(err)

	s.addrProvider, err = s.keyProvider.GetAddress()
	s.Require().NoError(err)

	res, err := clitestutil.ExecSend(
		ctx,
		cctx,
		cli.TestFlags().
			With(
				s.Network().Validators[0].Address.String(),
				s.addrDeployer.String(),
				sdk.NewCoins(sdk.NewInt64Coin(s.Config().BondDenom, 10000000)).String()).
			WithFrom(s.Network().Validators[0].Address.String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), cctx, res.Bytes())

	res, err = clitestutil.ExecSend(
		ctx,
		cctx,
		cli.TestFlags().
			With(
				s.Network().Validators[0].Address.String(),
				s.addrProvider.String(),
				sdk.NewCoins(sdk.NewInt64Coin(s.Config().BondDenom, 10000000)).String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), cctx, res.Bytes())

	// Create client certificate
	_, err = clitestutil.TxGenerateClientExec(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String())...,
	)
	s.Require().NoError(err)

	_, err = clitestutil.TxPublishClientExec(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
}

// Naming as Test{number} just to run all tests sequentially
func (s *marketIntegrationTestSuite) Test1QueryOrders() {
	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
	s.Require().NoError(err)

	ctx := s.CLIContext()
	cctx := s.cctx

	// create deployment
	_, err = clitestutil.ExecDeploymentCreate(
		ctx,
		cctx,
		cli.TestFlags().
			With(deploymentPath).
			WithFrom(s.addrDeployer.String()).
			WithDeposit(DefaultDeposit).
			WithSkipConfirm().
			WithGasAuto().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments
	resp, err := clitestutil.ExecQueryDeployments(
		ctx,
		cctx,
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	out := &dtypes.QueryDeploymentsResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1)
	s.Require().Equal(s.addrDeployer.String(), out.Deployments[0].Deployment.ID.Owner)

	// test query orders
	resp, err = clitestutil.ExecQueryOrders(
		ctx,
		cctx,
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(s.addrDeployer.String(), orders[0].ID.Owner)

	// test query order
	createdOrder := orders[0]
	resp, err = clitestutil.ExecQueryOrder(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOrderID(createdOrder.ID)...,
	)
	s.Require().NoError(err)

	var order types.Order
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), &order)
	s.Require().NoError(err)
	s.Require().Equal(createdOrder, order)

	// test query orders with filters
	resp, err = clitestutil.ExecQueryOrders(
		ctx,
		cctx,
		cli.TestFlags().
			WithOwner(s.addrDeployer.String()).
			WithState("open").
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	result = &types.QueryOrdersResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	s.Require().Equal(createdOrder, result.Orders[0])

	// test query orders with wrong owner value
	_, err = clitestutil.ExecQueryOrders(
		ctx,
		cctx,
		cli.TestFlags().
			WithOwner("cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt").
			WithOutputJSON()...,
	)
	s.Require().Error(err)

	// test query orders with wrong state value
	_, err = clitestutil.ExecQueryOrders(
		ctx,
		cctx,
		cli.TestFlags().
			WithState("hello").
			WithOutputJSON()...,
	)
	s.Require().Error(err)
}

// Naming as Test{number} just to run all tests sequentially
func (s *marketIntegrationTestSuite) Test2CreateBid() {
	providerPath, err := filepath.Abs("../../x/provider/testdata/provider.yaml")
	s.Require().NoError(err)

	ctx := s.CLIContext()
	cctx := s.cctx

	addr := s.addrProvider

	// create provider
	_, err = clitestutil.ExecTxCreateProvider(
		ctx,
		cctx,
		cli.TestFlags().
			With(providerPath).
			WithFrom(addr.String()).
			WithSkipConfirm().
			WithGasAuto().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query providers
	resp, err := clitestutil.ExecQueryProviders(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	out := &ptypes.QueryProvidersResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed in TestCreateBid")
	s.Require().Equal(addr.String(), out.Providers[0].Owner)

	// fetch orders
	resp, err = clitestutil.ExecQueryOrders(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)

	createdOrder := result.Orders[0]

	// create bid
	_, err = clitestutil.ExecCreateBid(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(addr.String()).
			WithOrderID(createdOrder.ID).
			WithDeposit(sdk.NewCoin("uakt", sdkmath.NewInt(5000000))).
			WithPrice(sdk.NewDecCoinFromDec("uakt", sdkmath.LegacyMustNewDecFromStr("1.1"))).
			WithSkipConfirm().
			WithGasAuto().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query bids
	resp, err = clitestutil.ExecQueryBids(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	bidRes := &types.QueryBidsResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	bids := bidRes.Bids
	s.Require().Equal(addr.String(), bids[0].Bid.ID.Provider)

	// test query bid
	createdBid := bids[0].Bid
	resp, err = clitestutil.ExecQueryBid(
		ctx,
		cctx,
		cli.TestFlags().
			WithBidID(createdBid.ID).
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var bid types.QueryBidResponse
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), &bid)
	s.Require().NoError(err)
	s.Require().Equal(createdBid, bid.Bid)

	// test query bids with filters
	resp, err = clitestutil.ExecQueryBids(
		ctx,
		cctx,
		cli.TestFlags().
			WithProvider(addr.String()).
			WithState(bid.Bid.State.String()).
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	bidRes = &types.QueryBidsResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	s.Require().Equal(createdBid, bidRes.Bids[0].Bid)

	// test query bids with wrong owner value
	_, err = clitestutil.ExecQueryBids(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner("akash102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")...,
	)
	s.Require().Error(err)

	// test query bids with wrong state value
	_, err = clitestutil.ExecQueryBids(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithState("hello")...,
	)
	s.Require().Error(err)

	// create lease
	_, err = clitestutil.ExecCreateLease(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithBidID(bid.Bid.ID).
			WithSkipConfirm().
			WithGasAuto().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
}

// Naming as Test{number} just to run all tests sequentially
func (s *marketIntegrationTestSuite) Test3QueryLeasesAndCloseBid() {
	ctx := s.CLIContext()
	cctx := s.cctx

	// test query leases
	resp, err := clitestutil.ExecQueryLeases(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	leaseRes := &types.QueryLeasesResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	leases := leaseRes.Leases
	s.Require().Equal(s.addrProvider.String(), leases[0].Lease.ID.Provider)

	// test query lease
	createdLease := leases[0].Lease
	resp, err = clitestutil.ExecQueryLease(
		ctx,
		cctx,
		cli.TestFlags().
			WithLeaseID(createdLease.ID).
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var lease types.QueryLeaseResponse
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), &lease)
	s.Require().NoError(err)
	s.Require().Equal(createdLease, lease.Lease)

	// close bid
	_, err = clitestutil.ExecCloseBid(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(s.addrProvider.String()).
			WithBidID(lease.Lease.ID.BidID()).
			WithSkipConfirm().
			WithGasAuto().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query closed bids
	resp, err = clitestutil.ExecQueryBids(
		ctx,
		cctx,
		cli.TestFlags().
			WithState("closed").
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	bidRes := &types.QueryBidsResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	s.Require().Equal(s.addrProvider.String(), bidRes.Bids[0].Bid.ID.Provider)

	// test query leases with state value filter
	resp, err = clitestutil.ExecQueryLeases(
		ctx,
		cctx,
		cli.TestFlags().
			WithState("closed").
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	leaseRes = &types.QueryLeasesResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)

	// test query leases with wrong owner value
	_, err = clitestutil.ExecQueryLeases(
		ctx,
		cctx,
		cli.TestFlags().
			WithOwner("akash102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt").
			WithOutputJSON()...,
	)
	s.Require().Error(err)

	// test query leases with wrong state value
	_, err = clitestutil.ExecQueryLeases(
		ctx,
		cctx,
		cli.TestFlags().
			WithState("hello").
			WithOutputJSON()...,
	)
	s.Require().Error(err)
}

// Naming as Test{number} just to run all tests sequentially
func (s *marketIntegrationTestSuite) Test4CloseOrder() {
	ctx := s.CLIContext()
	cctx := s.cctx

	// fetch open orders
	resp, err := clitestutil.ExecQueryOrders(
		ctx,
		cctx,
		cli.TestFlags().
			WithState("open").
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	openedOrders := result.Orders
	s.Require().Len(openedOrders, 0)
}
