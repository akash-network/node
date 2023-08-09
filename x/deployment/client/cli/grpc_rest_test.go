package cli_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkrest "github.com/cosmos/cosmos-sdk/types/rest"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/network"
	atypes "github.com/akash-network/node/types"
	ccli "github.com/akash-network/node/x/cert/client/cli"
	"github.com/akash-network/node/x/deployment/client/cli"
)

type GRPCRestTestSuite struct {
	suite.Suite

	cfg        network.Config
	network    *network.Network
	deployment types.QueryDeploymentResponse
}

func (s *GRPCRestTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	_, err := s.network.WaitForHeight(1)
	s.Require().NoError(err)

	val := s.network.Validators[0]

	deploymentPath, err := filepath.Abs("../../testdata/deployment.yaml")
	s.Require().NoError(err)

	// Generate client certificate
	_, err = ccli.TxGenerateClientExec(
		context.Background(),
		val.ClientCtx,
		val.Address,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

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

	// get deployment
	resp, err := cli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Cert Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.DeploymentID.Owner)

	s.deployment = deployments[0]
}

func (s *GRPCRestTestSuite) TestGetDeployments() {
	val := s.network.Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.QueryDeploymentResponse
		expLen  int
	}{
		{
			"get deployments without filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list", val.APIAddress, atypes.ProtoAPIVersion),
			false,
			deployment,
			1,
		},
		{
			"get deployments with filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.owner=%s", val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.DeploymentID.Owner),
			false,
			deployment,
			1,
		},
		{
			"get deployments with wrong state filter",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s", val.APIAddress, atypes.ProtoAPIVersion,
				types.DeploymentStateInvalid.String()),
			true,
			types.QueryDeploymentResponse{},
			0,
		},
		{
			"get deployments with two filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s&filters.dseq=%d",
				val.APIAddress, atypes.ProtoAPIVersion, deployment.Deployment.State.String(), deployment.Deployment.DeploymentID.DSeq),
			false,
			deployment,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var deployments types.QueryDeploymentsResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &deployments)

			if tc.expErr {
				s.Require().NotNil(err)
				s.Require().Empty(deployments.Deployments)
			} else {
				s.Require().NoError(err)
				s.Require().Len(deployments.Deployments, tc.expLen)
				s.Require().Equal(tc.expResp, deployments.Deployments[0])
			}
		})
	}
}

func (s *GRPCRestTestSuite) TestGetDeployment() {
	val := s.network.Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.QueryDeploymentResponse
	}{
		{
			"get deployment with empty input",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info", val.APIAddress, atypes.ProtoAPIVersion),
			true,
			types.QueryDeploymentResponse{},
		},
		{
			"get deployment with invalid input",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s", val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.DeploymentID.Owner),
			true,
			types.QueryDeploymentResponse{},
		},
		{
			"deployment not found",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.DeploymentID.Owner,
				249),
			true,
			types.QueryDeploymentResponse{},
		},
		{
			"valid get deployment request",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s&id.dseq=%d",
				val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.DeploymentID.Owner,
				deployment.Deployment.DeploymentID.DSeq),
			false,
			deployment,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryDeploymentResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out)
			}
		})
	}
}

func (s *GRPCRestTestSuite) TestGetGroup() {
	val := s.network.Validators[0]
	deployment := s.deployment
	s.Require().NotEqual(0, len(deployment.Groups))
	group := deployment.Groups[0]

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.Group
	}{
		{
			"get group with empty input",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info", val.APIAddress, atypes.ProtoAPIVersion),
			true,
			types.Group{},
		},
		{
			"get group with invalid input",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s", val.APIAddress,
				atypes.ProtoAPIVersion,
				group.GroupID.Owner),
			true,
			types.Group{},
		},
		{
			"group not found",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				atypes.ProtoAPIVersion,
				group.GroupID.Owner,
				249),
			true,
			types.Group{},
		},
		{
			"valid get group request",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s&id.dseq=%d&id.gseq=%d",
				val.APIAddress,
				atypes.ProtoAPIVersion,
				group.GroupID.Owner,
				group.GroupID.DSeq,
				group.GroupID.GSeq),
			false,
			group,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryGroupResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Group)
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
