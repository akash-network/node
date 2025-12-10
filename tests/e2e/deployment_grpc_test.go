//go:build e2e.integration

package e2e

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/client"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"
	v1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta5"

	"pkg.akt.dev/node/v2/testutil"
)

type deploymentGRPCRestTestSuite struct {
	*testutil.NetworkTestSuite

	cctx       client.Context
	deployment dvbeta.QueryDeploymentResponse
}

func (s *deploymentGRPCRestTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	val := s.Network().Validators[0]

	s.cctx = val.ClientCtx

	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
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
	s.Require().NoError(s.Network().WaitForNextBlock())

	// Publish client certificate
	_, err = clitestutil.TxPublishClientExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(val.Address.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAuto()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// create deployment
	_, err = clitestutil.ExecDeploymentCreate(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(deploymentPath).
			WithFrom(val.Address.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithDeposit(DefaultDeposit).
			WithGasAuto()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// get deployment
	resp, err := clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)

	s.Require().NoError(err)

	out := &dvbeta.QueryDeploymentsResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(val.Address.String(), deployments[0].Deployment.ID.Owner)

	s.deployment = deployments[0]
}

func (s *deploymentGRPCRestTestSuite) TestGetDeployments() {
	val := s.Network().Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp dvbeta.QueryDeploymentResponse
		expLen  int
	}{
		{
			"get deployments without filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list", val.APIAddress, dvbeta.GatewayVersion),
			false,
			deployment,
			1,
		},
		{
			"get deployments with filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.owner=%s", val.APIAddress,
				dvbeta.GatewayVersion,
				deployment.Deployment.ID.Owner),
			false,
			deployment,
			1,
		},
		{
			"get deployments with wrong state filter",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s", val.APIAddress, dvbeta.GatewayVersion,
				v1.DeploymentStateInvalid.String()),
			true,
			dvbeta.QueryDeploymentResponse{},
			0,
		},
		{
			"get deployments with two filters",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s&filters.dseq=%d",
				val.APIAddress, dvbeta.GatewayVersion, deployment.Deployment.State.String(), deployment.Deployment.ID.DSeq),
			false,
			deployment,
			1,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var deployments dvbeta.QueryDeploymentsResponse
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

func (s *deploymentGRPCRestTestSuite) TestGetDeployment() {
	val := s.Network().Validators[0]
	deployment := s.deployment

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp dvbeta.QueryDeploymentResponse
	}{
		{
			"get deployment with empty input",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info", val.APIAddress, dvbeta.GatewayVersion),
			true,
			dvbeta.QueryDeploymentResponse{},
		},
		{
			"get deployment with invalid input",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s", val.APIAddress, dvbeta.GatewayVersion,
				deployment.Deployment.ID.Owner),
			true,
			dvbeta.QueryDeploymentResponse{},
		},
		{
			"deployment not found",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s&id.dseq=%d", val.APIAddress, dvbeta.GatewayVersion,
				deployment.Deployment.ID.Owner,
				249),
			true,
			dvbeta.QueryDeploymentResponse{},
		},
		{
			"valid get deployment request",
			fmt.Sprintf("%s/akash/deployment/%s/deployments/info?id.owner=%s&id.dseq=%d",
				val.APIAddress,
				dvbeta.GatewayVersion,
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

			var out dvbeta.QueryDeploymentResponse
			err = s.cctx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out)
			}
		})
	}
}

func (s *deploymentGRPCRestTestSuite) TestGetGroup() {
	val := s.Network().Validators[0]
	deployment := s.deployment
	s.Require().NotEqual(0, len(deployment.Groups))
	group := deployment.Groups[0]

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp dvbeta.Group
	}{
		{
			"get group with empty input",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info", val.APIAddress, dvbeta.GatewayVersion),
			true,
			dvbeta.Group{},
		},
		{
			"get group with invalid input",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s", val.APIAddress, dvbeta.GatewayVersion, group.ID.Owner),
			true,
			dvbeta.Group{},
		},
		{
			"group not found",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s&id.dseq=%d", val.APIAddress,
				dvbeta.GatewayVersion,
				group.ID.Owner,
				249),
			true,
			dvbeta.Group{},
		},
		{
			"valid get group request",
			fmt.Sprintf("%s/akash/deployment/%s/groups/info?id.owner=%s&id.dseq=%d&id.gseq=%d",
				val.APIAddress,
				dvbeta.GatewayVersion,
				group.ID.Owner,
				group.ID.DSeq,
				group.ID.GSeq),
			false,
			group,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out dvbeta.QueryGroupResponse
			err = s.cctx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Group)
			}
		})
	}
}
