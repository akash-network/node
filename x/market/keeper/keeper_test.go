package keeper

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/ovrclk/akash/sdl"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
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
	const DENOM string = "stake"
	s.T().Log("Adding balance to owner account")
	err := s.bankKeeper.SetBalances(s.ctx, ownerAddr, sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000)))
	s.Require().Nil(err)
	s.Require().True(s.bankKeeper.GetAllBalances(s.ctx, ownerAddr).IsEqual(sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000))))

	s.T().Log("Adding balance to provider account")
	err = s.bankKeeper.SetBalances(s.ctx, providerAddr, sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000)))
	s.Require().Nil(err)
	s.Require().True(s.bankKeeper.GetAllBalances(s.ctx, providerAddr).IsEqual(sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000))))

	s.T().Log("verify deployment is created")
	sdl, readError := sdl.ReadFile("../../deployment/testdata/deployment.yml")
	s.Require().NoError(readError, "Error in reading file")
	groupSpecs, err := sdl.DeploymentGroups()
	s.Require().NoError(err, "Error in getting groups from file")
	msg := dtypes.MsgCreate{
		Owner:  ownerAddr,
		Groups: make([]dtypes.GroupSpec, 0, len(groupSpecs)),
	}
	deploymentID := dtypes.DeploymentID{
		Owner: msg.Owner,
		DSeq:  uint64(131),
	}
	deployment := dtypes.Deployment{
		DeploymentID: deploymentID,
		State:        dtypes.DeploymentActive,
	}
	for _, spec := range groupSpecs {
		msg.Groups = append(msg.Groups, *spec)
	}
	groups := make([]dtypes.Group, 0, len(msg.Groups))
	for idx, spec := range msg.Groups {
		groups = append(groups, dtypes.Group{
			GroupID:   dtypes.MakeGroupID(deployment.ID(), uint32(idx+1)),
			State:     dtypes.GroupOpen,
			GroupSpec: spec,
		})
	}

	if len(groups) > 0 {
		s.T().Log("verify create order")
		order := s.keeper.CreateOrder(s.ctx, groups[0].GroupID, groups[0].GroupSpec)
		_, ok := s.keeper.GetOrder(s.ctx, order.ID())
		s.Require().True(ok, "Order not created")

		s.T().Log("verify create bid")
		bidID := types.BidID{
			Owner:    order.OrderID.Owner,
			DSeq:     order.OrderID.DSeq,
			GSeq:     order.OrderID.GSeq,
			OSeq:     order.OrderID.OSeq,
			Provider: providerAddr,
		}
		s.keeper.CreateBid(s.ctx, order.ID(), providerAddr, sdk.NewInt64Coin(DENOM, 10))
		bid, ok := s.keeper.GetBid(s.ctx, bidID)
		s.Require().True(ok, "Bid not created")

		s.T().Log("verify create lease")
		s.keeper.CreateLease(s.ctx, bid)
		lease := types.Lease{
			LeaseID: types.LeaseID(bid.ID()),
			Price:   bid.Price,
		}
		_, ok = s.keeper.GetLease(s.ctx, lease.LeaseID)
		s.Require().True(ok, "Lease not created")

		s.T().Log("verify on bid matched")
		s.keeper.OnBidMatched(s.ctx, bid)
		bidDetails, _ := s.keeper.GetBid(s.ctx, bidID)
		s.Require().Equal(types.BidMatched, bidDetails.State, "OnBidMatched failed")

		s.T().Log("verify lease for order")
		_, ok = s.keeper.LeaseForOrder(s.ctx, order.ID())
		s.Require().True(ok, "LeaseForOrder failed")

		s.T().Log("verify lease on insufficient funds")
		s.keeper.OnInsufficientFunds(s.ctx, lease)
		leaseDetails, _ := s.keeper.GetLease(s.ctx, lease.LeaseID)
		s.Require().Equal(types.LeaseInsufficientFunds, leaseDetails.State, "OnInsufficientFunds failed")

		s.T().Log("verify lease on closed")
		s.keeper.CreateLease(s.ctx, bid)
		s.keeper.OnLeaseClosed(s.ctx, lease)
		leaseDetails, _ = s.keeper.GetLease(s.ctx, lease.LeaseID)
		s.Require().Equal(types.LeaseClosed, leaseDetails.State, "LeaseOnClosed failed")

		s.T().Log("verify on bid closed")
		s.keeper.OnBidClosed(s.ctx, bid)
		bidDetails, _ = s.keeper.GetBid(s.ctx, bidID)
		s.Require().Equal(types.BidClosed, bidDetails.State, "OnBidClosed failed")

		s.T().Log("verify on bid lost")
		s.keeper.OnBidLost(s.ctx, bid)
		bidDetails, _ = s.keeper.GetBid(s.ctx, bidID)
		s.Require().Equal(types.BidLost, bidDetails.State, "OnBidLost failed")

		s.T().Log("verify on order matched")
		s.keeper.OnOrderMatched(s.ctx, order)
		orderDetails, _ := s.keeper.GetOrder(s.ctx, order.ID())
		s.Require().Equal(types.OrderMatched, orderDetails.State, "OnOrderMatched failed")

		s.T().Log("verify on order closed")
		s.keeper.OnOrderClosed(s.ctx, order)
		orderDetails, _ = s.keeper.GetOrder(s.ctx, order.ID())
		s.Require().Equal(types.OrderClosed, orderDetails.State, "OnOrderClosed failed")
	}

	s.T().Log("verify get order with wrong orderID")
	_, ok := s.keeper.GetOrder(s.ctx, types.OrderID{
		Owner: addr2,
		DSeq:  134,
		GSeq:  20,
		OSeq:  2,
	})
	s.Require().False(ok, "Get order failed")

	s.T().Log("verify get bid with wrong bidID")
	bidID2 := types.BidID{
		Owner:    addr2,
		DSeq:     134,
		GSeq:     20,
		OSeq:     2,
		Provider: addr2,
	}
	_, ok = s.keeper.GetBid(s.ctx, bidID2)
	s.Require().False(ok, "Get bid failed")

	s.T().Log("verify get lease with wrong LeaseID")
	_, ok = s.keeper.GetLease(s.ctx, types.LeaseID(bidID2))
	s.Require().False(ok, "Get Lease Failed")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
