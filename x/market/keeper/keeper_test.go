package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/sdl"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
)

// testing vars
var (
	ownerPub     = ed25519.GenPrivKey().PubKey()
	ownerAddr    = sdk.AccAddress(ownerPub.Address())
	providerPub  = ed25519.GenPrivKey().PubKey()
	providerAddr = sdk.AccAddress(providerPub.Address())
	addr2Pub     = ed25519.GenPrivKey().PubKey()
	addr2        = sdk.AccAddress(addr2Pub.Address())
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
	const DENOM string = "stake"
	s.T().Log("Adding balance to owner account")
	err := s.app.Keepers.Bank.SetCoins(s.ctx, ownerAddr, sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000)))
	s.Require().Nil(err)
	s.Require().True(s.app.Keepers.Bank.GetCoins(s.ctx, ownerAddr).IsEqual(sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000))))

	s.T().Log("Adding balance to provider account")
	err = s.app.Keepers.Bank.SetCoins(s.ctx, providerAddr, sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000)))
	s.Require().Nil(err)
	s.Require().True(s.app.Keepers.Bank.GetCoins(s.ctx, providerAddr).IsEqual(sdk.NewCoins(sdk.NewInt64Coin(DENOM, 10000))))

	s.T().Log("verify deployment is created")
	sdl, readError := sdl.ReadFile("../../deployment/testdata/deployment.yml")
	s.Require().NoError(readError, "Error in reading file")
	groupSpecs, err := sdl.DeploymentGroups()
	s.Require().NoError(err, "Error in getting groups from file")
	msg := dtypes.MsgCreateDeployment{
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
		order := s.app.Keepers.Market.CreateOrder(s.ctx, groups[0].GroupID, groups[0].GroupSpec)
		_, ok := s.app.Keepers.Market.GetOrder(s.ctx, order.ID())
		s.Require().True(ok, "Order not created")

		s.T().Log("verify create bid")
		bidID := types.BidID{
			Owner:    order.OrderID.Owner,
			DSeq:     order.OrderID.DSeq,
			GSeq:     order.OrderID.GSeq,
			OSeq:     order.OrderID.OSeq,
			Provider: providerAddr,
		}
		s.app.Keepers.Market.CreateBid(s.ctx, order.ID(), providerAddr, sdk.NewInt64Coin(DENOM, 10))
		bid, ok := s.app.Keepers.Market.GetBid(s.ctx, bidID)
		s.Require().True(ok, "Bid not created")

		s.T().Log("verify create lease")
		s.app.Keepers.Market.CreateLease(s.ctx, bid)
		lease := types.Lease{
			LeaseID: types.LeaseID(bid.ID()),
			Price:   bid.Price,
		}
		_, ok = s.app.Keepers.Market.GetLease(s.ctx, lease.LeaseID)
		s.Require().True(ok, "Lease not created")

		s.T().Log("verify on bid matched")
		s.app.Keepers.Market.OnBidMatched(s.ctx, bid)
		bidDetails, _ := s.app.Keepers.Market.GetBid(s.ctx, bidID)
		s.Require().Equal(types.BidMatched, bidDetails.State, "OnBidMatched failed")

		s.T().Log("verify lease for order")
		_, ok = s.app.Keepers.Market.LeaseForOrder(s.ctx, order.ID())
		s.Require().True(ok, "LeaseForOrder failed")

		s.T().Log("verify lease on insufficient funds")
		s.app.Keepers.Market.OnInsufficientFunds(s.ctx, lease)
		leaseDetails, _ := s.app.Keepers.Market.GetLease(s.ctx, lease.LeaseID)
		s.Require().Equal(types.LeaseInsufficientFunds, leaseDetails.State, "OnInsufficientFunds failed")

		s.T().Log("verify lease on closed")
		s.app.Keepers.Market.CreateLease(s.ctx, bid)
		s.app.Keepers.Market.OnLeaseClosed(s.ctx, lease)
		leaseDetails, _ = s.app.Keepers.Market.GetLease(s.ctx, lease.LeaseID)
		s.Require().Equal(types.LeaseClosed, leaseDetails.State, "LeaseOnClosed failed")

		s.T().Log("verify on bid closed")
		s.app.Keepers.Market.OnBidClosed(s.ctx, bid)
		bidDetails, _ = s.app.Keepers.Market.GetBid(s.ctx, bidID)
		s.Require().Equal(types.BidClosed, bidDetails.State, "OnBidClosed failed")

		s.T().Log("verify on bid lost")
		s.app.Keepers.Market.OnBidLost(s.ctx, bid)
		bidDetails, _ = s.app.Keepers.Market.GetBid(s.ctx, bidID)
		s.Require().Equal(types.BidLost, bidDetails.State, "OnBidLost failed")

		s.T().Log("verify on order matched")
		s.app.Keepers.Market.OnOrderMatched(s.ctx, order)
		orderDetails, _ := s.app.Keepers.Market.GetOrder(s.ctx, order.ID())
		s.Require().Equal(types.OrderMatched, orderDetails.State, "OnOrderMatched failed")

		s.T().Log("verify on order closed")
		s.app.Keepers.Market.OnOrderClosed(s.ctx, order)
		orderDetails, _ = s.app.Keepers.Market.GetOrder(s.ctx, order.ID())
		s.Require().Equal(types.OrderClosed, orderDetails.State, "OnOrderClosed failed")
	}

	s.T().Log("verify get order with wrong orderID")
	_, ok := s.app.Keepers.Market.GetOrder(s.ctx, types.OrderID{
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
	_, ok = s.app.Keepers.Market.GetBid(s.ctx, bidID2)
	s.Require().False(ok, "Get bid failed")

	s.T().Log("verify get lease with wrong LeaseID")
	_, ok = s.app.Keepers.Market.GetLease(s.ctx, types.LeaseID(bidID2))
	s.Require().False(ok, "Get Lease Failed")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
