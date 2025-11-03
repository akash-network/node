//go:build e2e.integration

package e2e

//
//import (
//	"context"
//	"fmt"
//	"path/filepath"
//
//	"github.com/cosmos/cosmos-sdk/client"
//	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
//	"pkg.akt.dev/go/cli"
//	clitestutil "pkg.akt.dev/go/cli/testutil"
//	v1 "pkg.akt.dev/go/node/deployment/v1"
//	"pkg.akt.dev/go/node/deployment/v1beta4"
//
//	"pkg.akt.dev/node/v2/testutil"
//)
//
//type deploymentGRPCRestTestSuite struct {
//	*testutil.NetworkTestSuite
//
//	cctx       client.Context
//	deployment v1beta4.QueryDeploymentResponse
//}
//
//func (s *deploymentGRPCRestTestSuite) SetupSuite() {
//	s.NetworkTestSuite.SetupSuite()
//
//	val := s.Network().Validators[0]
//
//	s.cctx = val.ClientCtx
//
//	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
//	s.Require().NoError(err)
//
//	ctx := context.Background()
//
//	// Generate client certificate
//	_, err = clitestutil.TxGenerateClientExec(
//		ctx,
//		s.cctx,
//		cli.TestFlags().
//			WithFrom(val.Address.String())...,
//	)
//	s.Require().NoError(err)
//	s.Require().NoError(s.Network().WaitForNextBlock())
//
//	// Publish client certificate
//	_, err = clitestutil.TxPublishClientExec(
//		ctx,
//		s.cctx,
//		cli.TestFlags().
//			WithFrom(val.Address.String()).
//			WithSkipConfirm().
//			WithBroadcastModeBlock().
//			WithGasAutoFlags()...,
//	)
//	s.Require().NoError(err)
//	s.Require().NoError(s.Network().WaitForNextBlock())
//
//	// create deployment
//	_, err = clitestutil.TxCreateDeploymentExec(
//		ctx,
//		s.cctx,
//		deploymentPath,
//		cli.TestFlags().
//			WithFrom(val.Address.String()).
//			WithSkipConfirm().
//			WithBroadcastModeBlock().
//			WithDeposit(DefaultDeposit).
//			WithGasAutoFlags()...,
//	)
//	s.Require().NoError(err)
//	s.Require().NoError(s.Network().WaitForNextBlock())
//
//	// get deployment
//	resp, err := clitestutil.QueryDeploymentsExec(
//		ctx,
//		s.cctx,
//		cli.TestFlags().
//			WithOutputJSON()...,
//	)
//
//	s.Require().NoError(err)
//
//	out := &v1beta4.QueryDeploymentsResponse{}
//	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
//	s.Require().NoError(err)
//	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
//	deployments := out.Deployments
//	s.Require().Equal(val.Address.String(), deployments[0].Deployment.ID.Owner)
//
//	s.deployment = deployments[0]
//}
//
//func (s *deploymentGRPCRestTestSuite) TestGetDeployments() {
//	val := s.Network().Validators[0]
//	deployment := s.deployment
//
//	testCases := []struct {
//		name    string
//		url     string
//		expErr  bool
//		expResp v1beta4.QueryDeploymentResponse
//		expLen  int
//	}{
//		{
//			"get deployments without filters",
//			fmt.Sprintf("%s/akash/deployment/%s/deployments/list", val.APIAddress, v1beta4.GatewayVersion),
//			false,
//			deployment,
//			1,
//		},
//		{
//			"get deployments with filters",
//			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.owner=%s", val.APIAddress,
//				v1beta4.GatewayVersion,
//				deployment.Deployment.ID.Owner),
//			false,
//			deployment,
//			1,
//		},
//		{
//			"get deployments with wrong state filter",
//			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s", val.APIAddress, v1beta4.GatewayVersion,
//				v1.DeploymentStateInvalid.String()),
//			true,
//			v1beta4.QueryDeploymentResponse{},
//			0,
//		},
//		{
//			"get deployments with two filters",
//			fmt.Sprintf("%s/akash/deployment/%s/deployments/list?filters.state=%s&filters.dseq=%d",
//				val.APIAddress, v1beta4.GatewayVersion, deployment.Deployment.State.String(), deployment.Deployment.ID.DSeq),
//			false,
//			deployment,
//			1,
//		},
//	}
//
//	for _, tc := range testCases {
//		tc := tc
//		s.Run(tc.name, func() {
//			resp, err := sdktestutil.GetRequest(tc.url)
//			s.Require().NoError(err)
//
//			var deployments v1beta4.QueryDeploymentsResponse
//			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &deployments)
//
//			if tc.expErr {
//				s.Require().NotNil(err)
//				s.Require().Empty(deployments.Deployments)
//			} else {
//				s.Require().NoError(err)
//				s.Require().Len(deployments.Deployments, tc.expLen)
//				s.Require().Equal(tc.expResp, deployments.Deployments[0])
//			}
//		})
//	}
//}
//
//func (s *deploymentGRPCRestTestSuite) TestGetDeployment() {
//	val := s.Network().Validators[0]
//	deployment := s.deployment
//
//	testCases := []struct {
//		name    string
//		url     string
//		expErr  bool
//		expResp v1beta4.QueryDeploymentResponse
//	}{
//		{
//			"get deployment with empty input",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/deployments/info", val.APIAddress),
//			true,
//			v1beta4.QueryDeploymentResponse{},
//		},
//		{
//			"get deployment with invalid input",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/deployments/info?id.owner=%s", val.APIAddress,
//				deployment.Deployment.ID.Owner),
//			true,
//			v1beta4.QueryDeploymentResponse{},
//		},
//		{
//			"deployment not found",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/deployments/info?id.owner=%s&id.dseq=%d", val.APIAddress,
//				deployment.Deployment.ID.Owner,
//				249),
//			true,
//			v1beta4.QueryDeploymentResponse{},
//		},
//		{
//			"valid get deployment request",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/deployments/info?id.owner=%s&id.dseq=%d",
//				val.APIAddress,
//				deployment.Deployment.ID.Owner,
//				deployment.Deployment.ID.DSeq),
//			false,
//			deployment,
//		},
//	}
//
//	for _, tc := range testCases {
//		tc := tc
//		s.Run(tc.name, func() {
//			resp, err := sdktestutil.GetRequest(tc.url)
//			s.Require().NoError(err)
//
//			var out v1beta4.QueryDeploymentResponse
//			err = s.cctx.Codec.UnmarshalJSON(resp, &out)
//
//			if tc.expErr {
//				s.Require().Error(err)
//			} else {
//				s.Require().NoError(err)
//				s.Require().Equal(tc.expResp, out)
//			}
//		})
//	}
//}
//
//func (s *deploymentGRPCRestTestSuite) TestGetGroup() {
//	val := s.Network().Validators[0]
//	deployment := s.deployment
//	s.Require().NotEqual(0, len(deployment.Groups))
//	group := deployment.Groups[0]
//
//	testCases := []struct {
//		name    string
//		url     string
//		expErr  bool
//		expResp v1beta4.Group
//	}{
//		{
//			"get group with empty input",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/groups/info", val.APIAddress),
//			true,
//			v1beta4.Group{},
//		},
//		{
//			"get group with invalid input",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/groups/info?id.owner=%s", val.APIAddress,
//				group.ID.Owner),
//			true,
//			v1beta4.Group{},
//		},
//		{
//			"group not found",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/groups/info?id.owner=%s&id.dseq=%d", val.APIAddress,
//				group.ID.Owner,
//				249),
//			true,
//			v1beta4.Group{},
//		},
//		{
//			"valid get group request",
//			fmt.Sprintf("%s/akash/deployment/v1beta4/groups/info?id.owner=%s&id.dseq=%d&id.gseq=%d",
//				val.APIAddress,
//				group.ID.Owner,
//				group.ID.DSeq,
//				group.ID.GSeq),
//			false,
//			group,
//		},
//	}
//
//	for _, tc := range testCases {
//		tc := tc
//		s.Run(tc.name, func() {
//			resp, err := sdktestutil.GetRequest(tc.url)
//			s.Require().NoError(err)
//
//			var out v1beta4.QueryGroupResponse
//			err = s.cctx.Codec.UnmarshalJSON(resp, &out)
//
//			if tc.expErr {
//				s.Require().Error(err)
//			} else {
//				s.Require().NoError(err)
//				s.Require().Equal(tc.expResp, out.Group)
//			}
//		})
//	}
//}
