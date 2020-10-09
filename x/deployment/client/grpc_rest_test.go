package client_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkrest "github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/deployment/types"
)

type IntegrationTestSuite struct {
	suite.Suite

	cfg        network.Config
	network    *network.Network
	deployment types.DeploymentResponse
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	_, err := s.network.WaitForHeight(1)
	s.Require().NoError(err)

	val := s.network.Validators[0]

	deploymentPath, err := filepath.Abs("./../testdata/deployment.yaml")
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

	// get deployment
	resp, err := cli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.DeploymentID.Owner)

	s.deployment = deployments[0]
}

func (s *IntegrationTestSuite) TestGetDeployments() {
	val := s.network.Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.DeploymentResponse
		expLen  int
	}{
		{
			"get deployments without filters",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/list", val.APIAddress),
			false,
			deployment,
			1,
		},
		{
			"get deployments with filters",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/list?filters.owner=%s", val.APIAddress,
				deployment.Deployment.DeploymentID.Owner),
			false,
			deployment,
			1,
		},
		{
			"get deployments with wrong state filter",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/list?filters.state=%s", val.APIAddress,
				"invalid"),
			true,
			types.DeploymentResponse{},
			0,
		},
		{
			"get deployments with two filters",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/list?filters.state=%s&filters.dseq=%d",
				val.APIAddress, deployment.Deployment.State.String(), deployment.Deployment.DeploymentID.DSeq),
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
			err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp, &deployments)

			if tc.expErr {
				s.Require().Empty(deployments.Deployments)
			} else {
				s.Require().NoError(err)
				s.Require().Len(deployments.Deployments, tc.expLen)
				s.Require().Equal(tc.expResp, deployments.Deployments[0])
			}
		})
	}
}

func (s *IntegrationTestSuite) TestGetDeployment() {
	val := s.network.Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.DeploymentResponse
	}{
		{
			"get deployment with empty input",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/info", val.APIAddress),
			true,
			types.DeploymentResponse{},
		},
		{
			"get deployment with invalid input",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/info?id.owner=%s", val.APIAddress,
				deployment.Deployment.DeploymentID.Owner),
			true,
			types.DeploymentResponse{},
		},
		{
			"deployment not found",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				deployment.Deployment.DeploymentID.Owner, 249),
			true,
			types.DeploymentResponse{},
		},
		{
			"valid get deployment request",
			fmt.Sprintf("%s/akash/deployment/v1beta1/deployments/info?id.owner=%s&id.dseq=%d",
				val.APIAddress, deployment.Deployment.DeploymentID.Owner, deployment.Deployment.DeploymentID.DSeq),
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
			err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Deployment)
			}
		})
	}
}

func (s *IntegrationTestSuite) TestGetGroup() {
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
			fmt.Sprintf("%s/akash/deployment/v1beta1/groups/info", val.APIAddress),
			true,
			types.Group{},
		},
		{
			"get group with invalid input",
			fmt.Sprintf("%s/akash/deployment/v1beta1/groups/info?id.owner=%s", val.APIAddress,
				group.GroupID.Owner),
			true,
			types.Group{},
		},
		{
			"group not found",
			fmt.Sprintf("%s/akash/deployment/v1beta1/groups/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				group.GroupID.Owner, 249),
			true,
			types.Group{},
		},
		{
			"valid get group request",
			fmt.Sprintf("%s/akash/deployment/v1beta1/groups/info?id.owner=%s&id.dseq=%d&id.gseq=%d",
				val.APIAddress, group.GroupID.Owner, group.GroupID.DSeq, group.GroupID.GSeq),
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
			err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Group)
			}
		})
	}
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
