package types

import (
	"github.com/ovrclk/photon/state"
	"github.com/tendermint/tmlibs/log"
)

type BaseApp struct {
	name  string
	state state.State
	log   log.Logger
}

func NewBaseApp(name string, state state.State, log log.Logger) *BaseApp {
	return &BaseApp{
		name:  name,
		state: state,
		log:   log,
	}
}

func (a *BaseApp) Name() string {
	return a.name
}

func (a *BaseApp) State() state.State {
	return a.state
}

func (a *BaseApp) Log() log.Logger {
	return a.log
}
