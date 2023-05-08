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
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta3"

	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	"github.com/akash-network/node/testutil"
	ccli "github.com/akash-network/node/x/cert/client/cli"
	dcli "github.com/akash-network/node/x/deployment/client/cli"
	"github.com/akash-network/node/x/market/client/cli"
	pcli "github.com/akash-network/node/x/provider/client/cli"
)

type IntegrationTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1
	cfg.EnableLogging = true

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
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
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

// Naming as Test{number} just to run all tests sequentially
func (s *IntegrationTestSuite) Test1QueryOrders() {
	val := s.network.Validators[0]

	deploymentPath, err := filepath.Abs("../../../deployment/testdata/deployment.yaml")
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

	// test query deployments
	resp, err := dcli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &dtypes.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1)
	s.Require().Equal(val.Address.String(), out.Deployments[0].Deployment.DeploymentID.Owner)

	// test query orders
	resp, err = cli.QueryOrdersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(val.Address.String(), orders[0].OrderID.Owner)

	// test query order
	createdOrder := orders[0]
	resp, err = cli.QueryOrderExec(val.ClientCtx.WithOutputFormat("json"), createdOrder.OrderID)
	s.Require().NoError(err)

	var order types.Order
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &order)
	s.Require().NoError(err)
	s.Require().Equal(createdOrder, order)

	// test query orders with filters
	resp, err = cli.QueryOrdersExec(
		val.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--owner=%s", val.Address.String()),
		"--state=open",
	)
	s.Require().NoError(err)

	result = &types.QueryOrdersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	s.Require().Equal(createdOrder, result.Orders[0])

	// test query orders with wrong owner value
	_, err = cli.QueryOrdersExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt",
	)
	s.Require().Error(err)

	// test query orders with wrong state value
	_, err = cli.QueryOrdersExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=hello",
	)
	s.Require().Error(err)
}

// Naming as Test{number} just to run all tests sequentially
func (s *IntegrationTestSuite) Test2CreateBid() {
	val := s.network.Validators[0]

	providerPath, err := filepath.Abs("../../../provider/testdata/provider.yaml")
	s.Require().NoError(err)

	keyBar, err := val.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)

	// Send coins from validator to keyBar
	sendTokens := sdk.NewCoin(s.cfg.BondDenom, cli.DefaultDeposit.Amount.MulRaw(2))
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

	resp, err := bankcli.QueryBalancesExec(val.ClientCtx.WithOutputFormat("json"), keyBar.GetAddress())
	s.Require().NoError(err)

	var balRes banktypes.QueryAllBalancesResponse
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &balRes)
	s.Require().NoError(err)
	s.Require().Equal(sendTokens.Amount, balRes.Balances.AmountOf(s.cfg.BondDenom))

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

	// test query providers
	resp, err = pcli.QueryProvidersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &ptypes.QueryProvidersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed in TestCreateBid")
	s.Require().Equal(keyBar.GetAddress().String(), out.Providers[0].Owner)

	// fetch orders
	resp, err = cli.QueryOrdersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)

	createdOrder := result.Orders[0]

	// create bid
	_, err = cli.TxCreateBidExec(
		val.ClientCtx,
		createdOrder.OrderID,
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

	// test query bids
	resp, err = cli.QueryBidsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	bidRes := &types.QueryBidsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	bids := bidRes.Bids
	s.Require().Equal(keyBar.GetAddress().String(), bids[0].Bid.BidID.Provider)

	// test query bid
	createdBid := bids[0].Bid
	resp, err = cli.QueryBidExec(val.ClientCtx.WithOutputFormat("json"), createdBid.BidID)
	s.Require().NoError(err)

	var bid types.QueryBidResponse
	s.T().Logf("query response: %v", resp.String())
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &bid)
	s.Require().NoError(err)
	s.Require().Equal(createdBid, bid.Bid)

	// test query bids with filters
	resp, err = cli.QueryBidsExec(
		val.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--provider=%s", keyBar.GetAddress().String()),
		fmt.Sprintf("--state=%s", bid.Bid.State.String()),
	)
	s.Require().NoError(err)

	bidRes = &types.QueryBidsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	s.Require().Equal(createdBid, bidRes.Bids[0].Bid)

	// test query bids with wrong owner value
	_, err = cli.QueryBidsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt",
	)
	s.Require().Error(err)

	// test query bids with wrong state value
	_, err = cli.QueryBidsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=hello",
	)
	s.Require().Error(err)

	// create lease
	_, err = cli.TxCreateLeaseExec(
		val.ClientCtx,
		bid.Bid.BidID,
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())
}

// Naming as Test{number} just to run all tests sequentially
func (s *IntegrationTestSuite) Test3QueryLeasesAndCloseBid() {
	val := s.network.Validators[0]

	keyBar, err := val.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)

	// test query leases
	resp, err := cli.QueryLeasesExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &types.QueryLeasesResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	leases := leaseRes.Leases
	s.Require().Equal(keyBar.GetAddress().String(), leases[0].Lease.LeaseID.Provider)

	// test query lease
	createdLease := leases[0].Lease
	resp, err = cli.QueryLeaseExec(val.ClientCtx.WithOutputFormat("json"), createdLease.LeaseID)
	s.Require().NoError(err)

	var lease types.QueryLeaseResponse
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &lease)
	s.Require().NoError(err)
	s.Require().Equal(createdLease, lease.Lease)

	// create bid
	_, err = cli.TxCloseBidExec(
		val.ClientCtx,
		lease.Lease.LeaseID.OrderID(),
		keyBar.GetAddress(),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=auto"),
		"--gas-adjustment=1.5",
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// test query closed bids
	resp, err = cli.QueryBidsExec(val.ClientCtx.WithOutputFormat("json"), "--state=closed")
	s.Require().NoError(err)

	bidRes := &types.QueryBidsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), bidRes)
	s.Require().NoError(err)
	s.Require().Len(bidRes.Bids, 1)
	s.Require().Equal(keyBar.GetAddress().String(), bidRes.Bids[0].Bid.BidID.Provider)

	// test query leases with state value filter
	resp, err = cli.QueryLeasesExec(val.ClientCtx.WithOutputFormat("json"), "--state=closed")
	s.Require().NoError(err)

	leaseRes = &types.QueryLeasesResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)

	// test query leases with wrong owner value
	_, err = cli.QueryLeasesExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--provider=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt",
	)
	s.Require().Error(err)

	// test query leases with wrong state value
	_, err = cli.QueryLeasesExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=hello",
	)
	s.Require().Error(err)
}

// Naming as Test{number} just to run all tests sequentially
func (s *IntegrationTestSuite) Test4CloseOrder() {
	val := s.network.Validators[0]

	// fetch open orders
	resp, err := cli.QueryOrdersExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=open",
	)
	s.Require().NoError(err)

	result := &types.QueryOrdersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	openedOrders := result.Orders
	s.Require().Len(openedOrders, 0)
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
