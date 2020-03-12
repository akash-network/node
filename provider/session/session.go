package session

import (
	"github.com/tendermint/tendermint/libs/log"

	"github.com/ovrclk/akash/client"
	pquery "github.com/ovrclk/akash/x/provider/query"
)

// Session interface wraps Log, Client, Provider and ForModule methods
type Session interface {
	Log() log.Logger
	Client() client.Client
	Provider() *pquery.Provider
	ForModule(string) Session
}

// New returns new session instance with provided details
func New(log log.Logger, client client.Client, provider *pquery.Provider) Session {
	return session{
		client:   client,
		provider: provider,
		log:      log,
	}
}

type session struct {
	client   client.Client
	provider *pquery.Provider
	log      log.Logger
}

func (s session) Log() log.Logger {
	return s.log
}

func (s session) Client() client.Client {
	return s.client
}

func (s session) Provider() *pquery.Provider {
	return s.provider
}

func (s session) ForModule(name string) Session {
	s.log = s.log.With("module", name)
	return s
}
