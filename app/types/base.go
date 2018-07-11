package types

import (
	"github.com/tendermint/tendermint/libs/log"
)

type BaseApp struct {
	name string

	log log.Logger
}

func NewBaseApp(name string, log log.Logger) *BaseApp {
	return &BaseApp{
		name: name,
		log:  log,
	}
}

func (a *BaseApp) Name() string {
	return a.name
}

func (a *BaseApp) Log() log.Logger {
	return a.log
}
