// +build mainnet

package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{}
}

func akashKVStoreKeys() []string {
	return []string{}
}

func (app *AkashApp) setAkashKeepers() {
}

func (app *AkashApp) akashAppModules() []module.AppModule {
	return []module.AppModule{}
}

func (app *AkashApp) akashEndBlockModules() []string {
	return []string{}
}

func (app *AkashApp) akashInitGenesisOrder() []string {
	return []string{}
}

func (app *AkashApp) akashSimModules() []module.AppModuleSimulation {
	return []module.AppModuleSimulation{}
}
