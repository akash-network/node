package app

import (
	"encoding/json"
	"io"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"

	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmtypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

const (
	appName = "akash"
)

var (

	// ModuleBasics defines the module BasicManager is in charge of setting up basic,
	// non-dependant module elements, such as codec registration
	// and genesis verification.
	ModuleBasics = module.NewBasicManager(
		auth.AppModuleBasic{},
		supply.AppModuleBasic{},
		genutil.AppModuleBasic{},
		bank.AppModuleBasic{},
		staking.AppModuleBasic{},
		mint.AppModuleBasic{},
		distr.AppModuleBasic{},
		gov.NewAppModuleBasic(
			paramsclient.ProposalHandler, distr.ProposalHandler,
		),
		params.AppModuleBasic{},
		crisis.AppModuleBasic{},
		slashing.AppModuleBasic{},
		evidence.AppModuleBasic{},

		// akash
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
	)

	// module accounts that are allowed to receive tokens
	allowedReceivingModAcc = map[string]bool{
		distr.ModuleName: true,
	}
)

// MaccPerms returns the module account permissions
func MaccPerms() map[string][]string {
	return map[string][]string{
		auth.FeeCollectorName:     nil,
		distr.ModuleName:          nil,
		mint.ModuleName:           {supply.Minter},
		staking.BondedPoolName:    {supply.Burner, supply.Staking},
		staking.NotBondedPoolName: {supply.Burner, supply.Staking},
		gov.ModuleName:            {supply.Burner},
	}
}

var _ simapp.App = (*App)(nil)

// App extends ABCI appplication (i.e BaseApp)
type App struct {
	*bam.BaseApp
	cdc *codec.Codec

	invCheckPeriod uint

	// keys to access the substores
	keys  map[string]*sdk.KVStoreKey
	tkeys map[string]*sdk.TransientStoreKey

	// subspaces
	subspaces map[string]params.Subspace

	// keepers
	Keepers struct {
		Account    auth.AccountKeeper
		Bank       bank.Keeper
		Supply     supply.Keeper
		Staking    staking.Keeper
		Slashing   slashing.Keeper
		Mint       mint.Keeper
		Distr      distr.Keeper
		Gov        gov.Keeper
		Crisis     crisis.Keeper
		Params     params.Keeper
		Evidence   evidence.Keeper
		Deployment deployment.Keeper
		Market     market.Keeper
		Provider   provider.Keeper
	}

	// the module manager
	mm *module.Manager
}

// MakeCodec returns registered codecs
func MakeCodec() *codec.Codec {
	var cdc = codec.New()
	ModuleBasics.RegisterCodec(cdc)
	vesting.RegisterCodec(cdc)
	sdk.RegisterCodec(cdc)
	codec.RegisterCrypto(cdc)
	return cdc.Seal()
}

// https://github.com/cosmos/sdk-tutorials/blob/c6754a1e313eb1ed973c5c91dcc606f2fd288811/app.go#L73

// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger, db dbm.DB, traceStore io.Writer, loadLatest bool,
	invCheckPeriod uint, baseAppOptions ...func(*bam.BaseApp),
) *App {
	cdc := MakeCodec()

	bApp := bam.NewBaseApp(appName, logger, db, auth.DefaultTxDecoder(cdc), baseAppOptions...)
	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetAppVersion(version.Version)

	keys := sdk.NewKVStoreKeys(
		bam.MainStoreKey, auth.StoreKey, staking.StoreKey,
		supply.StoreKey, mint.StoreKey, distr.StoreKey, slashing.StoreKey,
		gov.StoreKey, params.StoreKey, evidence.StoreKey,
		deployment.StoreKey, market.StoreKey, provider.StoreKey,
	)

	tkeys := sdk.NewTransientStoreKeys(params.TStoreKey)

	app := &App{
		BaseApp:        bApp,
		cdc:            cdc,
		invCheckPeriod: invCheckPeriod,
		keys:           keys,
		tkeys:          tkeys,
		subspaces:      make(map[string]params.Subspace),
	}

	// init params keeper and subspaces
	app.Keepers.Params = params.NewKeeper(app.cdc, keys[params.StoreKey], tkeys[params.TStoreKey])
	app.subspaces[auth.ModuleName] = app.Keepers.Params.Subspace(auth.DefaultParamspace)
	app.subspaces[bank.ModuleName] = app.Keepers.Params.Subspace(bank.DefaultParamspace)
	app.subspaces[staking.ModuleName] = app.Keepers.Params.Subspace(staking.DefaultParamspace)
	app.subspaces[mint.ModuleName] = app.Keepers.Params.Subspace(mint.DefaultParamspace)
	app.subspaces[distr.ModuleName] = app.Keepers.Params.Subspace(distr.DefaultParamspace)
	app.subspaces[slashing.ModuleName] = app.Keepers.Params.Subspace(slashing.DefaultParamspace)
	app.subspaces[gov.ModuleName] = app.Keepers.Params.Subspace(gov.DefaultParamspace).WithKeyTable(gov.ParamKeyTable())
	app.subspaces[crisis.ModuleName] = app.Keepers.Params.Subspace(crisis.DefaultParamspace)
	app.subspaces[evidence.ModuleName] = app.Keepers.Params.Subspace(evidence.DefaultParamspace)

	// add keepers
	app.Keepers.Account = auth.NewAccountKeeper(
		app.cdc, keys[auth.StoreKey], app.subspaces[auth.ModuleName], auth.ProtoBaseAccount,
	)
	app.Keepers.Bank = bank.NewBaseKeeper(
		app.Keepers.Account, app.subspaces[bank.ModuleName], app.BlacklistedAccAddrs(),
	)
	app.Keepers.Supply = supply.NewKeeper(
		app.cdc, keys[supply.StoreKey], app.Keepers.Account, app.Keepers.Bank, MaccPerms(),
	)
	stakingKeeper := staking.NewKeeper(
		app.cdc, keys[staking.StoreKey], app.Keepers.Supply, app.subspaces[staking.ModuleName],
	)
	app.Keepers.Mint = mint.NewKeeper(
		app.cdc, keys[mint.StoreKey], app.subspaces[mint.ModuleName], &stakingKeeper,
		app.Keepers.Supply, auth.FeeCollectorName,
	)
	app.Keepers.Distr = distr.NewKeeper(
		app.cdc, keys[distr.StoreKey], app.subspaces[distr.ModuleName], &stakingKeeper,
		app.Keepers.Supply, auth.FeeCollectorName, app.ModuleAccountAddrs(),
	)
	app.Keepers.Slashing = slashing.NewKeeper(
		app.cdc, keys[slashing.StoreKey], &stakingKeeper, app.subspaces[slashing.ModuleName],
	)
	app.Keepers.Crisis = crisis.NewKeeper(
		app.subspaces[crisis.ModuleName], invCheckPeriod, app.Keepers.Supply, auth.FeeCollectorName,
	)

	// create evidence keeper with router
	evidenceKeeper := evidence.NewKeeper(
		app.cdc, keys[evidence.StoreKey], app.subspaces[evidence.ModuleName], &app.Keepers.Staking, app.Keepers.Slashing,
	)
	evidenceRouter := evidence.NewRouter()
	// TODO: Register evidence routes.
	evidenceKeeper.SetRouter(evidenceRouter)
	app.Keepers.Evidence = *evidenceKeeper

	// register the proposal types
	govRouter := gov.NewRouter()
	govRouter.AddRoute(gov.RouterKey, gov.ProposalHandler).
		AddRoute(params.RouterKey, params.NewParamChangeProposalHandler(app.Keepers.Params)).
		AddRoute(distr.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.Keepers.Distr))

	app.Keepers.Gov = gov.NewKeeper(
		app.cdc, keys[gov.StoreKey], app.subspaces[gov.ModuleName], app.Keepers.Supply,
		&stakingKeeper, govRouter,
	)

	// register the staking hooks
	// NOTE: stakingKeeper above is passed by reference, so that it will contain these hooks
	app.Keepers.Staking = *stakingKeeper.SetHooks(
		staking.NewMultiStakingHooks(app.Keepers.Distr.Hooks(), app.Keepers.Slashing.Hooks()),
	)

	app.Keepers.Deployment = deployment.NewKeeper(cdc, keys[deployment.StoreKey])
	app.Keepers.Market = market.NewKeeper(cdc, keys[market.StoreKey])
	app.Keepers.Provider = provider.NewKeeper(cdc, keys[provider.StoreKey])

	app.mm = module.NewManager(
		genutil.NewAppModule(app.Keepers.Account, app.Keepers.Staking, app.BaseApp.DeliverTx),
		auth.NewAppModule(app.Keepers.Account),
		bank.NewAppModule(app.Keepers.Bank, app.Keepers.Account),
		crisis.NewAppModule(&app.Keepers.Crisis),
		supply.NewAppModule(app.Keepers.Supply, app.Keepers.Account),
		gov.NewAppModule(app.Keepers.Gov, app.Keepers.Account, app.Keepers.Supply),
		mint.NewAppModule(app.Keepers.Mint),
		slashing.NewAppModule(app.Keepers.Slashing, app.Keepers.Account, app.Keepers.Staking),
		distr.NewAppModule(app.Keepers.Distr, app.Keepers.Account, app.Keepers.Supply, app.Keepers.Staking),
		staking.NewAppModule(app.Keepers.Staking, app.Keepers.Account, app.Keepers.Supply),
		evidence.NewAppModule(app.Keepers.Evidence),
		deployment.NewAppModule(app.Keepers.Deployment, app.Keepers.Market, app.Keepers.Bank),
		market.NewAppModule(app.Keepers.Market, app.Keepers.Deployment, app.Keepers.Provider, app.Keepers.Bank),
		provider.NewAppModule(app.Keepers.Provider, app.Keepers.Bank),
	)

	app.mm.SetOrderBeginBlockers(mint.ModuleName, distr.ModuleName, slashing.ModuleName)
	app.mm.SetOrderEndBlockers(staking.ModuleName, deployment.ModuleName, market.ModuleName)

	// NOTE: The genutils moodule must occur after staking so that pools are
	// properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
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
		deployment.ModuleName, // akash
		provider.ModuleName,
		market.ModuleName,
	)

	app.mm.RegisterRoutes(app.Router(), app.QueryRouter())

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetAnteHandler(ante.NewAnteHandler(app.Keepers.Account, app.Keepers.Supply, auth.DefaultSigVerificationGasConsumer))
	app.SetEndBlocker(app.EndBlocker)

	err := app.LoadLatestVersion(app.keys[bam.MainStoreKey])
	if err != nil {
		tmos.Exit("app initialization:" + err.Error())
	}

	return app
}

// Name returns the name of the App
func (app *App) Name() string { return app.BaseApp.Name() }

// BeginBlocker application updates every begin block
func (app *App) BeginBlocker(ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	return app.mm.BeginBlock(ctx, req)
}

// EndBlocker application updates every end block
func (app *App) EndBlocker(ctx sdk.Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
	return app.mm.EndBlock(ctx, req)
}

// InitChainer application update at chain initialization
func (app *App) InitChainer(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	app.cdc.MustUnmarshalJSON(req.AppStateBytes, &genesisState)
	return app.mm.InitGenesis(ctx, genesisState)
}

// LoadHeight method of App loads baseapp application version with given height
func (app *App) LoadHeight(height int64) error {
	return app.LoadVersion(height, app.keys[bam.MainStoreKey])
}

// ExportAppStateAndValidators returns application state json and slice of validators
func (app *App) ExportAppStateAndValidators(
	forZeroHeight bool, jailWhiteList []string,
) (appState json.RawMessage, validators []tmtypes.GenesisValidator, err error) {

	// as if they could withdraw from the start of the next block
	ctx := app.NewContext(true, abci.Header{Height: app.LastBlockHeight()})

	genState := app.mm.ExportGenesis(ctx)
	appState, err = codec.MarshalJSONIndent(app.cdc, genState)
	if err != nil {
		return nil, nil, err
	}

	validators = staking.WriteValidators(ctx, app.Keepers.Staking)

	return appState, validators, nil
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *App) ModuleAccountAddrs() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range MaccPerms() {
		modAccAddrs[supply.NewModuleAddress(acc).String()] = true
	}

	return modAccAddrs
}

// BlacklistedAccAddrs returns all the app's module account addresses black listed for receiving tokens.
func (app *App) BlacklistedAccAddrs() map[string]bool {
	blacklistedAddrs := make(map[string]bool)
	for acc := range MaccPerms() {
		blacklistedAddrs[supply.NewModuleAddress(acc).String()] = !allowedReceivingModAcc[acc]
	}

	return blacklistedAddrs
}

// Codec returns App's codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *App) Codec() *codec.Codec {
	return app.cdc
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *App) GetKey(storeKey string) *sdk.KVStoreKey {
	return app.keys[storeKey]
}

// GetTKey returns the TransientStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *App) GetTKey(storeKey string) *sdk.TransientStoreKey {
	return app.tkeys[storeKey]
}

// GetSubspace returns a param subspace for a given module name.
//
// NOTE: This is solely to be used for testing purposes.
func (app *App) GetSubspace(moduleName string) params.Subspace {
	return app.subspaces[moduleName]
}

// SimulationManager implements the SimulationApp interface
func (app *App) SimulationManager() *module.SimulationManager {
	// TODO: implement in a later PR
	return nil
}
