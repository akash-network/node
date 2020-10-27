package app

import (
	"io"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/upgrade"

	"github.com/cosmos/cosmos-sdk/x/params"
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
	csupply "github.com/ovrclk/cosmos-supply-summary/x/supply"
)

const (
	appName = "akash"
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

// https://github.com/cosmos/sdk-tutorials/blob/c6754a1e313eb1ed973c5c91dcc606f2fd288811/app.go#L73
// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger, db dbm.DB, tio io.Writer, invCheckPeriod uint, skipUpgradeHeights map[int64]bool, options ...func(*bam.BaseApp),
) *AkashApp {

	cdc := MakeCodec()

	bapp := bam.NewBaseApp(appName, logger, db, auth.DefaultTxDecoder(cdc), options...)
	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetAppVersion(version.Version)

	keys := kvStoreKeys()
	tkeys := transientStoreKeys()

	app := &AkashApp{
		BaseApp:        bapp,
		cdc:            cdc,
		invCheckPeriod: invCheckPeriod,
		keys:           keys,
		tkeys:          tkeys,
	}

	app.keeper.params = params.NewKeeper(
		app.cdc,
		app.keys[params.StoreKey],
		app.tkeys[params.TStoreKey],
	)

	app.keeper.acct = auth.NewAccountKeeper(
		app.cdc,
		app.keys[auth.StoreKey],
		app.keeper.params.Subspace(auth.DefaultParamspace),
		auth.ProtoBaseAccount,
	)

	app.keeper.bank = bank.NewBaseKeeper(
		app.keeper.acct,
		app.keeper.params.Subspace(bank.DefaultParamspace),
		macAddrs(),
	)

	app.keeper.supply = supply.NewKeeper(
		app.cdc,
		app.keys[supply.StoreKey],
		app.keeper.acct,
		app.keeper.bank,
		macPerms(),
	)

	skeeper := staking.NewKeeper(
		app.cdc,
		app.keys[staking.StoreKey],
		app.keeper.supply,
		app.keeper.params.Subspace(staking.DefaultParamspace),
	)

	app.keeper.distr = distr.NewKeeper(
		app.cdc,
		app.keys[distr.StoreKey],
		app.keeper.params.Subspace(distr.DefaultParamspace),
		&skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
		macAddrs(),
	)

	app.keeper.slashing = slashing.NewKeeper(
		app.cdc,
		app.keys[slashing.StoreKey],
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
		app.cdc,
		app.keys[mint.StoreKey],
		app.keeper.params.Subspace(mint.DefaultParamspace),
		&skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
	)

	app.keeper.upgrade = upgrade.NewKeeper(skipUpgradeHeights, app.keys[upgrade.StoreKey], app.cdc)

	app.keeper.crisis = crisis.NewKeeper(
		app.keeper.params.Subspace(crisis.DefaultParamspace),
		app.invCheckPeriod,
		app.keeper.supply,
		auth.FeeCollectorName,
	)

	// create evidence keeper with evidence router
	evidenceKeeper := evidence.NewKeeper(
		app.cdc, app.keys[evidence.StoreKey],
		app.keeper.params.Subspace(evidence.DefaultParamspace),
		&app.keeper.staking,
		app.keeper.slashing,
	)
	evidenceRouter := evidence.NewRouter()

	evidenceKeeper.SetRouter(evidenceRouter)

	app.keeper.evidence = *evidenceKeeper

	// register the proposal types
	govRouter := gov.NewRouter()
	govRouter.AddRoute(gov.RouterKey, gov.ProposalHandler).
		AddRoute(params.RouterKey, params.NewParamChangeProposalHandler(app.keeper.params)).
		AddRoute(distr.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.keeper.distr)).
		AddRoute(upgrade.RouterKey, upgrade.NewSoftwareUpgradeProposalHandler(app.keeper.upgrade))

	app.keeper.gov = gov.NewKeeper(
		app.cdc,
		app.keys[gov.StoreKey],
		app.keeper.params.Subspace(gov.DefaultParamspace).WithKeyTable(gov.ParamKeyTable()),
		app.keeper.supply,
		&skeeper,
		govRouter,
	)

	app.setAkashKeepers()

	app.mm = module.NewManager(
		append([]module.AppModule{
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
			csupply.NewAppModule(app.cdc, app.keeper.acct, app.keeper.supply),
		},

			app.akashAppModules()...,
		)...,
	)

	app.mm.SetOrderBeginBlockers(upgrade.ModuleName, mint.ModuleName, distr.ModuleName, slashing.ModuleName, evidence.ModuleName)
	app.mm.SetOrderEndBlockers(
		append([]string{
			crisis.ModuleName, gov.ModuleName, staking.ModuleName},
			app.akashEndBlockModules()...,
		)...,
	)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		append([]string{
			auth.ModuleName,
			distr.ModuleName,
			staking.ModuleName,
			bank.ModuleName,
			slashing.ModuleName,
			gov.ModuleName,
			mint.ModuleName,
			supply.ModuleName,
			crisis.ModuleName,
			genutil.ModuleName,
			evidence.ModuleName,
		},

			app.akashInitGenesisOrder()...,
		)...,
	)

	app.mm.RegisterInvariants(&app.keeper.crisis)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter())

	app.sm = module.NewSimulationManager(
		append([]module.AppModuleSimulation{
			auth.NewAppModule(app.keeper.acct),
			bank.NewAppModule(app.keeper.bank, app.keeper.acct),
			supply.NewAppModule(app.keeper.supply, app.keeper.acct),
			mint.NewAppModule(app.keeper.mint),
			staking.NewAppModule(app.keeper.staking, app.keeper.acct, app.keeper.supply),
			distr.NewAppModule(app.keeper.distr, app.keeper.acct, app.keeper.supply, app.keeper.staking),
			slashing.NewAppModule(app.keeper.slashing, app.keeper.acct, app.keeper.staking),
			params.NewAppModule(), // NOTE: only used for simulation to generate randomized param change proposals
		},
			app.akashSimModules()...,
		)...,
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
