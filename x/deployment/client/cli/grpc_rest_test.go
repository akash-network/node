package cli_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/go/cli"

	"pkg.akt.dev/akashd/testutil"
	"pkg.akt.dev/akashd/testutil/network"
	atypes "pkg.akt.dev/akashd/types"
	ccli "pkg.akt.dev/akashd/x/cert/client/cli"
	dcli "pkg.akt.dev/akashd/x/deployment/client/cli"
)

type GRPCRestTestSuite struct {
	suite.Suite

	cfg        network.Config
	network    *network.Network
	deployment v1beta4.QueryDeploymentResponse
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
		fmt.Sprintf("--%s=true", cli.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", cli.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", cli.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", cli.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// create deployment
	_, err = dcli.TxCreateDeploymentExec(
		val.ClientCtx,
		val.Address,
		deploymentPath,
		fmt.Sprintf("--%s=true", cli.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", cli.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", cli.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", cli.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", dcli.DefaultDeposit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// get deployment
	resp, err := dcli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &v1beta4.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Cert Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.ID.Owner)

	s.deployment = deployments[0]
}

func (s *GRPCRestTestSuite) TestGetDeployments() {
	val := s.network.Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp v1beta4.QueryDeploymentResponse
		expLen  int
	}{
		{
			"get deployments without filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list", val.APIAddress, v1beta4.GatewayVersion),
			false,
			deployment,
			1,
		},
		{
			"get deployments with filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.owner=%s", val.APIAddress,
				v1beta4.GatewayVersion,
				deployment.Deployment.ID.Owner),
			false,
			deployment,
			1,
		},
		{
			"get deployments with wrong state filter",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s", val.APIAddress, v1beta4.GatewayVersion,
				v1.DeploymentStateInvalid.String()),
			true,
			v1beta4.QueryDeploymentResponse{},
			0,
		},
		{
			"get deployments with two filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s&filters.dseq=%d",
				val.APIAddress, v1beta4.GatewayVersion, deployment.Deployment.State.String(), deployment.Deployment.ID.DSeq),
			false,
			deployment,
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var deployments v1beta4.QueryDeploymentsResponse
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
		expResp v1beta4.QueryDeploymentResponse
	}{
		{
			"get deployment with empty input",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info", val.APIAddress, atypes.ProtoAPIVersion),
			true,
			v1beta4.QueryDeploymentResponse{},
		},
		{
			"get deployment with invalid input",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s", val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.ID.Owner),
			true,
			v1beta4.QueryDeploymentResponse{},
		},
		{
			"deployment not found",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.ID.Owner,
				249),
			true,
			v1beta4.QueryDeploymentResponse{},
		},
		{
			"valid get deployment request",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s&id.dseq=%d",
				val.APIAddress,
				atypes.ProtoAPIVersion,
				deployment.Deployment.ID.Owner,
				deployment.Deployment.ID.DSeq),
			false,
			deployment,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out v1beta4.QueryDeploymentResponse
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
		expResp v1beta4.Group
	}{
		{
			"get group with empty input",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info", val.APIAddress, atypes.ProtoAPIVersion),
			true,
			v1beta4.Group{},
		},
		{
			"get group with invalid input",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s", val.APIAddress,
				atypes.ProtoAPIVersion,
				group.ID.Owner),
			true,
			v1beta4.Group{},
		},
		{
			"group not found",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				atypes.ProtoAPIVersion,
				group.ID.Owner,
				249),
			true,
			v1beta4.Group{},
		},
		{
			"valid get group request",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s&id.dseq=%d&id.gseq=%d",
				val.APIAddress,
				atypes.ProtoAPIVersion,
				group.ID.Owner,
				group.ID.DSeq,
				group.ID.GSeq),
			false,
			group,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out v1beta4.QueryGroupResponse
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
