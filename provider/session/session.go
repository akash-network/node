package session

import (
	"github.com/tendermint/tendermint/libs/log"

	"github.com/ovrclk/akash/client"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

// Session interface wraps Log, Client, Provider and ForModule methods
type Session interface {
	Log() log.Logger
	Client() client.Client
	Provider() *ptypes.Provider
	ForModule(string) Session
	CreatedAtBlockHeight() int64
}

// New returns new session instance with provided details
func New(log log.Logger, client client.Client, provider *ptypes.Provider, createdAtBlockHeight int64) Session {
	return session{
		client:               client,
		provider:             provider,
		log:                  log,
		createdAtBlockHeight: createdAtBlockHeight,
	}
}

type session struct {
	client               client.Client
	provider             *ptypes.Provider
	log                  log.Logger
	createdAtBlockHeight int64
}

func (s session) Log() log.Logger {
	return s.log
}

func (s session) Client() client.Client {
	return s.client
}

func (s session) Provider() *ptypes.Provider {
	return s.provider
}

func (s session) ForModule(name string) Session {
	s.log = s.log.With("module", name)
	return s
}

func (s session) CreatedAtBlockHeight() int64 {
	return s.createdAtBlockHeight
}
