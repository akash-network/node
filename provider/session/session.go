package session

import (
	"github.com/tendermint/tendermint/libs/log"

	"github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/sdkutil"
	pquery "github.com/ovrclk/akash/x/provider/query"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

// Session interface wraps Log, Client, Provider and ForModule methods
type Session interface {
	Log() log.Logger
	Client() client.Client
	Provider() *pquery.Provider
	RequiredAttributes() ptypes.Attributes
	MatchAttributes(ptypes.Attributes) bool
	ForModule(string) Session
}

// New returns new session instance with provided details
func New(log log.Logger, client client.Client, provider *pquery.Provider, requiredAttrs ptypes.Attributes) Session {
	return session{
		client:        client,
		provider:      provider,
		log:           log,
		requiredAttrs: requiredAttrs,
	}
}

type session struct {
	client        client.Client
	provider      *pquery.Provider
	log           log.Logger
	requiredAttrs ptypes.Attributes
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

func (s session) RequiredAttributes() ptypes.Attributes {
	return s.requiredAttrs
}

func (s session) MatchAttributes(attrs ptypes.Attributes) bool {
	return sdkutil.MatchAttributes(s.requiredAttrs, attrs)
}

func (s session) ForModule(name string) Session {
	s.log = s.log.With("module", name)
	return s
}
