package session

import (
	"github.com/tendermint/tendermint/libs/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/client"
)

type Session interface {
	Log() log.Logger
	Client() client.Client
	Provider() sdk.AccAddress
	ForModule(string) Session
}

func New(log log.Logger, client client.Client, provider sdk.AccAddress) Session {
	return session{
		client:   client,
		provider: provider,
		log:      log,
	}
}

type session struct {
	client   client.Client
	provider sdk.AccAddress
	log      log.Logger
}

func (s session) Log() log.Logger {
	return s.log
}

func (s session) Client() client.Client {
	return s.client
}

func (s session) Provider() sdk.AccAddress {
	return s.provider
}

func (s session) ForModule(name string) Session {
	s.log = s.log.With("module", name)
	return s
}
