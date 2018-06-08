package types

import (
	"github.com/ovrclk/akash/state"
	"github.com/tendermint/tmlibs/log"
)

type BaseApp struct {
	name        string
	cacheState  state.CacheState
	commitState state.CommitState
	log         log.Logger
}

func NewBaseApp(name string, commitState state.CommitState, cacheState state.CacheState, log log.Logger) *BaseApp {
	return &BaseApp{
		name:        name,
		commitState: commitState,
		cacheState:  cacheState,
		log:         log,
	}
}

func (a *BaseApp) Name() string {
	return a.name
}

func (a *BaseApp) CommitState() state.CommitState {
	return a.commitState
}

func (a *BaseApp) CacheState() state.CacheState {
	return a.cacheState
}

func (a *BaseApp) Log() log.Logger {
	return a.log
}
