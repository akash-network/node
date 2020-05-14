package keeper

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/suite"
)

type TestSuite struct {
	suite.Suite
	ctx           sdk.Context
	accountKeeper auth.AccountKeeper
	paramsKeeper  params.Keeper
	bankKeeper    bank.Keeper
	keeper        Keeper
}

func (s *TestSuite) SetupTest() {
	s.ctx, s.accountKeeper, s.paramsKeeper, s.bankKeeper, s.keeper = SetupTestInput()
}

func (s *TestSuite) TestKeeper() {
	s.T().Log("Adding balance to account")
	err := s.bankKeeper.SetCoins(s.ctx, ownerAddr, sdk.NewCoins(sdk.NewInt64Coin("stake", 10000)))
	s.Require().Nil(err)
	s.Require().True(s.bankKeeper.GetCoins(s.ctx, ownerAddr).IsEqual(sdk.NewCoins(sdk.NewInt64Coin("stake", 10000))))

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
	s.keeper.Create(s.ctx, deployment, groups)
	_, ok := s.keeper.GetDeployment(s.ctx, deploymentID)
	s.Require().True(ok, "Deployment not created")

	s.T().Log("verify get deployment with wrong deploymentID")
	_, ok = s.keeper.GetDeployment(s.ctx, types.DeploymentID{
		Owner: addr2,
		DSeq:  uint64(135),
	})
	s.Require().False(ok, "Get deployment failed")

	if len(groups) > 0 {
		s.T().Log("verify get groups with deploymentID")
		depGroups := s.keeper.GetGroups(s.ctx, deploymentID)
		s.Require().Equal(groups, depGroups, "Get Groups failed")

		s.T().Log("verify get group with groupID")
		_, ok = s.keeper.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().True(ok, "Get Group failed")

		s.T().Log("verify on order created")
		s.keeper.OnOrderCreated(s.ctx, groups[0])
		details, _ := s.keeper.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupOrdered, details.State, "OnOrderCreated failed")

		s.T().Log("verify on lease created")
		s.keeper.OnLeaseCreated(s.ctx, groups[0].GroupID)
		details, _ = s.keeper.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupMatched, details.State, "OnLeaseCreated failed")

		s.T().Log("verify on lease insufficient funds")
		s.keeper.OnLeaseInsufficientFunds(s.ctx, groups[0].GroupID)
		details, _ = s.keeper.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupInsufficientFunds, details.State, "OnLeaseInsufficientFunds failed")

		s.T().Log("verify on lease closed")
		s.keeper.OnLeaseClosed(s.ctx, groups[0].GroupID)
		details, _ = s.keeper.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupOpen, details.State, "OnLeaseClosed failed")

		s.T().Log("verify on deployment closed")
		s.keeper.OnDeploymentClosed(s.ctx, groups[0])
		details, _ = s.keeper.GetGroup(s.ctx, groups[0].GroupID)
		s.Require().Equal(types.GroupClosed, details.State, "OnDeploymentClosed failed")
	}

	s.T().Log("verify get groups with wrong deploymentID")
	depGroups := s.keeper.GetGroups(s.ctx, types.DeploymentID{
		Owner: addr2,
		DSeq:  uint64(136),
	})
	s.Require().Equal([]types.Group(nil), depGroups, "Get Groups failed with wrong data")

	s.T().Log("verify get group with wrong groupID")
	_, ok = s.keeper.GetGroup(s.ctx, types.GroupID{
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
	s.keeper.UpdateDeployment(s.ctx, deployment)
	deploymentDetails, _ := s.keeper.GetDeployment(s.ctx, deploymentID)
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
	err = s.keeper.UpdateDeployment(s.ctx, deployment)
	s.Require().NotNil(err, "Update deployment failed with wrong data")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
