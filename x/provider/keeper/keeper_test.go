package keeper

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/ovrclk/akash/x/provider/config"
	"github.com/ovrclk/akash/x/provider/types"
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

	s.T().Log("verify provider is created")
	cfg, err := config.ReadConfigPath("../testdata/provider.yml")
	s.Require().NoError(err, "Error in reading file")
	msg := types.MsgCreate{
		Owner:      ownerAddr,
		HostURI:    cfg.Host,
		Attributes: cfg.GetAttributes(),
	}
	s.keeper.Create(s.ctx, types.Provider(msg))
	_, ok := s.keeper.Get(s.ctx, ownerAddr)
	s.Require().True(ok, "Provider not created")

	s.T().Log("verify get provider with wrong owner")
	_, ok = s.keeper.Get(s.ctx, addr2)
	s.Require().False(ok, "Get Provider failed")

	s.T().Log("verify update provider")
	host := "akash.domain.com"
	msg.HostURI = host
	s.keeper.Update(s.ctx, types.Provider(msg))
	provider, _ := s.keeper.Get(s.ctx, ownerAddr)
	s.Require().Equal(host, provider.HostURI, "Provider not updated")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
