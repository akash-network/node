package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/x/deployment/types"
)

var (
	ownerPub  = ed25519.GenPrivKey().PubKey()
	ownerAddr = sdk.AccAddress(ownerPub.Address())
	addr2Pub  = ed25519.GenPrivKey().PubKey()
	addr2     = sdk.AccAddress(addr2Pub.Address())
)

type TestSuite struct {
	suite.Suite
	ctx sdk.Context
	app *app.App
}

func (s *TestSuite) SetupTest() {
	isCheckTx := false
	s.app = app.Setup(isCheckTx)
	s.ctx = s.app.BaseApp.NewContext(isCheckTx, abci.Header{})
}

func (s *TestSuite) TestKeeper() {
	s.T().Log("Adding balance to account")
	err := s.app.Keepers.Bank.SetCoins(s.ctx, ownerAddr, sdk.NewCoins(sdk.NewInt64Coin("stake", 10000)))
	s.Require().Nil(err)
	s.Require().True(s.app.Keepers.Bank.GetCoins(s.ctx, ownerAddr).IsEqual(sdk.NewCoins(sdk.NewInt64Coin("stake", 10000))))

	s.T().Log("verify deployment is created")
	sdl, readError := sdl.ReadFile("../testdata/deployment.yml")
	s.Require().NoError(readError, "Error in reading file")
	groupSpecs, err := sdl.DeploymentGroups()
	s.Require().NoError(err, "Error in getting groups from file")
	msg := types.MsgCreateDeployment{
		Owner:  ownerAddr,
		Groups: make([]types.GroupSpec, 0, len(groupSpecs)),
	}
	deploymentID := types.DeploymentID{
		Owner: msg.Owner,
		DSeq:  uint64(131),
	}
	deployment := types.Deployment{
		DeploymentID: deploymentID,
		State:        types.DeploymentActive,
	}
	for _, spec := range groupSpecs {
		msg.Groups = append(msg.Groups, *spec)
	}
	groups := make([]types.Group, 0, len(msg.Groups))
	for idx, spec := range msg.Groups {
		groups = append(groups, types.Group{
			GroupID:   types.MakeGroupID(deployment.ID(), uint32(idx+1)),
			State:     types.GroupOpen,
			GroupSpec: spec,
		})
	}
	s.app.Keepers.Deployment.Create(s.ctx, deployment, groups)
	_, ok := s.app.Keepers.Deployment.GetDeployment(s.ctx, deploymentID)
	s.Require().True(ok, "Deployment not created")

	s.T().Log("verify get deployment with wrong deploymentID")
	_, ok = s.app.Keepers.Deployment.GetDeployment(s.ctx, types.DeploymentID{
		Owner: addr2,
		DSeq:  uint64(135),
	})
	s.Require().False(ok, "Get deployment failed")

	if len(groups) > 0 {
		s.T().Log("verify get groups with deploymentID")
		depGroups := s.app.Keepers.Deployment.GetGroups(s.ctx, deploymentID)
		s.Require().Equal(groups, depGroups, "Get Groups failed")

		s.T().Log("verify get group with groupID")
		_, ok = s.app.Keepers.Deployment.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().True(ok, "Get Group failed")

		s.T().Log("verify on order created")
		s.app.Keepers.Deployment.OnOrderCreated(s.ctx, groups[0])
		details, _ := s.app.Keepers.Deployment.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupOrdered, details.State, "OnOrderCreated failed")

		s.T().Log("verify on lease created")
		s.app.Keepers.Deployment.OnLeaseCreated(s.ctx, groups[0].GroupID)
		details, _ = s.app.Keepers.Deployment.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupMatched, details.State, "OnLeaseCreated failed")

		s.T().Log("verify on lease insufficient funds")
		s.app.Keepers.Deployment.OnLeaseInsufficientFunds(s.ctx, groups[0].GroupID)
		details, _ = s.app.Keepers.Deployment.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupInsufficientFunds, details.State, "OnLeaseInsufficientFunds failed")

		s.T().Log("verify on lease closed")
		s.app.Keepers.Deployment.OnLeaseClosed(s.ctx, groups[0].GroupID)
		details, _ = s.app.Keepers.Deployment.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupOpen, details.State, "OnLeaseClosed failed")

		s.T().Log("verify on deployment closed")
		s.app.Keepers.Deployment.OnDeploymentClosed(s.ctx, groups[0])
		details, _ = s.app.Keepers.Deployment.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupClosed, details.State, "OnDeploymentClosed failed")
	}

	s.T().Log("verify get groups with wrong deploymentID")
	depGroups := s.app.Keepers.Deployment.GetGroups(s.ctx, types.DeploymentID{
		Owner: addr2,
		DSeq:  uint64(136),
	})
	s.Require().Equal([]types.Group(nil), depGroups, "Get Groups failed with wrong data")

	s.T().Log("verify get group with wrong groupID")
	_, ok = s.app.Keepers.Deployment.GetGroup(s.ctx, types.GroupID{
		Owner: addr2,
		DSeq:  135,
		GSeq:  12,
	})
	s.Require().False(ok, "Get Group failed with wrong data")

	s.T().Log("verify update deployment")
	deployment = types.Deployment{
		DeploymentID: deploymentID,
		State:        types.DeploymentClosed,
	}
	s.app.Keepers.Deployment.UpdateDeployment(s.ctx, deployment)
	deploymentDetails, ok := s.app.Keepers.Deployment.GetDeployment(s.ctx, deploymentID)
	s.Require().Equal(types.DeploymentClosed, deploymentDetails.State, "Update deployment failed")

	s.T().Log("verify update deployment with wrong deploymentID")
	deploymentID = types.DeploymentID{
		Owner: addr2,
		DSeq:  uint64(136),
	}
	deployment = types.Deployment{
		DeploymentID: deploymentID,
		State:        types.DeploymentClosed,
	}
	err = s.app.Keepers.Deployment.UpdateDeployment(s.ctx, deployment)
	s.Require().NotNil(err, "Update deployment failed with wrong data")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
