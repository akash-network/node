package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"

	"github.com/ovrclk/akash/x/audit"
	"github.com/ovrclk/akash/x/cert"
	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/escrow"
	ekeeper "github.com/ovrclk/akash/x/escrow/keeper"
	"github.com/ovrclk/akash/x/market"
	mhooks "github.com/ovrclk/akash/x/market/hooks"
	"github.com/ovrclk/akash/x/provider"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{
		escrow.AppModuleBasic{},
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
		audit.AppModuleBasic{},
		cert.AppModuleBasic{},
	}
}

func akashKVStoreKeys() []string {
	return []string{
		escrow.StoreKey,
		deployment.StoreKey,
		market.StoreKey,
		provider.StoreKey,
		audit.StoreKey,
		cert.StoreKey,
	}
}

func akashSubspaces(k paramskeeper.Keeper) paramskeeper.Keeper {
	k.Subspace(deployment.ModuleName)
	k.Subspace(market.ModuleName)
	return k
}

func (app *AkashApp) setAkashKeepers() {

	app.keeper.escrow = ekeeper.NewKeeper(
		app.appCodec,
		app.keys[escrow.StoreKey],
		app.keeper.bank,
	)

	app.keeper.deployment = deployment.NewKeeper(
		app.appCodec,
		app.keys[deployment.StoreKey],
		app.GetSubspace(deployment.ModuleName),
		app.keeper.escrow,
	)

	app.keeper.market = market.NewKeeper(
		app.appCodec,
		app.keys[market.StoreKey],
		app.GetSubspace(market.ModuleName),
		app.keeper.escrow,
	)

	hook := mhooks.New(app.keeper.deployment, app.keeper.market)

	app.keeper.escrow.AddOnAccountClosedHook(hook.OnEscrowAccountClosed)
	app.keeper.escrow.AddOnPaymentClosedHook(hook.OnEscrowPaymentClosed)

	app.keeper.provider = provider.NewKeeper(
		app.appCodec,
		app.keys[provider.StoreKey],
	)

	app.keeper.audit = audit.NewKeeper(
		app.appCodec,
		app.keys[audit.StoreKey],
	)

	app.keeper.cert = cert.NewKeeper(
		app.appCodec,
		app.keys[cert.StoreKey],
	)
}

func (app *AkashApp) akashAppModules() []module.AppModule {
	return []module.AppModule{

		escrow.NewAppModule(
			app.appCodec,
			app.keeper.escrow,
		),

		deployment.NewAppModule(
			app.appCodec,
			app.keeper.deployment,
			app.keeper.market,
			app.keeper.escrow,
			app.keeper.bank,
		),

		market.NewAppModule(
			app.appCodec,
			app.keeper.market,
			app.keeper.escrow,
			app.keeper.audit,
			app.keeper.deployment,
			app.keeper.provider,
			app.keeper.bank,
		),

		provider.NewAppModule(
			app.appCodec,
			app.keeper.provider,
			app.keeper.bank,
			app.keeper.market,
		),

		audit.NewAppModule(
			app.appCodec,
			app.keeper.audit,
		),

		cert.NewAppModule(
			app.appCodec,
			app.keeper.cert,
		),
	}
}

func (app *AkashApp) akashEndBlockModules() []string {
	return []string{
		deployment.ModuleName, market.ModuleName,
	}
}

func (app *AkashApp) akashInitGenesisOrder() []string {
	return []string{
		cert.ModuleName,
		escrow.ModuleName,
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
	}
}

func (app *AkashApp) akashSimModules() []module.AppModuleSimulation {
	return []module.AppModuleSimulation{
		deployment.NewAppModuleSimulation(
			app.keeper.deployment,
			app.keeper.acct,
			app.keeper.bank,
		),

		market.NewAppModuleSimulation(
			app.keeper.market,
			app.keeper.acct,
			app.keeper.deployment,
			app.keeper.provider,
			app.keeper.bank,
		),

		provider.NewAppModuleSimulation(
			app.keeper.provider,
			app.keeper.acct,
			app.keeper.bank,
		),

		cert.NewAppModuleSimulation(
			app.keeper.cert,
		),
	}
}
