// +build !mainnet

package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
	}
}

func akashKVStoreKeys() []string {
	return []string{
		deployment.StoreKey,
		market.StoreKey,
		provider.StoreKey,
	}
}

func (app *AkashApp) setAkashKeepers() {
	app.keeper.deployment = deployment.NewKeeper(
		app.cdc,
		app.keys[deployment.StoreKey],
	)

	app.keeper.market = market.NewKeeper(
		app.cdc,
		app.keys[market.StoreKey],
	)

	app.keeper.provider = provider.NewKeeper(
		app.cdc,
		app.keys[provider.StoreKey],
	)
}

func (app *AkashApp) akashAppModules() []module.AppModule {
	return []module.AppModule{
		deployment.NewAppModule(
			app.keeper.deployment,
			app.keeper.market,
			app.keeper.bank,
		),

		market.NewAppModule(
			app.keeper.market,
			app.keeper.deployment,
			app.keeper.provider,
			app.keeper.bank,
		),

		provider.NewAppModule(app.keeper.provider, app.keeper.bank, app.keeper.market),
	}
}

func (app *AkashApp) akashEndBlockModules() []string {
	return []string{
		deployment.ModuleName, market.ModuleName,
	}
}

func (app *AkashApp) akashInitGenesisOrder() []string {
	return []string{
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
	}
}

func (app *AkashApp) akashSimModules() []module.AppModuleSimulation {
	return []module.AppModuleSimulation{
		deployment.NewAppModuleSimulation(app.keeper.deployment, app.keeper.acct),
		market.NewAppModuleSimulation(app.keeper.market, app.keeper.acct, app.keeper.deployment,
			app.keeper.provider, app.keeper.bank),
		provider.NewAppModuleSimulation(app.keeper.provider, app.keeper.acct),
	}
}
