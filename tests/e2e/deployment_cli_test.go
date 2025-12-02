//go:build e2e.integration

package e2e

import (
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta4 "pkg.akt.dev/go/node/deployment/v1beta4"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	"pkg.akt.dev/node/testutil"
)

type deploymentIntegrationTestSuite struct {
	*testutil.NetworkTestSuite

	cctx         client.Context
	keyFunder    *keyring.Record
	addrFunder   sdk.AccAddress
	keyDeployer  *keyring.Record
	addrDeployer sdk.AccAddress
}

func (s *deploymentIntegrationTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	kb := s.Network().Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyFunder", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	_, _, err = kb.NewMnemonic("keyDeployer", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	val := s.Network().Validators[0]

	// Use the properly configured client context from NetworkTestSuite
	s.cctx = s.CLIClientContext()

	// Initialize funder keys with coins
	s.keyFunder, err = s.cctx.Keyring.Key("keyFunder")
	s.Require().NoError(err)

	s.keyDeployer, err = s.cctx.Keyring.Key("keyDeployer")
	s.Require().NoError(err)

	s.addrFunder, err = s.keyFunder.GetAddress()
	s.Require().NoError(err)

	s.addrDeployer, err = s.keyDeployer.GetAddress()
	s.Require().NoError(err)

	ctx := s.CLIContext()

	// Send enough tokens to cover DefaultDeposit plus gas fees
	sendAmount := DefaultDeposit.Amount.MulRaw(10)

	res, err := clitestutil.ExecSend(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(
				val.Address.String(),
				s.addrFunder.String(),
				sdk.NewCoins(sdk.NewCoin(s.Config().BondDenom, sendAmount)).String()).
			WithFrom(val.Address.String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	res, err = clitestutil.ExecSend(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(
				val.Address.String(),
				s.addrDeployer.String(),
				sdk.NewCoins(sdk.NewCoin(s.Config().BondDenom, sendAmount)).String()).
			WithFrom(val.Address.String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	// Create client certificate
	_, err = clitestutil.TxGenerateClientExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String())...,
	)
	s.Require().NoError(err)

	_, err = clitestutil.TxPublishClientExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
}

func (s *deploymentIntegrationTestSuite) TestDeployment() {
	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
	s.Require().NoError(err)

	ctx := s.CLIContext()

	// create deployment
	_, err = clitestutil.ExecDeploymentCreate(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(deploymentPath).
			WithFrom(s.addrDeployer.String()).
			WithDeposit(DefaultDeposit).
			WithSkipConfirm().
			WithGasAuto().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments (filter by owner to isolate from other tests)
	resp, err := clitestutil.ExecQueryDeployments(ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(s.addrDeployer.String())...,
	)
	s.Require().NoError(err)

	out := &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(out.Deployments), 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(s.addrDeployer.String(), deployments[0].Deployment.ID.Owner)

	// test query deployment
	createdDep := deployments[0]
	resp, err = clitestutil.ExecQueryDeployment(
		ctx,
		s.cctx,
		cli.TestFlags().WithOutputJSON().
			WithOwner(createdDep.Deployment.ID.Owner).
			WithDSeq(createdDep.Deployment.ID.DSeq)...,
	)
	s.Require().NoError(err)

	var deployment types.QueryDeploymentResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &deployment)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deployment)

	// test query deployments with filters
	resp, err = clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(s.addrDeployer.String()).
			WithDSeq(createdDep.Deployment.ID.DSeq)...,
	)
	s.Require().NoError(err, "Error when fetching deployments with owner filter")

	out = &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1)

	// test query deployments with wrong owner value
	_, err = clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner("akash102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")...,
	)
	s.Require().Error(err)

	// test query deployments with wrong state value
	_, err = clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithState("hello")...,
	)
	s.Require().Error(err)

	// test close deployment
	_, err = clitestutil.ExecDeploymentClose(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDSeq(createdDep.Deployment.ID.DSeq).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAuto()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments with state filter closed
	resp, err = clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithState("closed")...,
	)
	s.Require().NoError(err)

	out = &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Close Failed")
}

func (s *deploymentIntegrationTestSuite) TestGroup() {
	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
	s.Require().NoError(err)

	ctx := s.CLIContext()

	// create deployment
	_, err = clitestutil.ExecDeploymentCreate(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(deploymentPath).
			WithFrom(s.addrDeployer.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithDeposit(DefaultDeposit).
			WithGasAuto()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments
	resp, err := clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(s.addrDeployer.String()).
			WithState("active")...,
	)
	s.Require().NoError(err)

	out := &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(out.Deployments), 1, "Deployment Create Failed")
	// Use the latest deployment (highest dseq)
	deployments := out.Deployments
	s.Require().Equal(s.addrDeployer.String(), deployments[len(deployments)-1].Deployment.ID.Owner)

	createdDep := deployments[len(deployments)-1]

	s.Require().NotEqual(0, len(createdDep.Groups))

	// test close group tx
	_, err = ExecGroupClose(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithGroupID(createdDep.Groups[0].ID).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAuto()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	grp := createdDep.Groups[0]

	resp, err = clitestutil.ExecQueryGroup(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(grp.ID.Owner).
			WithDSeq(grp.ID.DSeq).
			WithGSeq(grp.ID.GSeq)...,
	)
	s.Require().NoError(err)

	var group types.Group
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &group)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupClosed, group.State)
}

func (s *deploymentIntegrationTestSuite) TestMultipleGroups() {
	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment-multi-groups.yaml")
	s.Require().NoError(err)

	ctx := s.CLIContext()

	// create deployment with multiple groups
	_, err = clitestutil.ExecDeploymentCreate(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(deploymentPath).
			WithFrom(s.addrDeployer.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithDeposit(DefaultDeposit).
			WithGasAuto()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments (filter by owner to isolate from other tests)
	resp, err := clitestutil.ExecQueryDeployments(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(s.addrDeployer.String()).
			WithState("active")...,
	)
	s.Require().NoError(err)

	out := &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(out.Deployments), 1, "Deployment Create Failed")
	// Use the latest deployment (highest dseq)
	deployments := out.Deployments
	s.Require().Equal(s.addrDeployer.String(), deployments[len(deployments)-1].Deployment.ID.Owner)

	createdDep := deployments[len(deployments)-1]

	// verify we have multiple groups (east and west)
	s.Require().Equal(2, len(createdDep.Groups), "Expected 2 groups in the deployment")

	// verify all groups are initially open
	for i, grp := range createdDep.Groups {
		s.Require().Equal(types.GroupOpen, grp.State, "Group %d should be open", i)
	}

	// close the first group
	_, err = ExecGroupClose(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithGroupID(createdDep.Groups[0].ID).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAuto()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// verify first group is closed
	resp, err = clitestutil.ExecQueryGroup(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(createdDep.Groups[0].ID.Owner).
			WithDSeq(createdDep.Groups[0].ID.DSeq).
			WithGSeq(createdDep.Groups[0].ID.GSeq)...,
	)
	s.Require().NoError(err)

	var group1 types.Group
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &group1)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupClosed, group1.State, "First group should be closed")

	// verify second group is still open
	resp, err = clitestutil.ExecQueryGroup(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(createdDep.Groups[1].ID.Owner).
			WithDSeq(createdDep.Groups[1].ID.DSeq).
			WithGSeq(createdDep.Groups[1].ID.GSeq)...,
	)
	s.Require().NoError(err)

	var group2 types.Group
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &group2)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupOpen, group2.State, "Second group should still be open")

	// close the second group
	_, err = ExecGroupClose(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithGroupID(createdDep.Groups[1].ID).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAuto()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// verify second group is now closed
	resp, err = clitestutil.ExecQueryGroup(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(createdDep.Groups[1].ID.Owner).
			WithDSeq(createdDep.Groups[1].ID.DSeq).
			WithGSeq(createdDep.Groups[1].ID.GSeq)...,
	)
	s.Require().NoError(err)

	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &group2)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupClosed, group2.State, "Second group should now be closed")

	// verify deployment is still active even with all groups closed
	resp, err = clitestutil.ExecQueryDeployment(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(createdDep.Deployment.ID.Owner).
			WithDSeq(createdDep.Deployment.ID.DSeq)...,
	)
	s.Require().NoError(err)

	var deployment types.QueryDeploymentResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &deployment)
	s.Require().NoError(err)
	s.Require().Equal(dv1.DeploymentActive, deployment.Deployment.State, "Deployment should still be active")
}
