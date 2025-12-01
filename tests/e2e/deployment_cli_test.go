//go:build e2e.integration

package e2e

import (
	"context"
	"path/filepath"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta4 "pkg.akt.dev/go/node/deployment/v1beta4"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	"pkg.akt.dev/node/v2/testutil"
)

type deploymentIntegrationTestSuite struct {
	*testutil.NetworkTestSuite

	cctx           client.Context
	keyFunder      *keyring.Record
	addrFunder     sdk.AccAddress
	keyDeployer    *keyring.Record
	addrDeployer   sdk.AccAddress
	defaultDeposit sdk.Coin
}

func (s *deploymentIntegrationTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	kb := s.Network().Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyFunder", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	_, _, err = kb.NewMnemonic("keyDeployer", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)

	val := s.Network().Validators[0]

	s.cctx = val.ClientCtx

	// Initialize funder keys with coins
	s.keyFunder, err = s.cctx.Keyring.Key("keyFunder")
	s.Require().NoError(err)

	s.keyDeployer, err = s.cctx.Keyring.Key("keyDeployer")
	s.Require().NoError(err)

	s.addrFunder, err = s.keyFunder.GetAddress()
	s.Require().NoError(err)

	s.addrDeployer, err = s.keyDeployer.GetAddress()
	s.Require().NoError(err)

	s.defaultDeposit, err = dv1beta4.DefaultParams().MinDepositFor(s.Config().BondDenom)
	s.Require().NoError(err)

	ctx := context.Background()

	res, err := clitestutil.ExecSend(
		ctx,
		s.cctx,
		cli.TestFlags().
			With(
				val.Address.String(),
				s.addrFunder.String(),
				sdk.NewCoins(sdk.NewCoin(s.Config().BondDenom, s.defaultDeposit.Amount.MulRaw(4))).String()).
			WithGasAutoFlags().
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
				sdk.NewCoins(sdk.NewCoin(s.Config().BondDenom, s.defaultDeposit.Amount.MulRaw(4))).String()).
			WithGasAutoFlags().
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
			WithGasAutoFlags().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
}

func (s *deploymentIntegrationTestSuite) TestDeployment() {
	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment.yaml")
	s.Require().NoError(err)

	deploymentPath2, err := filepath.Abs("../../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	ctx := context.Background()

	// create deployment
	_, err = clitestutil.TxCreateDeploymentExec(
		ctx,
		s.cctx,
		deploymentPath,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDeposit(DefaultDeposit).
			WithSkipConfirm().
			WithGasAutoFlags().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments
	resp, err := clitestutil.QueryDeploymentsExec(ctx,
		s.cctx,
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	out := &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(s.addrDeployer.String(), deployments[0].Deployment.ID.Owner)

	// test query deployment
	createdDep := deployments[0]
	resp, err = clitestutil.QueryDeploymentExec(
		ctx,
		s.cctx,
		cli.TestFlags().WithOutputJSON().
			WithOwner(createdDep.Deployment.ID.Owner).
			WithDseq(createdDep.Deployment.ID.DSeq)...,
	)
	s.Require().NoError(err)

	var deployment types.QueryDeploymentResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &deployment)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deployment)

	// test query deployments with filters
	resp, err = clitestutil.QueryDeploymentsExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(s.addrDeployer.String()).
			WithDseq(createdDep.Deployment.ID.DSeq)...,
	)
	s.Require().NoError(err, "Error when fetching deployments with owner filter")

	out = &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1)

	// test updating deployment
	_, err = clitestutil.TxUpdateDeploymentExec(
		ctx,
		s.cctx,
		deploymentPath2,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(createdDep.Deployment.ID.DSeq).
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	resp, err = clitestutil.QueryDeploymentExec(
		ctx,
		s.cctx,
		cli.TestFlags().WithOutputJSON().
			WithOwner(createdDep.Deployment.ID.Owner).
			WithDseq(createdDep.Deployment.ID.DSeq)...,
	)
	s.Require().NoError(err)

	var deploymentV2 types.QueryDeploymentResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &deploymentV2)
	s.Require().NoError(err)
	s.Require().NotEqual(deployment.Deployment.Hash, deploymentV2.Deployment.Hash)

	// test query deployments with wrong owner value
	_, err = clitestutil.QueryDeploymentsExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner("akash102ruvpv2srmunfffxavttxnhezln6fnc3pf7tt")...,
	)
	s.Require().Error(err)

	// test query deployments with wrong state value
	_, err = clitestutil.QueryDeploymentsExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithState("hello")...,
	)
	s.Require().Error(err)

	// test close deployment
	_, err = clitestutil.TxCloseDeploymentExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(createdDep.Deployment.ID.DSeq).
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments with state filter closed
	resp, err = clitestutil.QueryDeploymentsExec(
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

	ctx := context.Background()

	// create deployment
	_, err = clitestutil.TxCreateDeploymentExec(
		ctx,
		s.cctx,
		deploymentPath,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithDeposit(DefaultDeposit).
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// test query deployments
	resp, err := clitestutil.QueryDeploymentsExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithState("active")...,
	)
	s.Require().NoError(err)

	out := &dv1beta4.QueryDeploymentsResponse{}
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Deployments, 1, "Deployment Create Failed")
	deployments := out.Deployments
	s.Require().Equal(s.addrDeployer.String(), deployments[0].Deployment.ID.Owner)

	createdDep := deployments[0]

	s.Require().NotEqual(0, len(createdDep.Groups))

	// test close group tx
	_, err = clitestutil.TxCloseGroupExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithGroupID(createdDep.Groups[0].ID).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForNextBlock())

	grp := createdDep.Groups[0]

	resp, err = clitestutil.QueryGroupExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithOutputJSON().
			WithOwner(grp.ID.Owner).
			WithDseq(grp.ID.DSeq).
			WithGseq(grp.ID.GSeq)...,
	)
	s.Require().NoError(err)

	var group types.Group
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &group)
	s.Require().NoError(err)
	s.Require().Equal(types.GroupClosed, group.State)
}

func (s *deploymentIntegrationTestSuite) TestFundedDeployment() {
	deploymentPath, err := filepath.Abs("../../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	deploymentID := dv1.DeploymentID{
		Owner: s.addrDeployer.String(),
		DSeq:  uint64(105),
	}

	prevFunderBal := s.getAccountBalance(s.addrFunder)

	ctx := context.Background()

	// Creating deployment paid by funder's account without any authorization from funder should fail
	_, err = clitestutil.TxCreateDeploymentExec(
		ctx,
		s.cctx,
		deploymentPath,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDepositor(s.addrFunder).
			WithDseq(deploymentID.DSeq).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().Error(err)

	// funder's balance shouldn't be deducted
	s.Require().Equal(prevFunderBal, s.getAccountBalance(s.addrFunder))

	// Grant the tenant authorization to use funds from the funder's account
	res, err := clitestutil.TxGrantAuthorizationExec(
		ctx,
		s.cctx,
		s.addrDeployer,
		cli.TestFlags().
			WithFrom(s.addrFunder.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())
	prevFunderBal = s.getAccountBalance(s.addrFunder)

	ownerBal := s.getAccountBalance(s.addrDeployer)

	// Creating deployment paid by funder's account should work now
	res, err = clitestutil.TxCreateDeploymentExec(
		ctx,
		s.cctx,
		deploymentPath,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(deploymentID.DSeq).
			WithDepositor(s.addrFunder).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)

	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	// funder's balance should be deducted correctly
	curFunderBal := s.getAccountBalance(s.addrFunder)
	s.Require().Equal(prevFunderBal.Sub(s.defaultDeposit.Amount), curFunderBal)
	prevFunderBal = curFunderBal

	fees := clitestutil.GetTxFees(ctx, s.T(), s.cctx, res.Bytes())

	// owner's balance should be deducted for fees correctly
	curOwnerBal := s.getAccountBalance(s.addrDeployer)
	s.Require().Equal(ownerBal.SubRaw(fees.GetFee().AmountOf("uakt").Int64()), curOwnerBal)

	ownerBal = curOwnerBal

	// depositing additional funds from the owner's account should work
	res, err = clitestutil.TxDepositDeploymentExec(
		ctx,
		s.cctx,
		s.defaultDeposit,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(deploymentID.DSeq).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	fees = clitestutil.GetTxFees(ctx, s.T(), s.cctx, res.Bytes())

	// owner's balance should be deducted correctly
	curOwnerBal = s.getAccountBalance(s.addrDeployer)
	s.Require().Equal(ownerBal.Sub(s.defaultDeposit.Amount).SubRaw(fees.GetFee().AmountOf("uakt").Int64()), curOwnerBal)
	ownerBal = curOwnerBal

	// depositing additional funds from the funder's account should work
	res, err = clitestutil.TxDepositDeploymentExec(
		ctx,
		s.cctx,
		s.defaultDeposit,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(deploymentID.DSeq).
			WithDepositor(s.addrFunder).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	// funder's balance should be deducted correctly
	curFunderBal = s.getAccountBalance(s.addrFunder)
	s.Require().Equal(prevFunderBal.Sub(s.defaultDeposit.Amount), curFunderBal)
	prevFunderBal = curFunderBal

	// revoke the authorization given to the deployment owner by the funder
	res, err = clitestutil.TxRevokeAuthorizationExec(
		ctx,
		s.cctx,
		s.addrDeployer,
		cli.TestFlags().
			WithFrom(s.addrFunder.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)

	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	prevFunderBal = s.getAccountBalance(s.addrFunder)

	// depositing additional funds from the funder's account should fail now
	_, err = clitestutil.TxDepositDeploymentExec(
		ctx,
		s.cctx,
		s.defaultDeposit,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(deploymentID.DSeq).
			WithDepositor(s.addrFunder).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().Error(err)

	// funder's balance shouldn't be deducted
	s.Require().Equal(prevFunderBal, s.getAccountBalance(s.addrFunder))
	ownerBal = s.getAccountBalance(s.addrDeployer)

	// closing the deployment should return the funds and balance in escrow to the funder and
	// owner's account
	res, err = clitestutil.TxCloseDeploymentExec(
		ctx,
		s.cctx,
		cli.TestFlags().
			WithFrom(s.addrDeployer.String()).
			WithDseq(deploymentID.DSeq).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(ctx, s.T(), s.cctx, res.Bytes())

	fees = clitestutil.GetTxFees(ctx, s.T(), s.cctx, res.Bytes())

	s.Require().Equal(prevFunderBal.Add(s.defaultDeposit.Amount.MulRaw(2)), s.getAccountBalance(s.addrFunder))
	s.Require().Equal(ownerBal.Add(s.defaultDeposit.Amount).SubRaw(fees.GetFee().AmountOf("uakt").Int64()), s.getAccountBalance(s.addrDeployer))
}

func (s *deploymentIntegrationTestSuite) getAccountBalance(address sdk.AccAddress) sdkmath.Int {
	cctxJSON := s.Network().Validators[0].ClientCtx.WithOutputFormat("json")
	res, err := sdktestutil.QueryBalancesExec(cctxJSON, address)
	s.Require().NoError(err)
	var balRes banktypes.QueryAllBalancesResponse
	err = cctxJSON.Codec.UnmarshalJSON(res.Bytes(), &balRes)
	s.Require().NoError(err)
	return balRes.Balances.AmountOf(s.Config().BondDenom)
}
