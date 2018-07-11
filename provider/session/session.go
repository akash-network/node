package session

import (
	"github.com/tendermint/tendermint/libs/log"

	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
)

type Session interface {
	Provider() *types.Provider

	TX() txutil.Client
	Query() query.Client

	Log() log.Logger

	ForModule(string) Session
}

func New(log log.Logger, provider *types.Provider, txc txutil.Client, qc query.Client) Session {
	return &session{
		provider: provider,
		txc:      txc,
		qc:       qc,
		log:      log,
	}
}

type session struct {
	provider *types.Provider

	txc txutil.Client
	qc  query.Client
	log log.Logger
}

func (s *session) Provider() *types.Provider {
	return s.provider
}

func (s *session) TX() txutil.Client {
	return s.txc
}

func (s *session) Query() query.Client {
	return s.qc
}

func (s *session) Log() log.Logger {
	return s.log
}

func (s *session) ForModule(name string) Session {
	return &session{
		provider: s.provider,
		txc:      s.txc,
		qc:       s.qc,
		log:      s.log.With("module", name),
	}
}
