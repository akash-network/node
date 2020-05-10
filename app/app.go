package app

import (
	"io"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradeclient "github.com/cosmos/cosmos-sdk/x/upgrade/client"

	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/staking"

	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"

	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	abci "github.com/tendermint/tendermint/abci/types"
	tmos "github.com/tendermint/tendermint/libs/os"

	"github.com/cosmos/cosmos-sdk/version"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/supply"
)

const (
	appName = "akash"
)

var (
	mbasics = module.NewBasicManager(
		genutil.AppModuleBasic{},

		// accounts, fees.
		auth.AppModuleBasic{},

		// tokens, token balance.
		bank.AppModuleBasic{},

		// total supply of the chain
		supply.AppModuleBasic{},

		// inflation
		mint.AppModuleBasic{},

		staking.AppModuleBasic{},

		slashing.AppModuleBasic{},

		distr.AppModuleBasic{},

		gov.NewAppModuleBasic(
			paramsclient.ProposalHandler, distr.ProposalHandler, upgradeclient.ProposalHandler,
		),

		params.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evidence.AppModuleBasic{},
		crisis.AppModuleBasic{},

		// akash
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
	)
)

// AkashApp extends ABCI appplication
type AkashApp struct {
	*bam.BaseApp
	cdc *codec.Codec

	invCheckPeriod uint

	keys  map[string]*sdk.KVStoreKey
	tkeys map[string]*sdk.TransientStoreKey

	keeper struct {
		acct       auth.AccountKeeper
		bank       bank.Keeper
		params     params.Keeper
		supply     supply.Keeper
		staking    staking.Keeper
		distr      distr.Keeper
		slashing   slashing.Keeper
		mint       mint.Keeper
		gov        gov.Keeper
		upgrade    upgrade.Keeper
		crisis     crisis.Keeper
		evidence   evidence.Keeper
		deployment deployment.Keeper
		market     market.Keeper
		provider   provider.Keeper
	}

	mm *module.Manager

	// simulation manager
	sm *module.SimulationManager
}

// ModuleBasics returns all app modules basics
func ModuleBasics() module.BasicManager {
	return mbasics
}

// MakeCodec returns registered codecs
func MakeCodec() *codec.Codec {
	var cdc = codec.New()

	mbasics.RegisterCodec(cdc)

	sdk.RegisterCodec(cdc)
	vesting.RegisterCodec(cdc)
	codec.RegisterCrypto(cdc)
	codec.RegisterEvidences(cdc)

	return cdc.Seal()
}

// https://github.com/cosmos/sdk-tutorials/blob/c6754a1e313eb1ed973c5c91dcc606f2fd288811/app.go#L73

// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger, db dbm.DB, tio io.Writer, invCheckPeriod uint, skipUpgradeHeights map[int64]bool, options ...func(*bam.BaseApp),
) *AkashApp {

	cdc := MakeCodec()

	keys := sdk.NewKVStoreKeys(
		bam.MainStoreKey,
		auth.StoreKey,
		params.StoreKey,
		slashing.StoreKey,
		distr.StoreKey,
		supply.StoreKey,
		staking.StoreKey,
		mint.StoreKey,
		gov.StoreKey,
		upgrade.StoreKey,
		evidence.StoreKey,
		deployment.StoreKey,
		market.StoreKey,
		provider.StoreKey,
	)

	tkeys := sdk.NewTransientStoreKeys(params.TStoreKey)

	bapp := bam.NewBaseApp(appName, logger, db, auth.DefaultTxDecoder(cdc), options...)
	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetAppVersion(version.Version)

	app := &AkashApp{
		BaseApp:        bapp,
		cdc:            cdc,
		invCheckPeriod: invCheckPeriod,
		keys:           keys,
		tkeys:          tkeys,
	}

	app.keeper.params = params.NewKeeper(
		cdc,
		keys[params.StoreKey],
		tkeys[params.TStoreKey],
	)

	app.keeper.acct = auth.NewAccountKeeper(
		cdc,
		keys[auth.StoreKey],
		app.keeper.params.Subspace(auth.DefaultParamspace),
		auth.ProtoBaseAccount,
	)

	app.keeper.bank = bank.NewBaseKeeper(
		app.keeper.acct,
		app.keeper.params.Subspace(bank.DefaultParamspace),
		macAddrs(),
	)

	app.keeper.supply = supply.NewKeeper(
		cdc,
		keys[supply.StoreKey],
		app.keeper.acct,
		app.keeper.bank,
		macPerms(),
	)

	skeeper := staking.NewKeeper(
		cdc,
		keys[staking.StoreKey],
		app.keeper.supply,
		app.keeper.params.Subspace(staking.DefaultParamspace),
	)

	app.keeper.distr = distr.NewKeeper(
		cdc,
		keys[distr.StoreKey],
		app.keeper.params.Subspace(distr.DefaultParamspace),
		&skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
		macAddrs(),
	)

	app.keeper.slashing = slashing.NewKeeper(
		cdc,
		keys[slashing.StoreKey],
		&skeeper,
		app.keeper.params.Subspace(slashing.DefaultParamspace),
	)

	app.keeper.staking = *skeeper.SetHooks(
		staking.NewMultiStakingHooks(
			app.keeper.distr.Hooks(),
			app.keeper.slashing.Hooks(),
		),
	)

	app.keeper.mint = mint.NewKeeper(
		cdc,
		keys[mint.StoreKey],
		app.keeper.params.Subspace(mint.DefaultParamspace),
		&skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
	)

	app.keeper.upgrade = upgrade.NewKeeper(skipUpgradeHeights, keys[upgrade.StoreKey], cdc)

	app.keeper.crisis = crisis.NewKeeper(
		app.keeper.params.Subspace(crisis.DefaultParamspace),
		invCheckPeriod,
		app.keeper.supply,
		auth.FeeCollectorName,
	)

	// create evidence keeper with evidence router
	evidenceKeeper := evidence.NewKeeper(
		app.cdc, keys[evidence.StoreKey],
		app.keeper.params.Subspace(evidence.DefaultParamspace),
		&app.keeper.staking,
		app.keeper.slashing,
	)
	evidenceRouter := evidence.NewRouter()

	// TODO: register evidence routes
	evidenceKeeper.SetRouter(evidenceRouter)

	app.keeper.evidence = *evidenceKeeper

	// register the proposal types
	govRouter := gov.NewRouter()
	govRouter.AddRoute(gov.RouterKey, gov.ProposalHandler).
		AddRoute(params.RouterKey, params.NewParamChangeProposalHandler(app.keeper.params)).
		AddRoute(distr.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.keeper.distr)).
		AddRoute(upgrade.RouterKey, upgrade.NewSoftwareUpgradeProposalHandler(app.keeper.upgrade))

	app.keeper.gov = gov.NewKeeper(
		cdc,
		keys[gov.StoreKey],
		app.keeper.params.Subspace(gov.DefaultParamspace).WithKeyTable(gov.ParamKeyTable()),
		app.keeper.supply,
		&skeeper,
		govRouter,
	)

	app.keeper.deployment = deployment.NewKeeper(
		cdc,
		keys[deployment.StoreKey],
	)

	app.keeper.market = market.NewKeeper(
		cdc,
		keys[market.StoreKey],
	)

	app.keeper.provider = provider.NewKeeper(
		cdc,
		keys[provider.StoreKey],
	)

	app.mm = module.NewManager(
		genutil.NewAppModule(app.keeper.acct, app.keeper.staking, app.BaseApp.DeliverTx),
		auth.NewAppModule(app.keeper.acct),
		bank.NewAppModule(app.keeper.bank, app.keeper.acct),

		supply.NewAppModule(app.keeper.supply, app.keeper.acct),
		distr.NewAppModule(app.keeper.distr, app.keeper.acct, app.keeper.supply, app.keeper.staking),

		mint.NewAppModule(app.keeper.mint),
		slashing.NewAppModule(app.keeper.slashing, app.keeper.acct, app.keeper.staking),

		staking.NewAppModule(app.keeper.staking, app.keeper.acct, app.keeper.supply),

		gov.NewAppModule(app.keeper.gov, app.keeper.acct, app.keeper.supply),
		upgrade.NewAppModule(app.keeper.upgrade),
		evidence.NewAppModule(app.keeper.evidence),
		crisis.NewAppModule(&app.keeper.crisis),

		// akash
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

		provider.NewAppModule(app.keeper.provider, app.keeper.bank),
	)

	app.mm.SetOrderBeginBlockers(upgrade.ModuleName, mint.ModuleName, distr.ModuleName, slashing.ModuleName, evidence.ModuleName)
	app.mm.SetOrderEndBlockers(staking.ModuleName, gov.ModuleName, crisis.ModuleName, deployment.ModuleName, market.ModuleName)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		distr.ModuleName,
		staking.ModuleName,
		auth.ModuleName,
		bank.ModuleName,
		slashing.ModuleName,
		gov.ModuleName,
		mint.ModuleName,
		supply.ModuleName,
		crisis.ModuleName,
		genutil.ModuleName,
		evidence.ModuleName,

		// akash
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
	)

	app.mm.RegisterInvariants(&app.keeper.crisis)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter())

	app.sm = module.NewSimulationManager(
		auth.NewAppModule(app.keeper.acct),
		bank.NewAppModule(app.keeper.bank, app.keeper.acct),
		supply.NewAppModule(app.keeper.supply, app.keeper.acct),
		mint.NewAppModule(app.keeper.mint),
		staking.NewAppModule(app.keeper.staking, app.keeper.acct, app.keeper.supply),
		distr.NewAppModule(app.keeper.distr, app.keeper.acct, app.keeper.supply, app.keeper.staking),
		slashing.NewAppModule(app.keeper.slashing, app.keeper.acct, app.keeper.staking),
		params.NewAppModule(), // NOTE: only used for simulation to generate randomized param change proposals
		deployment.NewAppModuleSimulation(app.keeper.deployment, app.keeper.acct),
		market.NewAppModuleSimulation(app.keeper.market, app.keeper.acct, app.keeper.deployment,
			app.keeper.provider, app.keeper.bank),
		provider.NewAppModuleSimulation(app.keeper.provider, app.keeper.acct),
	)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)

	app.SetAnteHandler(
		auth.NewAnteHandler(
			app.keeper.acct,
			app.keeper.supply,
			auth.DefaultSigVerificationGasConsumer,
		),
	)

	app.SetEndBlocker(app.EndBlocker)

	err := app.LoadLatestVersion(app.keys[bam.MainStoreKey])
	if err != nil {
		tmos.Exit("app initialization:" + err.Error())
	}

	return app
}

// InitChainer application update at chain initialization
func (app *AkashApp) InitChainer(
	ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	app.cdc.MustUnmarshalJSON(req.AppStateBytes, &genesisState)

	return app.mm.InitGenesis(ctx, genesisState)
}

// BeginBlocker is a function in which application updates every begin block
func (app *AkashApp) BeginBlocker(
	ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	return app.mm.BeginBlock(ctx, req)
}

// EndBlocker is a function in which application updates every end block
func (app *AkashApp) EndBlocker(
	ctx sdk.Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
	return app.mm.EndBlock(ctx, req)
}

// Codec returns SimApp's codec.
func (app *AkashApp) Codec() *codec.Codec {
	return app.cdc
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *AkashApp) ModuleAccountAddrs() map[string]bool {
	return macAddrs()
}

// SimulationManager implements the SimulationApp interface
func (app *AkashApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// LoadHeight method of AkashApp loads baseapp application version with given height
func (app *AkashApp) LoadHeight(height int64) error {
	return app.LoadVersion(height, app.keys[bam.MainStoreKey])
}
