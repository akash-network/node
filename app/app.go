package app

import (
	"io"
	"net/http"
	"os"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authrest "github.com/cosmos/cosmos-sdk/x/auth/client/rest"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	evidencekeeper "github.com/cosmos/cosmos-sdk/x/evidence/keeper"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradekeeper "github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"

	"github.com/cosmos/cosmos-sdk/x/params"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	paramproposal "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"

	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	abci "github.com/tendermint/tendermint/abci/types"
	tmos "github.com/tendermint/tendermint/libs/os"

	"github.com/cosmos/cosmos-sdk/version"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"

	// unnamed import of statik for swagger UI support
	_ "github.com/ovrclk/akash/client/docs/statik"
)

const (
	AppName = "akash"
)

var DefaultHome = os.ExpandEnv("$HOME/.akash")

// AkashApp extends ABCI appplication
type AkashApp struct {
	*bam.BaseApp
	cdc               *codec.LegacyAmino
	appCodec          codec.Marshaler
	interfaceRegistry codectypes.InterfaceRegistry

	invCheckPeriod uint

	keys  map[string]*sdk.KVStoreKey
	tkeys map[string]*sdk.TransientStoreKey

	keeper struct {
		acct       authkeeper.AccountKeeper
		bank       bankkeeper.Keeper
		params     paramskeeper.Keeper
		staking    stakingkeeper.Keeper
		distr      distrkeeper.Keeper
		slashing   slashingkeeper.Keeper
		mint       mintkeeper.Keeper
		gov        govkeeper.Keeper
		upgrade    upgradekeeper.Keeper
		crisis     crisiskeeper.Keeper
		evidence   evidencekeeper.Keeper
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
	logger log.Logger, db dbm.DB, tio io.Writer, invCheckPeriod uint, skipUpgradeHeights map[int64]bool,
	homePath string, options ...func(*bam.BaseApp),
) *AkashApp {

	// TODO: Remove cdc in favor of appCodec once all modules are migrated.
	encodingConfig := MakeEncodingConfig()
	appCodec := encodingConfig.Marshaler
	cdc := encodingConfig.Amino
	interfaceRegistry := encodingConfig.InterfaceRegistry

	bapp := bam.NewBaseApp(AppName, logger, db, encodingConfig.TxConfig.TxDecoder(), options...)
	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetAppVersion(version.Version)
	bapp.GRPCQueryRouter().SetInterfaceRegistry(interfaceRegistry)
	bapp.GRPCQueryRouter().RegisterSimulateService(bapp.Simulate, interfaceRegistry)

	keys := kvStoreKeys()
	tkeys := transientStoreKeys()

	app := &AkashApp{
		BaseApp:           bapp,
		cdc:               cdc,
		appCodec:          appCodec,
		interfaceRegistry: interfaceRegistry,
		invCheckPeriod:    invCheckPeriod,
		keys:              keys,
		tkeys:             tkeys,
	}

	app.keeper.params = initParamsKeeper(appCodec, cdc, app.keys[paramstypes.StoreKey], tkeys[paramstypes.TStoreKey])

	// set the BaseApp's parameter store
	bapp.SetParamStore(app.keeper.params.Subspace(bam.Paramspace).WithKeyTable(paramskeeper.ConsensusParamsKeyTable()))

	app.keeper.acct = authkeeper.NewAccountKeeper(
		appCodec,
		app.keys[authtypes.StoreKey],
		app.GetSubspace(authtypes.ModuleName),
		authtypes.ProtoBaseAccount,
		MacPerms(),
	)

	app.keeper.bank = bankkeeper.NewBaseKeeper(
		appCodec,
		app.keys[banktypes.StoreKey],
		app.keeper.acct,
		app.GetSubspace(banktypes.ModuleName),
		app.ModuleAccountAddrs(),
	)

	skeeper := stakingkeeper.NewKeeper(
		appCodec,
		app.keys[stakingtypes.StoreKey],
		app.keeper.acct,
		app.keeper.bank,
		app.GetSubspace(stakingtypes.ModuleName),
	)

	app.keeper.distr = distrkeeper.NewKeeper(
		appCodec,
		app.keys[distrtypes.StoreKey],
		app.GetSubspace(distrtypes.ModuleName),
		app.keeper.acct,
		app.keeper.bank,
		&skeeper,
		authtypes.FeeCollectorName,
		app.ModuleAccountAddrs(),
	)

	app.keeper.slashing = slashingkeeper.NewKeeper(
		appCodec,
		app.keys[slashingtypes.StoreKey],
		&skeeper,
		app.GetSubspace(slashingtypes.ModuleName),
	)

	// register the staking hooks
	// NOTE: stakingKeeper above is passed by reference, so that it will contain these hooks
	app.keeper.staking = *skeeper.SetHooks(
		stakingtypes.NewMultiStakingHooks(
			app.keeper.distr.Hooks(),
			app.keeper.slashing.Hooks(),
		),
	)

	app.keeper.mint = mintkeeper.NewKeeper(
		appCodec,
		app.keys[minttypes.StoreKey],
		app.GetSubspace(minttypes.ModuleName),
		&skeeper,
		app.keeper.acct,
		app.keeper.bank,
		authtypes.FeeCollectorName,
	)

	app.keeper.upgrade = upgradekeeper.NewKeeper(skipUpgradeHeights, app.keys[upgradetypes.StoreKey], appCodec, homePath)

	app.keeper.crisis = crisiskeeper.NewKeeper(
		app.GetSubspace(crisistypes.ModuleName),
		invCheckPeriod,
		app.keeper.bank,
		authtypes.FeeCollectorName,
	)

	// create evidence keeper with evidence router
	evidenceKeeper := evidencekeeper.NewKeeper(
		appCodec, app.keys[evidencetypes.StoreKey], &app.keeper.staking, app.keeper.slashing,
	)
	evidenceRouter := evidencetypes.NewRouter()

	evidenceKeeper.SetRouter(evidenceRouter)

	app.keeper.evidence = *evidenceKeeper

	// register the proposal types
	govRouter := govtypes.NewRouter()
	govRouter.AddRoute(govtypes.RouterKey, govtypes.ProposalHandler).
		AddRoute(paramproposal.RouterKey, params.NewParamChangeProposalHandler(app.keeper.params)).
		AddRoute(distrtypes.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.keeper.distr)).
		AddRoute(upgradetypes.RouterKey, upgrade.NewSoftwareUpgradeProposalHandler(app.keeper.upgrade))

	app.keeper.gov = govkeeper.NewKeeper(
		appCodec,
		app.keys[govtypes.StoreKey],
		app.GetSubspace(govtypes.ModuleName),
		app.keeper.acct,
		app.keeper.bank,
		&skeeper,
		govRouter,
	)

	app.setAkashKeepers()

	app.mm = module.NewManager(
		append([]module.AppModule{
			genutil.NewAppModule(app.keeper.acct, app.keeper.staking, app.BaseApp.DeliverTx, encodingConfig.TxConfig),
			auth.NewAppModule(appCodec, app.keeper.acct, authsims.RandomGenesisAccounts),
			bank.NewAppModule(appCodec, app.keeper.bank, app.keeper.acct),
			crisis.NewAppModule(&app.keeper.crisis),
			gov.NewAppModule(appCodec, app.keeper.gov, app.keeper.acct, app.keeper.bank),
			mint.NewAppModule(appCodec, app.keeper.mint, app.keeper.acct),
			slashing.NewAppModule(appCodec, app.keeper.slashing, app.keeper.acct, app.keeper.bank, app.keeper.staking),
			distr.NewAppModule(appCodec, app.keeper.distr, app.keeper.acct, app.keeper.bank, app.keeper.staking),
			staking.NewAppModule(appCodec, app.keeper.staking, app.keeper.acct, app.keeper.bank),
			upgrade.NewAppModule(app.keeper.upgrade),
			evidence.NewAppModule(app.keeper.evidence),
		},

			app.akashAppModules()...,
		)...,
	)

	app.mm.SetOrderBeginBlockers(
		upgradetypes.ModuleName, minttypes.ModuleName, distrtypes.ModuleName, slashingtypes.ModuleName,
		evidencetypes.ModuleName, stakingtypes.ModuleName,
	)
	app.mm.SetOrderEndBlockers(
		append([]string{
			crisistypes.ModuleName, govtypes.ModuleName, stakingtypes.ModuleName},
			app.akashEndBlockModules()...,
		)...,
	)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		append([]string{
			authtypes.ModuleName,
			banktypes.ModuleName,
			distrtypes.ModuleName,
			stakingtypes.ModuleName,
			slashingtypes.ModuleName,
			govtypes.ModuleName,
			minttypes.ModuleName,
			crisistypes.ModuleName,
			genutiltypes.ModuleName,
			evidencetypes.ModuleName,
		},

			app.akashInitGenesisOrder()...,
		)...,
	)

	app.mm.RegisterInvariants(&app.keeper.crisis)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter(), encodingConfig.Amino)
	app.mm.RegisterQueryServices(app.GRPCQueryRouter())

	app.sm = module.NewSimulationManager(
		append([]module.AppModuleSimulation{
			auth.NewAppModule(appCodec, app.keeper.acct, authsims.RandomGenesisAccounts),
			bank.NewAppModule(appCodec, app.keeper.bank, app.keeper.acct),
			gov.NewAppModule(appCodec, app.keeper.gov, app.keeper.acct, app.keeper.bank),
			mint.NewAppModule(appCodec, app.keeper.mint, app.keeper.acct),
			staking.NewAppModule(appCodec, app.keeper.staking, app.keeper.acct, app.keeper.bank),
			distr.NewAppModule(appCodec, app.keeper.distr, app.keeper.acct, app.keeper.bank, app.keeper.staking),
			slashing.NewAppModule(appCodec, app.keeper.slashing, app.keeper.acct, app.keeper.bank, app.keeper.staking),
			params.NewAppModule(app.keeper.params),
			evidence.NewAppModule(app.keeper.evidence),
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
		ante.NewAnteHandler(
			app.keeper.acct,
			app.keeper.bank,
			ante.DefaultSigVerificationGasConsumer,
			encodingConfig.TxConfig.SignModeHandler(),
		),
	)

	app.SetEndBlocker(app.EndBlocker)

	err := app.LoadLatestVersion()
	if err != nil {
		tmos.Exit("app initialization:" + err.Error())
	}

	return app
}

// MakeCodecs constructs the *std.Codec and *codec.LegacyAmino instances used by
// simapp. It is useful for tests and clients who do not want to construct the
// full simapp
func MakeCodecs() (codec.Marshaler, *codec.LegacyAmino) {
	config := MakeEncodingConfig()
	return config.Marshaler, config.Amino
}

// Name returns the name of the App
func (app *AkashApp) Name() string { return app.BaseApp.Name() }

// InitChainer application update at chain initialization
func (app *AkashApp) InitChainer(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	app.cdc.MustUnmarshalJSON(req.AppStateBytes, &genesisState)
	return app.mm.InitGenesis(ctx, app.appCodec, genesisState)
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

// LegacyAmino returns AkashApp's amino codec.
func (app *AkashApp) LegacyAmino() *codec.LegacyAmino {
	return app.cdc
}

// AppCodec returns AkashApp's app codec.
func (app *AkashApp) AppCodec() codec.Marshaler {
	return app.appCodec
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *AkashApp) ModuleAccountAddrs() map[string]bool {
	return MacAddrs()
}

// InterfaceRegistry returns AkashApp's InterfaceRegistry
func (app *AkashApp) InterfaceRegistry() codectypes.InterfaceRegistry {
	return app.interfaceRegistry
}

// GetKey returns the KVStoreKey for the provided store key.
func (app *AkashApp) GetKey(storeKey string) *sdk.KVStoreKey {
	return app.keys[storeKey]
}

// GetTKey returns the TransientStoreKey for the provided store key.
func (app *AkashApp) GetTKey(storeKey string) *sdk.TransientStoreKey {
	return app.tkeys[storeKey]
}

// GetSubspace returns a param subspace for a given module name.
func (app *AkashApp) GetSubspace(moduleName string) paramstypes.Subspace {
	subspace, _ := app.keeper.params.GetSubspace(moduleName)
	return subspace
}

// SimulationManager implements the SimulationApp interface
func (app *AkashApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *AkashApp) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx
	rpc.RegisterRoutes(clientCtx, apiSvr.Router)
	authrest.RegisterTxRoutes(clientCtx, apiSvr.Router)

	ModuleBasics().RegisterRESTRoutes(clientCtx, apiSvr.Router)
	ModuleBasics().RegisterGRPCRoutes(apiSvr.ClientCtx, apiSvr.GRPCRouter)

	// register swagger API from root so that other applications can override easily
	if apiConfig.Swagger {
		RegisterSwaggerAPI(clientCtx, apiSvr.Router)
	}
}

// RegisterSwaggerAPI registers swagger route with API Server
func RegisterSwaggerAPI(ctx client.Context, rtr *mux.Router) {
	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}

	staticServer := http.FileServer(statikFS)
	rtr.PathPrefix("/swagger/").Handler(http.StripPrefix("/swagger/", staticServer))
}

// LoadHeight method of AkashApp loads baseapp application version with given height
func (app *AkashApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// initParamsKeeper init params keeper and its subspaces
func initParamsKeeper(appCodec codec.BinaryMarshaler, legacyAmino *codec.LegacyAmino, key, tkey sdk.StoreKey) paramskeeper.Keeper {
	paramsKeeper := paramskeeper.NewKeeper(appCodec, legacyAmino, key, tkey)

	paramsKeeper.Subspace(authtypes.ModuleName)
	paramsKeeper.Subspace(banktypes.ModuleName)
	paramsKeeper.Subspace(stakingtypes.ModuleName)
	paramsKeeper.Subspace(minttypes.ModuleName)
	paramsKeeper.Subspace(distrtypes.ModuleName)
	paramsKeeper.Subspace(slashingtypes.ModuleName)
	paramsKeeper.Subspace(govtypes.ModuleName).WithKeyTable(govtypes.ParamKeyTable())
	paramsKeeper.Subspace(crisistypes.ModuleName)

	return paramsKeeper
}
