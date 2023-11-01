package cli_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkrest "github.com/cosmos/cosmos-sdk/types/rest"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/network"
	ccli "github.com/akash-network/node/x/cert/client/cli"
	dcli "github.com/akash-network/node/x/deployment/client/cli"
	"github.com/akash-network/node/x/market/client/cli"
	pcli "github.com/akash-network/node/x/provider/client/cli"
)

type GRPCRestTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network
	order   types.Order
	bid     types.Bid
	lease   types.Lease
}

func (s *GRPCRestTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	keyBar, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, "",
		hd.Secp256k1)
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
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
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
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", dcli.DefaultDeposit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query orders
	resp, err := cli.QueryOrdersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(val.Address.String(), orders[0].OrderID.Owner)

	// test query order
	s.order = orders[0]

	// Send coins from validator to keyBar
	sendTokens := cli.DefaultDeposit.Add(cli.DefaultDeposit)
	_, err = bankcli.MsgSendExec(
		val.ClientCtx,
		val.Address,
		keyBar.GetAddress(),
		sdk.NewCoins(sendTokens),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// create provider
	_, err = pcli.TxCreateProviderExec(
		val.ClientCtx,
		keyBar.GetAddress(),
		providerPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	_, err = cli.TxCreateBidExec(
		val.ClientCtx,
		s.order.OrderID,
		sdk.NewDecCoinFromDec(testutil.CoinDenom, sdk.MustNewDecFromStr("1.1")),
		keyBar.GetAddress(),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", cli.DefaultDeposit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// get bid
	resp, err = cli.QueryBidsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	bidRes := &types.QueryBidsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	bids := bidRes.Bids
	s.Require().Equal(keyBar.GetAddress().String(), bids[0].Bid.BidID.Provider)

	s.bid = bids[0].Bid

	// create lease
	_, err = cli.TxCreateLeaseExec(
		val.ClientCtx,
		s.bid.BidID,
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query leases
	resp, err = cli.QueryLeasesExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &types.QueryLeasesResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	leases := leaseRes.Leases
	s.Require().Equal(keyBar.GetAddress().String(), leases[0].Lease.LeaseID.Provider)

	s.order.State = types.OrderActive
	s.bid.State = types.BidActive

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
		expResp types.Order
		expLen  int
	}{
		{
			"get orders without filters",
			fmt.Sprintf("%s/akash/market/%s/orders/list", val.APIAddress, types.APIVersion),
			false,
			order,
			1,
		},
		{
			"get orders with filters",
			fmt.Sprintf("%s/akash/market/%s/orders/list?filters.owner=%s", val.APIAddress,
				types.APIVersion,
				order.OrderID.Owner),
			false,
			order,
			1,
		},
		{
			"get orders with wrong state filter",
			fmt.Sprintf("%s/akash/market/%s/orders/list?filters.state=%s", val.APIAddress,
				types.APIVersion,
				types.OrderStateInvalid.String()),
			true,
			types.Order{},
			0,
		},
		{
			"get orders with two filters",
			fmt.Sprintf("%s/akash/market/%s/orders/list?filters.state=%s&filters.oseq=%d",
				val.APIAddress, types.APIVersion, order.State.String(), order.OrderID.OSeq),
			false,
			order,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var orders types.QueryOrdersResponse
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
		expResp types.Order
	}{
		{
			"get order with empty input",
			fmt.Sprintf("%s/akash/market/%s/orders/info", val.APIAddress, types.APIVersion),
			true,
			types.Order{},
		},
		{
			"get order with invalid input",
			fmt.Sprintf("%s/akash/market/%s/orders/info?id.owner=%s", val.APIAddress,
				types.APIVersion,
				order.OrderID.Owner),
			true,
			types.Order{},
		},
		{
			"order not found",
			fmt.Sprintf("%s/akash/market/%s/orders/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d",
				val.APIAddress,
				types.APIVersion,
				order.OrderID.Owner, 249, 32, 235),
			true,
			types.Order{},
		},
		{
			"valid get order request",
			fmt.Sprintf("%s/akash/market/%s/orders/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d",
				val.APIAddress,
				types.APIVersion,
				order.OrderID.Owner,
				order.OrderID.DSeq,
				order.OrderID.GSeq,
				order.OrderID.OSeq),
			false,
			order,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryOrderResponse
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
		expResp types.Bid
		expLen  int
	}{
		{
			"get bids without filters",
			fmt.Sprintf("%s/akash/market/%s/bids/list", val.APIAddress, types.APIVersion),
			false,
			bid,
			1,
		},
		{
			"get bids with filters",
			fmt.Sprintf("%s/akash/market/%s/bids/list?filters.owner=%s",
				val.APIAddress,
				types.APIVersion,
				bid.BidID.Owner),
			false,
			bid,
			1,
		},
		{
			"get bids with wrong state filter",
			fmt.Sprintf("%s/akash/market/%s/bids/list?filters.state=%s",
				val.APIAddress,
				types.APIVersion,
				types.BidStateInvalid.String()),
			true,
			types.Bid{},
			0,
		},
		{
			"get bids with more filters",
			fmt.Sprintf("%s/akash/market/%s/bids/list?filters.state=%s&filters.oseq=%d&filters.provider=%s",
				val.APIAddress,
				types.APIVersion,
				bid.State.String(),
				bid.BidID.OSeq,
				bid.BidID.Provider),
			false,
			bid,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var bids types.QueryBidsResponse
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
		expResp types.Bid
	}{
		{
			"get bid with empty input",
			fmt.Sprintf("%s/akash/market/%s/bids/info", val.APIAddress, types.APIVersion),
			true,
			types.Bid{},
		},
		{
			"get bid with invalid input",
			fmt.Sprintf("%s/akash/market/%s/bids/info?id.owner=%s",
				val.APIAddress,
				types.APIVersion,
				bid.BidID.Owner),
			true,
			types.Bid{},
		},
		{
			"bid not found",
			fmt.Sprintf("%s/akash/market/%s/bids/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				types.APIVersion,
				bid.BidID.Provider,
				249,
				32,
				235,
				bid.BidID.Owner),
			true,
			types.Bid{},
		},
		{
			"valid get bid request",
			fmt.Sprintf("%s/akash/market/%s/bids/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				types.APIVersion,
				bid.BidID.Owner,
				bid.BidID.DSeq,
				bid.BidID.GSeq,
				bid.BidID.OSeq,
				bid.BidID.Provider),
			false,
			bid,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryBidResponse
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
		expResp types.Lease
		expLen  int
	}{
		{
			"get leases without filters",
			fmt.Sprintf("%s/akash/market/%s/leases/list", val.APIAddress, types.APIVersion),
			false,
			lease,
			1,
		},
		{
			"get leases with filters",
			fmt.Sprintf("%s/akash/market/%s/leases/list?filters.owner=%s",
				val.APIAddress,
				types.APIVersion,
				lease.LeaseID.Owner),
			false,
			lease,
			1,
		},
		{
			"get leases with wrong state filter",
			fmt.Sprintf("%s/akash/market/%s/leases/list?filters.state=%s",
				val.APIAddress,
				types.APIVersion,
				types.LeaseStateInvalid.String()),
			true,
			types.Lease{},
			0,
		},
		{
			"get leases with more filters",
			fmt.Sprintf("%s/akash/market/%s/leases/list?filters.state=%s&filters.oseq=%d&filters.provider=%s",
				val.APIAddress,
				types.APIVersion,
				lease.State.String(),
				lease.LeaseID.OSeq,
				lease.LeaseID.Provider),
			false,
			lease,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var leases types.QueryLeasesResponse
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
		expResp types.Lease
	}{
		{
			"get lease with empty input",
			fmt.Sprintf("%s/akash/market/%s/leases/info", val.APIAddress, types.APIVersion),
			true,
			types.Lease{},
		},
		{
			"get lease with invalid input",
			fmt.Sprintf("%s/akash/market/%s/leases/info?id.owner=%s",
				val.APIAddress,
				types.APIVersion,
				lease.LeaseID.Owner),
			true,
			types.Lease{},
		},
		{
			"lease not found",
			fmt.Sprintf("%s/akash/market/%s/leases/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				types.APIVersion,
				lease.LeaseID.Provider,
				249,
				32,
				235,
				lease.LeaseID.Owner),
			true,
			types.Lease{},
		},
		{
			"valid get lease request",
			fmt.Sprintf("%s/akash/market/%s/leases/info?id.owner=%s&id.dseq=%d&id.gseq=%d&id.oseq=%d&id.provider=%s",
				val.APIAddress,
				types.APIVersion,
				lease.LeaseID.Owner,
				lease.LeaseID.DSeq,
				lease.LeaseID.GSeq,
				lease.LeaseID.OSeq,
				lease.LeaseID.Provider),
			false,
			lease,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryLeaseResponse
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
