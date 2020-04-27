package app

import (
	"encoding/json"
	"io"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/capability"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"

	"github.com/cosmos/cosmos-sdk/x/ibc"
	ibcclient "github.com/cosmos/cosmos-sdk/x/ibc/02-client"
	port "github.com/cosmos/cosmos-sdk/x/ibc/05-port"
	transfer "github.com/cosmos/cosmos-sdk/x/ibc/20-transfer"

	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradeclient "github.com/cosmos/cosmos-sdk/x/upgrade/client"

	"github.com/cosmos/cosmos-sdk/x/crisis"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	paramproposal "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	"github.com/cosmos/cosmos-sdk/x/staking"

	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	abci "github.com/tendermint/tendermint/abci/types"
	tmos "github.com/tendermint/tendermint/libs/os"

	codecstd "github.com/cosmos/cosmos-sdk/codec/std"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/version"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	"github.com/cosmos/cosmos-sdk/x/slashing"
)

const (
	appName = "akash"
)

var (
	// mbasics defines the module BasicManager is in charge of setting up basic,
	// non-dependant module elements, such as codec registration
	// and genesis verification.
	mbasics = module.NewBasicManager(
		genutil.AppModuleBasic{},

		// accounts, fees.
		auth.AppModuleBasic{},

		// tokens, token balance.
		bank.AppModuleBasic{},

		capability.AppModuleBasic{},

		// inflation
		mint.AppModuleBasic{},

		staking.AppModuleBasic{},
		slashing.AppModuleBasic{},
		distr.AppModuleBasic{},
		gov.NewAppModuleBasic(
			paramsclient.ProposalHandler, distr.ProposalHandler, upgradeclient.ProposalHandler,
		),
		params.AppModuleBasic{},
		ibc.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evidence.AppModuleBasic{},
		transfer.AppModuleBasic{},
		crisis.AppModuleBasic{},

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

// AkashApp extends ABCI appplication
type AkashApp struct {
	*bam.BaseApp
	cdc *codec.Codec

	invCheckPeriod uint

	keys    map[string]*sdk.KVStoreKey
	tkeys   map[string]*sdk.TransientStoreKey
	memKeys map[string]*sdk.MemoryStoreKey

	keeper struct {
		acct       auth.AccountKeeper
		bank       bank.Keeper
		capability *capability.Keeper
		params     params.Keeper
		staking    staking.Keeper
		distr      distr.Keeper
		slashing   slashing.Keeper
		mint       mint.Keeper
		gov        gov.Keeper
		evidence   evidence.Keeper
		ibc        *ibc.Keeper // IBC Keeper must be a pointer in the app, so we can SetRouter on it correctly
		transfer   transfer.Keeper
		upgrade    upgrade.Keeper
		crisis     crisis.Keeper

		// make scoped keepers public for test purposes
		scopedIBC      capability.ScopedKeeper
		scopedTransfer capability.ScopedKeeper

		deployment deployment.Keeper
		market     market.Keeper
		provider   provider.Keeper
	}

	mm *module.Manager

	// simulation manager
	sm *module.SimulationManager
}

// ModuleBasics returns all app modules b
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
	logger log.Logger, db dbm.DB, tio io.Writer, invCheckPeriod uint,
	skipUpgradeHeights map[int64]bool, home string, options ...func(*bam.BaseApp),
) *AkashApp {

	cdc := MakeCodec()

	appCodec := codecstd.NewAppCodec(cdc)

	keys := sdk.NewKVStoreKeys(
		auth.StoreKey,
		bank.StoreKey,
		params.StoreKey,
		slashing.StoreKey,
		distr.StoreKey,
		evidence.StoreKey,
		staking.StoreKey,
		mint.StoreKey,
		ibc.StoreKey,
		transfer.StoreKey,
		capability.StoreKey,
		gov.StoreKey,
		upgrade.StoreKey,
		deployment.StoreKey,
		market.StoreKey,
		provider.StoreKey,
	)

	tkeys := sdk.NewTransientStoreKeys(params.TStoreKey)
	memKeys := sdk.NewMemoryStoreKeys(capability.MemStoreKey)

	bapp := bam.NewBaseApp(appName, logger, db, auth.DefaultTxDecoder(cdc), options...)
	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetAppVersion(version.Version)

	app := &AkashApp{
		BaseApp:        bapp,
		cdc:            cdc,
		invCheckPeriod: invCheckPeriod,
		keys:           keys,
		tkeys:          tkeys,
		memKeys:        memKeys,
	}

	app.keeper.params = params.NewKeeper(
		appCodec,
		keys[params.StoreKey],
		tkeys[params.TStoreKey],
	)

	bapp.SetParamStore(app.keeper.params.Subspace(bam.Paramspace).WithKeyTable(std.ConsensusParamsKeyTable()))

	app.keeper.capability = capability.NewKeeper(appCodec, keys[capability.StoreKey], memKeys[capability.MemStoreKey])

	scopedIBC := app.keeper.capability.ScopeToModule(ibc.ModuleName)
	scopedTransfer := app.keeper.capability.ScopeToModule(transfer.ModuleName)

	app.keeper.acct = auth.NewAccountKeeper(
		appCodec,
		keys[auth.StoreKey],
		app.keeper.params.Subspace(auth.DefaultParamspace),
		auth.ProtoBaseAccount,
		macPerms(),
	)

	app.keeper.bank = bank.NewBaseKeeper(
		appCodec,
		keys[bank.StoreKey],
		app.keeper.acct,
		app.keeper.params.Subspace(bank.DefaultParamspace),
		app.BlacklistedAccAddrs(),
	)

	skeeper := staking.NewKeeper(
		appCodec,
		keys[staking.StoreKey],
		app.keeper.acct,
		app.keeper.bank,
		app.keeper.params.Subspace(staking.DefaultParamspace),
	)

	app.keeper.distr = distr.NewKeeper(
		appCodec,
		keys[distr.StoreKey],
		app.keeper.params.Subspace(distr.DefaultParamspace),
		app.keeper.acct,
		app.keeper.bank,
		&skeeper,
		auth.FeeCollectorName,
		macAddrs(),
	)

	app.keeper.slashing = slashing.NewKeeper(
		appCodec,
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
		appCodec,
		keys[mint.StoreKey],
		app.keeper.params.Subspace(mint.DefaultParamspace),
		&skeeper,
		app.keeper.acct,
		app.keeper.bank,
		auth.FeeCollectorName,
	)

	// Create IBC Keeper
	app.keeper.ibc = ibc.NewKeeper(
		app.cdc, keys[ibc.StoreKey], app.keeper.staking, scopedIBC,
	)

	// Create Transfer Keepers
	app.keeper.transfer = transfer.NewKeeper(
		app.cdc, keys[transfer.StoreKey],
		app.keeper.ibc.ChannelKeeper, &app.keeper.ibc.PortKeeper,
		app.keeper.acct, app.keeper.bank,
		scopedTransfer,
	)
	transferModule := transfer.NewAppModule(app.keeper.transfer)

	// Create static IBC router, add transfer route, then set and seal it
	ibcRouter := port.NewRouter()
	ibcRouter.AddRoute(transfer.ModuleName, transferModule)
	app.keeper.ibc.SetRouter(ibcRouter)

	// create evidence keeper with router
	evidenceKeeper := evidence.NewKeeper(
		appCodec, keys[evidence.StoreKey], &app.keeper.staking, app.keeper.slashing,
	)
	evidenceRouter := evidence.NewRouter().
		AddRoute(ibcclient.RouterKey, ibcclient.HandlerClientMisbehaviour(app.keeper.ibc.ClientKeeper))

	evidenceKeeper.SetRouter(evidenceRouter)
	app.keeper.evidence = *evidenceKeeper

	app.keeper.crisis = crisis.NewKeeper(
		app.keeper.params.Subspace(crisis.DefaultParamspace), invCheckPeriod, app.keeper.bank, auth.FeeCollectorName,
	)

	app.keeper.upgrade = upgrade.NewKeeper(skipUpgradeHeights, keys[upgrade.StoreKey], appCodec, home)

	// register the proposal types
	govRouter := gov.NewRouter()
	govRouter.AddRoute(gov.RouterKey, gov.ProposalHandler).
		AddRoute(paramproposal.RouterKey, params.NewParamChangeProposalHandler(app.keeper.params)).
		AddRoute(distr.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.keeper.distr)).
		AddRoute(upgrade.RouterKey, upgrade.NewSoftwareUpgradeProposalHandler(app.keeper.upgrade))

	app.keeper.gov = gov.NewKeeper(
		appCodec,
		keys[gov.StoreKey],
		app.keeper.params.Subspace(gov.DefaultParamspace).WithKeyTable(gov.ParamKeyTable()),
		app.keeper.acct,
		app.keeper.bank,
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
		auth.NewAppModule(appCodec, app.keeper.acct),
		bank.NewAppModule(appCodec, app.keeper.bank, app.keeper.acct),
		capability.NewAppModule(*app.keeper.capability),
		distr.NewAppModule(appCodec, app.keeper.distr, app.keeper.acct, app.keeper.bank, app.keeper.staking),

		mint.NewAppModule(appCodec, app.keeper.mint, app.keeper.acct),
		slashing.NewAppModule(appCodec, app.keeper.slashing, app.keeper.acct, app.keeper.bank, app.keeper.staking),

		staking.NewAppModule(appCodec, app.keeper.staking, app.keeper.acct, app.keeper.bank),
		evidence.NewAppModule(appCodec, app.keeper.evidence),
		gov.NewAppModule(appCodec, app.keeper.gov, app.keeper.acct, app.keeper.bank),
		ibc.NewAppModule(app.keeper.ibc),
		params.NewAppModule(app.keeper.params),
		transferModule,
		upgrade.NewAppModule(app.keeper.upgrade),
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

	app.mm.SetOrderBeginBlockers(
		upgrade.ModuleName, mint.ModuleName, distr.ModuleName, slashing.ModuleName,
		evidence.ModuleName, staking.ModuleName, ibc.ModuleName,
	)
	app.mm.SetOrderEndBlockers(staking.ModuleName, crisis.ModuleName, gov.ModuleName, deployment.ModuleName, market.ModuleName)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		auth.ModuleName,
		distr.ModuleName,
		staking.ModuleName,
		bank.ModuleName,
		slashing.ModuleName,
		gov.ModuleName,
		mint.ModuleName,
		ibc.ModuleName,
		genutil.ModuleName,
		evidence.ModuleName,
		transfer.ModuleName,
		crisis.ModuleName,

		// akash
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
	)

	app.mm.RegisterInvariants(&app.keeper.crisis)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter())

	app.sm = module.NewSimulationManager(
		auth.NewAppModule(appCodec, app.keeper.acct),
		bank.NewAppModule(appCodec, app.keeper.bank, app.keeper.acct),
		gov.NewAppModule(appCodec, app.keeper.gov, app.keeper.acct, app.keeper.bank),
		mint.NewAppModule(appCodec, app.keeper.mint, app.keeper.acct),
		staking.NewAppModule(appCodec, app.keeper.staking, app.keeper.acct, app.keeper.bank),
		distr.NewAppModule(appCodec, app.keeper.distr, app.keeper.acct, app.keeper.bank, app.keeper.staking),
		slashing.NewAppModule(appCodec, app.keeper.slashing, app.keeper.acct, app.keeper.bank, app.keeper.staking),
		params.NewAppModule(app.keeper.params), // NOTE: only used for simulation to generate randomized param change proposals
		deployment.NewAppModuleSimulation(app.keeper.deployment, app.keeper.acct, app.keeper.bank),
		market.NewAppModuleSimulation(app.keeper.market, app.keeper.acct, app.keeper.deployment,
			app.keeper.provider, app.keeper.bank),
		provider.NewAppModuleSimulation(app.keeper.provider, app.keeper.acct, app.keeper.bank),
	)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)
	app.MountMemoryStores(memKeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)

	app.SetAnteHandler(
		auth.NewAnteHandler(
			app.keeper.acct,
			app.keeper.bank,
			*app.keeper.ibc,
			auth.DefaultSigVerificationGasConsumer,
		),
	)

	app.SetEndBlocker(app.EndBlocker)

	err := app.LoadLatestVersion()
	if err != nil {
		tmos.Exit("app initialization:" + err.Error())
	}

	// Initialize and seal the capability keeper so all persistent capabilities
	// are loaded in-memory and prevent any further modules from creating scoped
	// sub-keepers.
	// This must be done during creation of baseapp rather than in InitChain so
	// that in-memory capabilities get regenerated on app restart
	ctx := app.BaseApp.NewContext(true, abci.Header{})
	app.keeper.capability.InitializeAndSeal(ctx)

	app.keeper.scopedIBC = scopedIBC
	app.keeper.scopedTransfer = scopedTransfer

	return app
}

// InitChainer application update at chain initialization
func (app *AkashApp) InitChainer(
	ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	app.cdc.MustUnmarshalJSON(req.AppStateBytes, &genesisState)

	return app.mm.InitGenesis(ctx, app.cdc, genesisState)
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

// BlacklistedAccAddrs returns all the app's module account addresses black listed for receiving tokens.
func (app *AkashApp) BlacklistedAccAddrs() map[string]bool {
	blacklistedAddrs := make(map[string]bool)
	for acc := range macPerms() {
		blacklistedAddrs[auth.NewModuleAddress(acc).String()] = !allowedReceivingModAcc[acc]
	}

	return blacklistedAddrs
}

// SimulationManager implements the SimulationApp interface
func (app *AkashApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// LoadHeight method of AkashApp loads baseapp application version with given height
func (app *AkashApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// ExportAppStateAndValidators returns application state json and slice of validators
func (app *AkashApp) ExportAppStateAndValidators(
	forZeroHeight bool, jailWhiteList []string,
) (appState json.RawMessage, validators []tmtypes.GenesisValidator, cp *abci.ConsensusParams, err error) {

	// as if they could withdraw from the start of the next block
	ctx := app.NewContext(true, abci.Header{Height: app.LastBlockHeight()})

	genState := app.mm.ExportGenesis(ctx, app.cdc)
	appState, err = codec.MarshalJSONIndent(app.cdc, genState)
	if err != nil {
		return nil, nil, nil, err
	}

	validators = staking.WriteValidators(ctx, app.keeper.staking)

	return appState, validators, app.BaseApp.GetConsensusParams(ctx), nil
}
