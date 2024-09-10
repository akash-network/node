//go:build e2e.integration

package e2e

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	"pkg.akt.dev/akashd/testutil"
)

type marketGRPCRestTestSuite struct {
	*testutil.NetworkTestSuite

	cctx  client.Context
	order v1beta5.Order
	bid   v1beta5.Bid
	lease v1.Lease
}

func (s *marketGRPCRestTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	val := s.Network().Validators[0]

	s.cctx = val.ClientCtx

	kb := s.cctx.Keyring
	keyBar, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	keyAddr, err := keyBar.GetAddress()
	s.Require().NoError(err)

	ctx := context.Background()

	// Generate client certificate
	_, err = clitestutil.TxGenerateClientExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(val.Address.String())...,
	)
	s.Require().NoError(err)

	// Publish client certificate
	_, err = clitestutil.TxPublishClientExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(val.Address.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForBlocks(2))

	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
	s.Require().NoError(err)

	providerPath, err := filepath.Abs("../../x/provider/testdata/provider.yaml")
	s.Require().NoError(err)

	// create deployment
	_, err = clitestutil.TxCreateDeploymentExec(
		ctx,
		s.cctx,
		deploymentPath,
		cli.TestFlags().
			WithFrom(val.Address.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithDeposit(DefaultDeposit).
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForBlocks(2))

	// test query orders
	resp, err := clitestutil.QueryOrdersExec(
		ctx, val.ClientCtx.WithOutputFormat("json"),
	)
	s.Require().NoError(err)

	result := &v1beta5.QueryOrdersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(val.Address.String(), orders[0].ID.Owner)

	// test query order
	s.order = orders[0]

	// Send coins from validator to keyBar
	sendTokens := DefaultDeposit.Add(DefaultDeposit)
	_, err = clitestutil.ExecSend(
		ctx,
		val.ClientCtx,
		cli.TestFlags().
			With(
				val.Address.String(),
				keyAddr.String(),
				sdk.NewCoins(sendTokens).String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	// create provider
	_, err = clitestutil.TxCreateProviderExec(
		ctx,
		s.cctx,
		providerPath,
		cli.TestFlags().
			WithFrom(keyAddr.String()).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	_, err = clitestutil.TxCreateBidExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(keyAddr.String()).
			WithOrderID(s.order.ID).
			WithPrice(sdk.NewDecCoinFromDec(testutil.CoinDenom, sdk.MustNewDecFromStr("1.1"))).
			WithDeposit(DefaultDeposit).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	// get bid
	resp, err = clitestutil.QueryBidsExec(
		ctx, val.ClientCtx.WithOutputFormat("json"),
	)
	s.Require().NoError(err)

	bidRes := &v1beta5.QueryBidsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	bids := bidRes.Bids
	s.Require().Equal(keyAddr.String(), bids[0].Bid.ID.Provider)

	s.bid = bids[0].Bid

	// create lease
	_, err = clitestutil.TxCreateLeaseExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(val.Address.String()).
			WithBidID(s.bid.ID).
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query leases
	resp, err = clitestutil.QueryLeasesExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	leaseRes := &v1beta5.QueryLeasesResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	leases := leaseRes.Leases
	s.Require().Equal(keyAddr.String(), leases[0].Lease.ID.Provider)

	s.order.State = v1beta5.OrderActive
	s.bid.State = v1beta5.BidActive

	// test query lease
	s.lease = leases[0].Lease
}

func (s *marketGRPCRestTestSuite) TestGetOrders() {
	val := s.Network().Validators[0]
	order := s.order

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1beta5.Order
		expLen  int
	}{
		{
			"get orders without filters",
			fmt.Sprintf("%s/akash/market/%s/orders/list", val.APIAddress, v1beta5.GatewayVersion),
			false,
			order,
			1,
		},
		{
			"get orders with filters",
			fmt.Sprintf("%s/akash/market/%s/orders/list?filters.owner=%s", val.APIAddress,
				v1beta5.GatewayVersion,
				order.ID.Owner),
			false,
			order,
			1,
		},
		{
			"get orders with wrong state filter",
			fmt.Sprintf("%s/akash/market/%s/orders/list?filters.state=%s", val.APIAddress,
				v1beta5.GatewayVersion,
				v1beta5.OrderStateInvalid.String()),
			true,
			v1beta5.Order{},
			0,
		},
		{
			"get orders with two filters",
			fmt.Sprintf("%s/akash/market/%s/orders/list?filters.state=%s&filters.oseq=%d",
				val.APIAddress, v1beta5.GatewayVersion, order.State.String(), order.ID.OSeq),
			false,
			order,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var orders v1beta5.QueryOrdersResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &orders)

			if tc.expErr {
				s.Require().NotNil(err)
				s.Require().Empty(orders.Orders)
			} else {
				s.Require().NoError(err)
				s.Require().Len(orders.Orders, tc.expLen)
				s.Require().Equal(tc.expResp, orders.Orders[0])
			}
		})
	}
}

func (s *marketGRPCRestTestSuite) TestGetOrder() {
	val := s.Network().Validators[0]
	order := s.order

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1beta5.Order
	}{
		{
			"get order with empty input",
			fmt.Sprintf("%s/akash/market/%s/orders/info", val.APIAddress, v1beta5.GatewayVersion),
			true,
			v1beta5.Order{},
		},
		{
			"get order with invalid input",
			fmt.Sprintf("%s/akash/market/%s/orders/info?id.owner=%s", val.APIAddress,
				v1beta5.GatewayVersion,
				order.ID.Owner),
			true,
			v1beta5.Order{},
		},
		{
			"order not found",
			fmt.Sprintf("%s/akash/market/%s/orders/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d",
				val.APIAddress,
				v1beta5.GatewayVersion,
				order.ID.Owner, 249, 32, 235),
			true,
			v1beta5.Order{},
		},
		{
			"valid get order request",
			fmt.Sprintf("%s/akash/market/%s/orders/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d",
				val.APIAddress,
				v1beta5.GatewayVersion,
				order.ID.Owner,
				order.ID.DSeq,
				order.ID.GSeq,
				order.ID.OSeq),
			false,
			order,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out v1beta5.QueryOrderResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Order)
			}
		})
	}
}

func (s *marketGRPCRestTestSuite) TestGetBids() {
	val := s.Network().Validators[0]
	bid := s.bid

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1beta5.Bid
		expLen  int
	}{
		{
			"get bids without filters",
			fmt.Sprintf("%s/akash/market/%s/bids/list", val.APIAddress, v1beta5.GatewayVersion),
			false,
			bid,
			1,
		},
		{
			"get bids with filters",
			fmt.Sprintf("%s/akash/market/%s/bids/list?filters.owner=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				bid.ID.Owner),
			false,
			bid,
			1,
		},
		{
			"get bids with wrong state filter",
			fmt.Sprintf("%s/akash/market/%s/bids/list?filters.state=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				v1beta5.BidStateInvalid.String()),
			true,
			v1beta5.Bid{},
			0,
		},
		{
			"get bids with more filters",
			fmt.Sprintf("%s/akash/market/%s/bids/list?filters.state=%s&filters.oseq=%d&filters.provider=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				bid.State.String(),
				bid.ID.OSeq,
				bid.ID.Provider),
			false,
			bid,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var bids v1beta5.QueryBidsResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &bids)

			if tc.expErr {
				s.Require().NotNil(err)
				s.Require().Empty(bids.Bids)
			} else {
				s.Require().NoError(err)
				s.Require().Len(bids.Bids, tc.expLen)
				s.Require().Equal(tc.expResp, bids.Bids[0].Bid)
			}
		})
	}
}

func (s *marketGRPCRestTestSuite) TestGetBid() {
	val := s.Network().Validators[0]
	bid := s.bid

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1beta5.Bid
	}{
		{
			"get bid with empty input",
			fmt.Sprintf("%s/akash/market/%s/bids/info", val.APIAddress, v1beta5.GatewayVersion),
			true,
			v1beta5.Bid{},
		},
		{
			"get bid with invalid input",
			fmt.Sprintf("%s/akash/market/%s/bids/info?id.owner=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				bid.ID.Owner),
			true,
			v1beta5.Bid{},
		},
		{
			"bid not found",
			fmt.Sprintf("%s/akash/market/%s/bids/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				bid.ID.Provider,
				249,
				32,
				235,
				bid.ID.Owner),
			true,
			v1beta5.Bid{},
		},
		{
			"valid get bid request",
			fmt.Sprintf("%s/akash/market/%s/bids/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				bid.ID.Owner,
				bid.ID.DSeq,
				bid.ID.GSeq,
				bid.ID.OSeq,
				bid.ID.Provider),
			false,
			bid,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out v1beta5.QueryBidResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Bid)
			}
		})
	}
}

func (s *marketGRPCRestTestSuite) TestGetLeases() {
	val := s.Network().Validators[0]
	lease := s.lease

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1.Lease
		expLen  int
	}{
		{
			"get leases without filters",
			fmt.Sprintf("%s/akash/market/%s/leases/list", val.APIAddress, v1beta5.GatewayVersion),
			false,
			lease,
			1,
		},
		{
			"get leases with filters",
			fmt.Sprintf("%s/akash/market/%s/leases/list?filters.owner=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				lease.ID.Owner),
			false,
			lease,
			1,
		},
		{
			"get leases with wrong state filter",
			fmt.Sprintf("%s/akash/market/%s/leases/list?filters.state=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				v1.LeaseStateInvalid.String()),
			true,
			v1.Lease{},
			0,
		},
		{
			"get leases with more filters",
			fmt.Sprintf("%s/akash/market/%s/leases/list?filters.state=%s&filters.oseq=%d&filters.provider=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				lease.State.String(),
				lease.ID.OSeq,
				lease.ID.Provider),
			false,
			lease,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var leases v1beta5.QueryLeasesResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &leases)

			if tc.expErr {
				s.Require().NotNil(err)
				s.Require().Empty(leases.Leases)
			} else {
				s.Require().NoError(err)
				s.Require().Len(leases.Leases, tc.expLen)
				s.Require().Equal(tc.expResp, leases.Leases[0].Lease)
			}
		})
	}
}

func (s *marketGRPCRestTestSuite) TestGetLease() {
	val := s.Network().Validators[0]
	lease := s.lease

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1.Lease
	}{
		{
			"get lease with empty input",
			fmt.Sprintf("%s/akash/market/%s/leases/info", val.APIAddress, v1beta5.GatewayVersion),
			true,
			v1.Lease{},
		},
		{
			"get lease with invalid input",
			fmt.Sprintf("%s/akash/market/%s/leases/info?id.owner=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				lease.ID.Owner),
			true,
			v1.Lease{},
		},
		{
			"lease not found",
			fmt.Sprintf("%s/akash/market/%s/leases/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				lease.ID.Provider,
				249,
				32,
				235,
				lease.ID.Owner),
			true,
			v1.Lease{},
		},
		{
			"valid get lease request",
			fmt.Sprintf("%s/akash/market/%s/leases/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				v1beta5.GatewayVersion,
				lease.ID.Owner,
				lease.ID.DSeq,
				lease.ID.GSeq,
				lease.ID.OSeq,
				lease.ID.Provider),
			false,
			lease,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out v1beta5.QueryLeaseResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Lease)
			}
		})
	}
}
