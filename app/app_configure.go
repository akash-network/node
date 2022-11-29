package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	icatypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/types"
	transfertypes "github.com/cosmos/ibc-go/v3/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v3/modules/core/24-host"

	"github.com/ovrclk/akash/x/audit"
	audittypes "github.com/ovrclk/akash/x/audit/types/v1beta2"
	"github.com/ovrclk/akash/x/cert"
	certtypes "github.com/ovrclk/akash/x/cert/types/v1beta2"
	"github.com/ovrclk/akash/x/deployment"
	deploymenttypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	"github.com/ovrclk/akash/x/escrow"
	ekeeper "github.com/ovrclk/akash/x/escrow/keeper"
	escrowtypes "github.com/ovrclk/akash/x/escrow/types/v1beta2"
	icaauthtypes "github.com/ovrclk/akash/x/icaauth/types/v1beta2"
	"github.com/ovrclk/akash/x/inflation"
	inflationtypes "github.com/ovrclk/akash/x/inflation/types/v1beta2"
	"github.com/ovrclk/akash/x/market"
	mhooks "github.com/ovrclk/akash/x/market/hooks"
	markettypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/ovrclk/akash/x/provider"
	providertypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{
		escrow.AppModuleBasic{},
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
		audit.AppModuleBasic{},
		cert.AppModuleBasic{},
		inflation.AppModuleBasic{},
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
		inflation.StoreKey,
	}
}

func akashSubspaces(k paramskeeper.Keeper) paramskeeper.Keeper {
	k.Subspace(deployment.ModuleName)
	k.Subspace(market.ModuleName)
	k.Subspace(inflation.ModuleName)
	return k
}

func (app *AkashApp) setAkashKeepers() {

	app.Keeper.escrow = ekeeper.NewKeeper(
		app.appCodec,
		app.keys[escrow.StoreKey],
		app.Keeper.Bank,
	)

	app.Keeper.deployment = deployment.NewKeeper(
		app.appCodec,
		app.keys[deployment.StoreKey],
		app.GetSubspace(deployment.ModuleName),
		app.Keeper.escrow,
	)

	app.Keeper.market = market.NewKeeper(
		app.appCodec,
		app.keys[market.StoreKey],
		app.GetSubspace(market.ModuleName),
		app.Keeper.escrow,
	)

	hook := mhooks.New(app.Keeper.deployment, app.Keeper.market)

	app.Keeper.escrow.AddOnAccountClosedHook(hook.OnEscrowAccountClosed)
	app.Keeper.escrow.AddOnPaymentClosedHook(hook.OnEscrowPaymentClosed)

	app.Keeper.provider = provider.NewKeeper(
		app.appCodec,
		app.keys[provider.StoreKey],
	)

	app.Keeper.audit = audit.NewKeeper(
		app.appCodec,
		app.keys[audit.StoreKey],
	)

	app.Keeper.cert = cert.NewKeeper(
		app.appCodec,
		app.keys[cert.StoreKey],
	)

	app.Keeper.inflation = inflation.NewKeeper(
		app.appCodec,
		app.keys[inflation.StoreKey],
		app.GetSubspace(inflation.ModuleName),
	)
}

func (app *AkashApp) akashAppModules() []module.AppModule {
	return []module.AppModule{

		escrow.NewAppModule(
			app.appCodec,
			app.Keeper.escrow,
		),

		deployment.NewAppModule(
			app.appCodec,
			app.Keeper.deployment,
			app.Keeper.market,
			app.Keeper.escrow,
			app.Keeper.Bank,
			app.Keeper.authz,
		),

		market.NewAppModule(
			app.appCodec,
			app.Keeper.market,
			app.Keeper.escrow,
			app.Keeper.audit,
			app.Keeper.deployment,
			app.Keeper.provider,
			app.Keeper.Bank,
		),

		provider.NewAppModule(
			app.appCodec,
			app.Keeper.provider,
			app.Keeper.Bank,
			app.Keeper.market,
		),

		audit.NewAppModule(
			app.appCodec,
			app.Keeper.audit,
		),

		cert.NewAppModule(
			app.appCodec,
			app.Keeper.cert,
		),

		inflation.NewAppModule(
			app.appCodec,
			app.Keeper.inflation,
		),
	}
}

// akashEndBlockModules returns all end block modules.
func (app *AkashApp) akashBeginBlockModules() []string {
	return []string{
		upgradetypes.ModuleName,
		capabilitytypes.ModuleName,
		banktypes.ModuleName,
		paramstypes.ModuleName,
		deploymenttypes.ModuleName,
		govtypes.ModuleName,
		providertypes.ModuleName,
		certtypes.ModuleName,
		markettypes.ModuleName,
		audittypes.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		crisistypes.ModuleName,
		inflationtypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		escrowtypes.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		stakingtypes.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		icatypes.ModuleName,
		icaauthtypes.ModuleName,
	}
}

// akashEndBlockModules returns all end block modules.
func (app *AkashApp) akashEndBlockModules() []string {
	return []string{
		crisistypes.ModuleName,
		govtypes.ModuleName,
		stakingtypes.ModuleName,
		upgradetypes.ModuleName,
		capabilitytypes.ModuleName,
		banktypes.ModuleName,
		paramstypes.ModuleName,
		deploymenttypes.ModuleName,
		providertypes.ModuleName,
		certtypes.ModuleName,
		markettypes.ModuleName,
		audittypes.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		inflationtypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		escrowtypes.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		icatypes.ModuleName,
		icaauthtypes.ModuleName,
	}
}

func (app *AkashApp) akashInitGenesisOrder() []string {
	return []string{
		capabilitytypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		banktypes.ModuleName,
		distrtypes.ModuleName,
		stakingtypes.ModuleName,
		slashingtypes.ModuleName,
		govtypes.ModuleName,
		vestingtypes.ModuleName,
		paramstypes.ModuleName,
		audittypes.ModuleName,
		upgradetypes.ModuleName,
		minttypes.ModuleName,
		crisistypes.ModuleName,
		ibchost.ModuleName,
		genutiltypes.ModuleName,
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		icatypes.ModuleName,
		icaauthtypes.ModuleName,
		cert.ModuleName,
		escrow.ModuleName,
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
		inflation.ModuleName,
	}
}

func (app *AkashApp) akashSimModules() []module.AppModuleSimulation {
	return []module.AppModuleSimulation{
		deployment.NewAppModuleSimulation(
			app.Keeper.deployment,
			app.Keeper.acct,
			app.Keeper.Bank,
		),

		market.NewAppModuleSimulation(
			app.Keeper.market,
			app.Keeper.acct,
			app.Keeper.deployment,
			app.Keeper.provider,
			app.Keeper.Bank,
		),

		provider.NewAppModuleSimulation(
			app.Keeper.provider,
			app.Keeper.acct,
			app.Keeper.Bank,
		),

		cert.NewAppModuleSimulation(
			app.Keeper.cert,
		),

		inflation.NewAppModuleSimulation(
			app.Keeper.inflation,
		),
	}
}
