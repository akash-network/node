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

	transfertypes "github.com/cosmos/ibc-go/v3/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v3/modules/core/24-host"

	audittypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
	certtypes "github.com/akash-network/akash-api/go/node/cert/v1beta3"
	deploymenttypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	escrowtypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
	agovtypes "github.com/akash-network/akash-api/go/node/gov/v1beta3"
	inflationtypes "github.com/akash-network/akash-api/go/node/inflation/v1beta3"
	markettypes "github.com/akash-network/akash-api/go/node/market/v1beta3"
	providertypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"
	astakingtypes "github.com/akash-network/akash-api/go/node/staking/v1beta3"

	"github.com/akash-network/node/x/audit"
	"github.com/akash-network/node/x/cert"
	"github.com/akash-network/node/x/deployment"
	"github.com/akash-network/node/x/escrow"
	ekeeper "github.com/akash-network/node/x/escrow/keeper"
	agov "github.com/akash-network/node/x/gov"
	agovkeeper "github.com/akash-network/node/x/gov/keeper"
	"github.com/akash-network/node/x/inflation"
	"github.com/akash-network/node/x/market"
	mhooks "github.com/akash-network/node/x/market/hooks"
	"github.com/akash-network/node/x/provider"
	astaking "github.com/akash-network/node/x/staking"
	astakingkeeper "github.com/akash-network/node/x/staking/keeper"
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
		agov.AppModuleBasic{},
		astaking.AppModuleBasic{},
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
		agov.StoreKey,
		astaking.StoreKey,
	}
}

func akashSubspaces(k paramskeeper.Keeper) paramskeeper.Keeper {
	k.Subspace(deployment.ModuleName)
	k.Subspace(market.ModuleName)
	k.Subspace(inflation.ModuleName)
	k.Subspace(agov.ModuleName)
	k.Subspace(astaking.ModuleName)

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

	app.keeper.inflation = inflation.NewKeeper(
		app.appCodec,
		app.keys[inflation.StoreKey],
		app.GetSubspace(inflation.ModuleName),
	)

	app.keeper.agov = agovkeeper.NewKeeper(
		app.appCodec,
		app.keys[agov.StoreKey],
		app.GetSubspace(agov.ModuleName),
	)

	app.keeper.astaking = astakingkeeper.NewKeeper(
		app.appCodec,
		app.keys[astaking.StoreKey],
		app.GetSubspace(astaking.ModuleName),
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
			app.keeper.authz,
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

		inflation.NewAppModule(
			app.appCodec,
			app.keeper.inflation,
		),

		astaking.NewAppModule(
			app.appCodec,
			app.keeper.astaking,
		),

		agov.NewAppModule(
			app.appCodec,
			app.keeper.agov,
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
		agovtypes.ModuleName,
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
		astakingtypes.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
	}
}

// akashEndBlockModules returns all end block modules.
func (app *AkashApp) akashEndBlockModules() []string {
	return []string{
		crisistypes.ModuleName,
		govtypes.ModuleName,
		agovtypes.ModuleName,
		stakingtypes.ModuleName,
		astakingtypes.ModuleName,
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
		cert.ModuleName,
		escrow.ModuleName,
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
		inflation.ModuleName,
		agov.ModuleName,
		astaking.ModuleName,
		genutiltypes.ModuleName,
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

		inflation.NewAppModuleSimulation(
			app.keeper.inflation,
		),

		agov.NewAppModuleSimulation(
			app.keeper.agov,
		),

		astaking.NewAppModuleSimulation(
			app.keeper.astaking,
		),
	}
}
