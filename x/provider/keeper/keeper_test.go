package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/x/provider/config"
	"github.com/ovrclk/akash/x/provider/types"
)

// testing vars
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

	s.T().Log("verify provider is created")
	cfg, err := config.ReadConfigPath("../testdata/provider.yml")
	s.Require().NoError(err, "Error in reading file")
	msg := types.MsgCreateProvider{
		Owner:      ownerAddr,
		HostURI:    cfg.Host,
		Attributes: cfg.GetAttributes(),
	}
	s.app.Keepers.Provider.Create(s.ctx, types.Provider(msg))
	_, ok := s.app.Keepers.Provider.Get(s.ctx, ownerAddr)
	s.Require().True(ok, "Provider not created")

	s.T().Log("verify get provider with wrong owner")
	_, ok = s.app.Keepers.Provider.Get(s.ctx, addr2)
	s.Require().False(ok, "Get Provider failed")

	s.T().Log("verify update provider")
	host := "akash.domain.com"
	msg.HostURI = host
	s.app.Keepers.Provider.Update(s.ctx, types.Provider(msg))
	provider, _ := s.app.Keepers.Provider.Get(s.ctx, ownerAddr)
	s.Require().Equal(host, provider.HostURI, "Provider not updated")
}

func TestTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}
