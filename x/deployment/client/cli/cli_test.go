package cli_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/deployment/types"
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

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	_, err := s.network.WaitForHeight(1)
	s.Require().NoError(err)
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
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query deployments
	resp, err := cli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.DeploymentID.Owner)

	// test query deployment
	createdDep := deployments[0]
	resp, err = cli.QueryDeploymentExec(val.ClientCtx.WithOutputFormat("json"), createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	var deployment types.DeploymentResponse
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &deployment)
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
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
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

	var deploymentV2 types.DeploymentResponse
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &deploymentV2)
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
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
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
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
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
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &group)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupClosed, group.State)
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
