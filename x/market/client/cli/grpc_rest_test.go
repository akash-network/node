package cli_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdktestutilcli "github.com/cosmos/cosmos-sdk/testutil/cli"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/akashd/testutil"
	"pkg.akt.dev/akashd/testutil/network"
	ccli "pkg.akt.dev/akashd/x/cert/client/cli"
	dcli "pkg.akt.dev/akashd/x/deployment/client/cli"
	mcli "pkg.akt.dev/akashd/x/market/client/cli"
	pcli "pkg.akt.dev/akashd/x/provider/client/cli"
)

type GRPCRestTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network
	order   v1beta5.Order
	bid     v1beta5.Bid
	lease   v1.Lease
}

func (s *GRPCRestTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	keyBar, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	keyAddr, err := keyBar.GetAddress()
	s.Require().NoError(err)

	_, err = s.network.WaitForHeight(1)
	s.Require().NoError(err)

	val := s.network.Validators[0]

	// Generate client certificate
	_, err = ccli.TxGenerateClientExec(
		context.Background(),
		val.ClientCtx,
		val.Address,
	)
	s.Require().NoError(err)

	// Publish client certificate
	_, err = ccli.TxPublishClientExec(
		context.Background(),
		val.ClientCtx,
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	deploymentPath, err := filepath.Abs("./../../../deployment/testdata/deployment.yaml")
	s.Require().NoError(err)

	providerPath, err := filepath.Abs("./../../../provider/testdata/provider.yaml")
	s.Require().NoError(err)

	// create deployment
	_, err = dcli.TxCreateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", dcli.DefaultDeposit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query orders
	resp, err := mcli.QueryOrdersExec(val.ClientCtx.WithOutputFormat("json"))
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
	sendTokens := dcli.DefaultDeposit.Add(dcli.DefaultDeposit)
	_, err = sdktestutilcli.MsgSendExec(
		val.ClientCtx,
		val.Address,
		keyAddr,
		sdk.NewCoins(sendTokens),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// create provider
	_, err = pcli.TxCreateProviderExec(
		val.ClientCtx,
		keyAddr,
		providerPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	_, err = mcli.TxCreateBidExec(
		val.ClientCtx,
		s.order.ID,
		sdk.NewDecCoinFromDec(testutil.CoinDenom, sdk.MustNewDecFromStr("1.1")),
		keyAddr,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", dcli.DefaultDeposit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// get bid
	resp, err = mcli.QueryBidsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	bidRes := &v1beta5.QueryBidsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	bids := bidRes.Bids
	s.Require().Equal(keyAddr.String(), bids[0].Bid.ID.Provider)

	s.bid = bids[0].Bid

	// create lease
	_, err = mcli.TxCreateLeaseExec(
		val.ClientCtx,
		s.bid.ID,
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query leases
	resp, err = mcli.QueryLeasesExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &v1beta5.QueryLeasesResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	leases := leaseRes.Leases
	s.Require().Equal(keyAddr.String(), leases[0].Lease.ID.Provider)

	s.order.State = v1.OrderActive
	s.bid.State = v1.BidActive

	// test query lease
	s.lease = leases[0].Lease
}

func (s *GRPCRestTestSuite) TestGetOrders() {
	val := s.network.Validators[0]
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
				v1.OrderStateInvalid.String()),
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

func (s *GRPCRestTestSuite) TestGetOrder() {
	val := s.network.Validators[0]
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

func (s *GRPCRestTestSuite) TestGetBids() {
	val := s.network.Validators[0]
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
				v1.BidStateInvalid.String()),
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

func (s *GRPCRestTestSuite) TestGetBid() {
	val := s.network.Validators[0]
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

func (s *GRPCRestTestSuite) TestGetLeases() {
	val := s.network.Validators[0]
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

func (s *GRPCRestTestSuite) TestGetLease() {
	val := s.network.Validators[0]
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

func (s *GRPCRestTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func TestGRPCRestTestSuite(t *testing.T) {
	suite.Run(t, new(GRPCRestTestSuite))
}
