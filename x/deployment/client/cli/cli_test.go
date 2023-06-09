package cli_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	banktestutil "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	clitestutil "github.com/akash-network/node/testutil/cli"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"github.com/akash-network/node/testutil"
	ccli "github.com/akash-network/node/x/cert/client/cli"
	"github.com/akash-network/node/x/deployment/client/cli"
)

type IntegrationTestSuite struct {
	suite.Suite
	cfg            network.Config
	network        *network.Network
	keyFunder      keyring.Info
	defaultDeposit sdk.Coin
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1
	// cfg.EnableLogging = true

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyFoo", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	_, err = s.network.WaitForHeight(1)
	s.Require().NoError(err)

	val := s.network.Validators[0]

	// Initialize funder keys with coins
	s.keyFunder, err = val.ClientCtx.Keyring.Key("keyFoo")
	s.Require().NoError(err)

	s.defaultDeposit, err = types.DefaultParams().MinDepositFor("uakt")
	s.Require().NoError(err)

	res, err := banktestutil.MsgSendExec(
		val.ClientCtx,
		val.Address,
		s.keyFunder.GetAddress(),
		sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, s.defaultDeposit.Amount.MulRaw(4))),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())

	// Create client certificate
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

func (s *IntegrationTestSuite) TestDeployment() {
	val := s.network.Validators[0]

	deploymentPath, err := filepath.Abs("../../testdata/deployment.yaml")
	s.Require().NoError(err)

	deploymentPath2, err := filepath.Abs("../../testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	// create deployment
	_, err = cli.TxCreateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", cli.DefaultDeposit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query deployments
	resp, err := cli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.DeploymentID.Owner)

	// test query deployment
	createdDep := deployments[0]
	resp, err = cli.QueryDeploymentExec(val.ClientCtx.WithOutputFormat("json"), createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	var deployment types.QueryDeploymentResponse
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &deployment)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deployment)

	// test query deployments with filters
	resp, err = cli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--owner=%s", val.Address.String()),
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
	)
	s.Require().NoError(err, "Error when fetching deployments with owner filter")

	out = &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1)

	// test updating deployment
	_, err = cli.TxUpdateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath2,
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	resp, err = cli.QueryDeploymentExec(val.ClientCtx.WithOutputFormat("json"), createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	var deploymentV2 types.QueryDeploymentResponse
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &deploymentV2)
	s.Require().NoError(err)
	s.Require().NotEqual(deployment.Deployment.Version, deploymentV2.Deployment.Version)

	// test query deployments with wrong owner value
	_, err = cli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--owner=cosmos102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt",
	)
	s.Require().Error(err)

	// test query deployments with wrong state value
	_, err = cli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=hello",
	)
	s.Require().Error(err)

	// test close deployment
	_, err = cli.TxCloseDeploymentExec(
		val.ClientCtx,
		val.Address,
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query deployments with state filter closed
	resp, err = cli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=closed",
	)
	s.Require().NoError(err)

	out = &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Close Failed")
}

func (s *IntegrationTestSuite) TestGroup() {
	val := s.network.Validators[0]

	deploymentPath, err := filepath.Abs("../../testdata/deployment.yaml")
	s.Require().NoError(err)

	// create deployment
	_, err = cli.TxCreateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", cli.DefaultDeposit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// test query deployments
	resp, err := cli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=active",
	)
	s.Require().NoError(err)

	out := &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.DeploymentID.Owner)

	createdDep := deployments[0]

	s.Require().NotEqual(0, len(createdDep.Groups))

	// test close group tx
	_, err = cli.TxCloseGroupExec(
		val.ClientCtx,
		createdDep.Groups[0].GroupID,
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	resp, err = cli.QueryGroupExec(val.ClientCtx.WithOutputFormat("json"), createdDep.Groups[0].GroupID)
	s.Require().NoError(err)

	var group types.Group
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &group)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupClosed, group.State)
}

func (s *IntegrationTestSuite) TestFundedDeployment() {
	// setup
	val := s.network.Validators[0]
	deploymentPath, err := filepath.Abs("../../testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	deploymentID := types.DeploymentID{
		Owner: val.Address.String(),
		DSeq:  uint64(105),
	}

	prevFunderBal := s.getAccountBalance(s.keyFunder.GetAddress())

	// Creating deployment paid by funder's account without any authorization from funder should
	// fail
	res, err := cli.TxCreateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
		fmt.Sprintf("--depositor-account=%s", s.keyFunder.GetAddress().String()),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxUnSuccessful(s.T(), val.ClientCtx, res.Bytes())

	// funder's balance shouldn't be deducted
	s.Require().Equal(prevFunderBal, s.getAccountBalance(s.keyFunder.GetAddress()))

	// Grant the tenant authorization to use funds from the funder's account
	res, err = cli.TxGrantAuthorizationExec(
		val.ClientCtx,
		s.keyFunder.GetAddress(),
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())
	prevFunderBal = s.getAccountBalance(s.keyFunder.GetAddress())

	ownerBal := s.getAccountBalance(val.Address)

	// Creating deployment paid by funder's account should work now
	res, err = cli.TxCreateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
		fmt.Sprintf("--depositor-account=%s", s.keyFunder.GetAddress().String()),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())

	// funder's balance should be deducted correctly
	curFunderBal := s.getAccountBalance(s.keyFunder.GetAddress())
	s.Require().Equal(prevFunderBal.Sub(s.defaultDeposit.Amount), curFunderBal)
	prevFunderBal = curFunderBal

	// owner's balance should be deducted correctly
	curOwnerBal := s.getAccountBalance(val.Address)
	s.Require().Equal(ownerBal.SubRaw(20), curOwnerBal)
	ownerBal = curOwnerBal

	// depositing additional funds from the owner's account should work
	res, err = cli.TxDepositDeploymentExec(
		val.ClientCtx,
		s.defaultDeposit,
		val.Address,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())

	// owner's balance should be deducted correctly
	curOwnerBal = s.getAccountBalance(val.Address)
	s.Require().Equal(ownerBal.Sub(s.defaultDeposit.Amount).SubRaw(20), curOwnerBal)
	// s.Require().Equal(prevOwnerBal.Sub(types.DefaultDeploymentMinDeposit.Amount), curOwnerBal)
	ownerBal = curOwnerBal

	// depositing additional funds from the funder's account should work
	res, err = cli.TxDepositDeploymentExec(
		val.ClientCtx,
		s.defaultDeposit,
		val.Address,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
		fmt.Sprintf("--depositor-account=%s", s.keyFunder.GetAddress().String()),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())

	// funder's balance should be deducted correctly
	curFunderBal = s.getAccountBalance(s.keyFunder.GetAddress())
	s.Require().Equal(prevFunderBal.Sub(s.defaultDeposit.Amount), curFunderBal)
	prevFunderBal = curFunderBal

	// revoke the authorization given to the deployment owner by the funder
	res, err = cli.TxRevokeAuthorizationExec(
		val.ClientCtx,
		s.keyFunder.GetAddress(),
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())
	prevFunderBal = s.getAccountBalance(s.keyFunder.GetAddress())

	// depositing additional funds from the funder's account should fail now
	res, err = cli.TxDepositDeploymentExec(
		val.ClientCtx,
		s.defaultDeposit,
		val.Address,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
		fmt.Sprintf("--depositor-account=%s", s.keyFunder.GetAddress().String()),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxUnSuccessful(s.T(), val.ClientCtx, res.Bytes())

	// funder's balance shouldn't be deducted
	s.Require().Equal(prevFunderBal, s.getAccountBalance(s.keyFunder.GetAddress()))
	ownerBal = s.getAccountBalance(val.Address)

	// closing the deployment should return the funds and balance in escrow to the funder and
	// owner's account
	res, err = cli.TxCloseDeploymentExec(
		val.ClientCtx,
		val.Address,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), val.ClientCtx, res.Bytes())
	s.Require().Equal(prevFunderBal.Add(s.defaultDeposit.Amount.MulRaw(2)), s.getAccountBalance(s.keyFunder.GetAddress()))
	s.Require().Equal(ownerBal.Add(s.defaultDeposit.Amount).SubRaw(20), s.getAccountBalance(val.Address))
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) getAccountBalance(address sdk.AccAddress) sdk.Int {
	cctxJSON := s.network.Validators[0].ClientCtx.WithOutputFormat("json")
	res, err := banktestutil.QueryBalancesExec(cctxJSON, address)
	s.Require().NoError(err)
	var balRes banktypes.QueryAllBalancesResponse
	err = cctxJSON.Codec.UnmarshalJSON(res.Bytes(), &balRes)
	s.Require().NoError(err)
	return balRes.Balances.AmountOf(s.cfg.BondDenom)
}
