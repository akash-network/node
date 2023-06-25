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
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	transfertypes "github.com/cosmos/ibc-go/v4/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v4/modules/core/24-host"

	audittypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
	certtypes "github.com/akash-network/akash-api/go/node/cert/v1beta3"
	deploymenttypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	escrowtypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
	inflationtypes "github.com/akash-network/akash-api/go/node/inflation/v1beta3"
	markettypes "github.com/akash-network/akash-api/go/node/market/v1beta3"
	providertypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"
	taketypes "github.com/akash-network/akash-api/go/node/take/v1beta3"

	"github.com/akash-network/node/x/audit"
	akeeper "github.com/akash-network/node/x/audit/keeper"
	"github.com/akash-network/node/x/cert"
	ckeeper "github.com/akash-network/node/x/cert/keeper"
	"github.com/akash-network/node/x/deployment"
	"github.com/akash-network/node/x/escrow"
	ekeeper "github.com/akash-network/node/x/escrow/keeper"
	agov "github.com/akash-network/node/x/gov"
	agovkeeper "github.com/akash-network/node/x/gov/keeper"
	"github.com/akash-network/node/x/inflation"
	ikeeper "github.com/akash-network/node/x/inflation/keeper"
	"github.com/akash-network/node/x/market"
	mhooks "github.com/akash-network/node/x/market/hooks"
	"github.com/akash-network/node/x/provider"
	astaking "github.com/akash-network/node/x/staking"
	astakingkeeper "github.com/akash-network/node/x/staking/keeper"
	"github.com/akash-network/node/x/take"
	tkeeper "github.com/akash-network/node/x/take/keeper"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{
		take.AppModuleBasic{},
		escrow.AppModuleBasic{},
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
		audit.AppModuleBasic{},
		cert.AppModuleBasic{},
		inflation.AppModuleBasic{},
		astaking.AppModuleBasic{},
		agov.AppModuleBasic{},
	}
}

func akashKVStoreKeys() []string {
	return []string{
		take.StoreKey,
		escrow.StoreKey,
		deployment.StoreKey,
		market.StoreKey,
		provider.StoreKey,
		audit.StoreKey,
		cert.StoreKey,
		inflation.StoreKey,
		astaking.StoreKey,
		agov.StoreKey,
	}
}

func akashSubspaces(k paramskeeper.Keeper) paramskeeper.Keeper {
	k.Subspace(deployment.ModuleName)
	k.Subspace(market.ModuleName)
	k.Subspace(inflation.ModuleName)
	k.Subspace(astaking.ModuleName)
	k.Subspace(agov.ModuleName)
	k.Subspace(take.ModuleName)
	return k
}

func (app *AkashApp) setAkashKeepers() {
	app.Keepers.Akash.Take = tkeeper.NewKeeper(
		app.appCodec,
		app.keys[take.StoreKey],
		app.GetSubspace(take.ModuleName),
	)

	app.Keepers.Akash.Escrow = ekeeper.NewKeeper(
		app.appCodec,
		app.keys[escrow.StoreKey],
		app.Keepers.Cosmos.Bank,
		app.Keepers.Akash.Take,
		app.Keepers.Cosmos.Distr,
	)

	app.Keepers.Akash.Deployment = deployment.NewKeeper(
		app.appCodec,
		app.keys[deployment.StoreKey],
		app.GetSubspace(deployment.ModuleName),
		app.Keepers.Akash.Escrow,
	)

	app.Keepers.Akash.Market = market.NewKeeper(
		app.appCodec,
		app.keys[market.StoreKey],
		app.GetSubspace(market.ModuleName),
		app.Keepers.Akash.Escrow,
	)

	hook := mhooks.New(
		app.Keepers.Akash.Deployment,
		app.Keepers.Akash.Market,
	)

	app.Keepers.Akash.Escrow.AddOnAccountClosedHook(hook.OnEscrowAccountClosed)
	app.Keepers.Akash.Escrow.AddOnPaymentClosedHook(hook.OnEscrowPaymentClosed)

	app.Keepers.Akash.Provider = provider.NewKeeper(
		app.appCodec,
		app.keys[provider.StoreKey],
	)

	app.Keepers.Akash.Audit = akeeper.NewKeeper(
		app.appCodec,
		app.keys[audit.StoreKey],
	)

	app.Keepers.Akash.Cert = ckeeper.NewKeeper(
		app.appCodec,
		app.keys[cert.StoreKey],
	)

	app.Keepers.Akash.Inflation = ikeeper.NewKeeper(
		app.appCodec,
		app.keys[inflation.StoreKey],
		app.GetSubspace(inflation.ModuleName),
	)

	app.Keepers.Akash.Staking = astakingkeeper.NewKeeper(
		app.appCodec,
		app.keys[astaking.StoreKey],
		app.GetSubspace(astaking.ModuleName),
	)

	app.Keepers.Akash.Gov = agovkeeper.NewKeeper(
		app.appCodec,
		app.keys[agov.StoreKey],
		app.GetSubspace(agov.ModuleName),
	)
}

func (app *AkashApp) akashAppModules() []module.AppModule {
	return []module.AppModule{
		take.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Take,
		),

		escrow.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Escrow,
		),

		deployment.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Market,
			app.Keepers.Akash.Escrow,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Authz,
		),

		market.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Market,
			app.Keepers.Akash.Escrow,
			app.Keepers.Akash.Audit,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Bank,
		),

		provider.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Akash.Market,
		),

		audit.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Audit,
		),

		cert.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Cert,
		),

		inflation.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Inflation,
		),

		astaking.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Staking,
		),

		agov.NewAppModule(
			app.appCodec,
			app.Keepers.Akash.Gov,
		),
	}
}

// akashBeginBlockModules returns all end block modules.
func (app *AkashApp) akashBeginBlockModules() []string {
	return []string{
		upgradetypes.ModuleName,
		capabilitytypes.ModuleName,
		banktypes.ModuleName,
		paramstypes.ModuleName,
		deploymenttypes.ModuleName,
		govtypes.ModuleName,
		agov.ModuleName,
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
		taketypes.ModuleName,
		escrowtypes.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		stakingtypes.ModuleName,
		astaking.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		feegrant.ModuleName,
	}
}

// akashEndBlockModules returns all end block modules.
func (app *AkashApp) akashEndBlockModules() []string {
	return []string{
		crisistypes.ModuleName,
		govtypes.ModuleName,
		agov.ModuleName,
		stakingtypes.ModuleName,
		astaking.ModuleName,
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
		taketypes.ModuleName,
		escrowtypes.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		feegrant.ModuleName,
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
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		feegrant.ModuleName,
		cert.ModuleName,
		taketypes.ModuleName,
		escrow.ModuleName,
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
		inflation.ModuleName,
		astaking.ModuleName,
		agov.ModuleName,
		genutiltypes.ModuleName,
	}
}

func (app *AkashApp) akashSimModules() []module.AppModuleSimulation {
	return []module.AppModuleSimulation{
		take.NewAppModuleSimulation(
			app.Keepers.Akash.Take,
		),

		deployment.NewAppModuleSimulation(
			app.Keepers.Akash.Deployment,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
		),

		market.NewAppModuleSimulation(
			app.Keepers.Akash.Market,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Bank,
		),

		provider.NewAppModuleSimulation(
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
		),

		cert.NewAppModuleSimulation(
			app.Keepers.Akash.Cert,
		),

		inflation.NewAppModuleSimulation(
			app.Keepers.Akash.Inflation,
		),

		astaking.NewAppModuleSimulation(
			app.Keepers.Akash.Staking,
		),

		agov.NewAppModuleSimulation(
			app.Keepers.Akash.Gov,
		),
	}
}
