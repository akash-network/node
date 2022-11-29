package app

import (
	"fmt"
	"io"
	"net/http"
	"os"

	icahost "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/host"
	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/cast"

	abci "github.com/tendermint/tendermint/abci/types"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	dbm "github.com/tendermint/tm-db"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/simapp"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authrest "github.com/cosmos/cosmos-sdk/x/auth/client/rest"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/capability"
	capabilitykeeper "github.com/cosmos/cosmos-sdk/x/capability/keeper"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	evidencekeeper "github.com/cosmos/cosmos-sdk/x/evidence/keeper"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	paramproposal "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradekeeper "github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	ica "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts"
	icacontroller "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/controller"
	icacontrollerkeeper "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/controller/keeper"
	icacontrollertypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/controller/types"
	icahostkeeper "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/host/keeper"
	icahosttypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/host/types"
	"github.com/cosmos/ibc-go/v3/modules/apps/transfer"
	ibctransferkeeper "github.com/cosmos/ibc-go/v3/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v3/modules/apps/transfer/types"
	ibc "github.com/cosmos/ibc-go/v3/modules/core"
	ibcclient "github.com/cosmos/ibc-go/v3/modules/core/02-client"
	ibcclienttypes "github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	porttypes "github.com/cosmos/ibc-go/v3/modules/core/05-port/types"
	ibchost "github.com/cosmos/ibc-go/v3/modules/core/24-host"
	ibckeeper "github.com/cosmos/ibc-go/v3/modules/core/keeper"

	"github.com/ovrclk/akash/x/audit"
	"github.com/ovrclk/akash/x/cert"
	dkeeper "github.com/ovrclk/akash/x/deployment/keeper"
	escrowkeeper "github.com/ovrclk/akash/x/escrow/keeper"
	"github.com/ovrclk/akash/x/icaauth"
	icaauthkeeper "github.com/ovrclk/akash/x/icaauth/keeper"
	icaauthtypes "github.com/ovrclk/akash/x/icaauth/types/v1beta2"
	"github.com/ovrclk/akash/x/inflation"
	mkeeper "github.com/ovrclk/akash/x/market/keeper"
	pkeeper "github.com/ovrclk/akash/x/provider/keeper"

	// unnamed import of statik for swagger UI support
	_ "github.com/ovrclk/akash/client/docs/statik"
)

const (
	AppName = "akash"
)

var (
	DefaultHome                         = os.ExpandEnv("$HOME/.akash")
	_           servertypes.Application = (*AkashApp)(nil)

	// module accounts that are allowed to receive tokens
	allowedReceivingModAcc = map[string]bool{}
)

// AkashApp extends ABCI application
type AkashApp struct {
	// https://github.com/cosmos/sdk-tutorials/blob/c6754a1e313eb1ed973c5c91dcc606f2fd288811/app.go#L73
	*bam.BaseApp
	cdc               *codec.LegacyAmino
	appCodec          codec.Codec
	interfaceRegistry codectypes.InterfaceRegistry

	invCheckPeriod uint

	keys    map[string]*sdk.KVStoreKey
	tkeys   map[string]*sdk.TransientStoreKey
	memkeys map[string]*sdk.MemoryStoreKey

	Keeper struct {
		acct     authkeeper.AccountKeeper
		authz    authzkeeper.Keeper
		Bank     bankkeeper.Keeper
		cap      *capabilitykeeper.Keeper
		staking  stakingkeeper.Keeper
		slashing slashingkeeper.Keeper
		mint     mintkeeper.Keeper
		distr    distrkeeper.Keeper
		gov      govkeeper.Keeper
		crisis   crisiskeeper.Keeper
		Upgrade  upgradekeeper.Keeper
		params   paramskeeper.Keeper
		IBC      *ibckeeper.Keeper
		evidence evidencekeeper.Keeper
		transfer ibctransferkeeper.Keeper

		// interchain accounts
		icaHost       icahostkeeper.Keeper
		ICAController icacontrollerkeeper.Keeper
		icaAuth       icaauthkeeper.Keeper

		// make scoped keepers public for test purposes
		scopedIBC           capabilitykeeper.ScopedKeeper
		scopedTransfer      capabilitykeeper.ScopedKeeper
		scopedICAController capabilitykeeper.ScopedKeeper
		scopedICAHost       capabilitykeeper.ScopedKeeper
		scopedICAAuth       capabilitykeeper.ScopedKeeper

		// akash keepers
		escrow     escrowkeeper.Keeper
		deployment dkeeper.IKeeper
		market     mkeeper.IKeeper
		provider   pkeeper.IKeeper
		audit      audit.Keeper
		cert       cert.Keeper
		inflation  inflation.Keeper
	}

	mm *module.Manager

	// simulation manager
	sm *module.SimulationManager

	// module configurator
	configurator module.Configurator
}

// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger, db dbm.DB, tio io.Writer, loadLatest bool, invCheckPeriod uint, skipUpgradeHeights map[int64]bool,
	homePath string, appOpts servertypes.AppOptions, options ...func(*bam.BaseApp),
) *AkashApp {
	// find out the genesis time, to be used later in inflation calculation
	// genesisTime := getGenesisTime(appOpts, homePath)

	// TODO: Remove cdc in favor of appCodec once all modules are migrated.
	encodingConfig := MakeEncodingConfig()
	appCodec := encodingConfig.Marshaler
	cdc := encodingConfig.Amino
	interfaceRegistry := encodingConfig.InterfaceRegistry

	bapp := bam.NewBaseApp(AppName, logger, db, encodingConfig.TxConfig.TxDecoder(), options...)
	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetVersion(version.Version)
	bapp.SetInterfaceRegistry(interfaceRegistry)

	keys := kvStoreKeys()
	tkeys := transientStoreKeys()
	memkeys := memStoreKeys()

	app := &AkashApp{
		BaseApp:           bapp,
		cdc:               cdc,
		appCodec:          appCodec,
		interfaceRegistry: interfaceRegistry,
		invCheckPeriod:    invCheckPeriod,
		keys:              keys,
		tkeys:             tkeys,
		memkeys:           memkeys,
	}
	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())

	app.Keeper.params = initParamsKeeper(appCodec, cdc, app.keys[paramstypes.StoreKey], tkeys[paramstypes.TStoreKey])

	// set the BaseApp's parameter store
	bapp.SetParamStore(app.Keeper.params.Subspace(bam.Paramspace).WithKeyTable(paramskeeper.ConsensusParamsKeyTable()))

	// add capability keeper and ScopeToModule for ibc module
	app.Keeper.cap = capabilitykeeper.NewKeeper(appCodec, app.keys[capabilitytypes.StoreKey], app.memkeys[capabilitytypes.MemStoreKey])

	scopedIBCKeeper := app.Keeper.cap.ScopeToModule(ibchost.ModuleName)
	scopedTransferKeeper := app.Keeper.cap.ScopeToModule(ibctransfertypes.ModuleName)
	scopedICAAuthKeeper := app.Keeper.cap.ScopeToModule(icaauthtypes.ModuleName)
	scopedICAHostKeeper := app.Keeper.cap.ScopeToModule(icahosttypes.SubModuleName)
	scopedICAControllerKeeper := app.Keeper.cap.ScopeToModule(icacontrollertypes.SubModuleName)

	// seal the capability keeper so all persistent capabilities are loaded in-memory and prevent
	// any further modules from creating scoped sub-keepers.
	app.Keeper.cap.Seal()

	app.Keeper.acct = authkeeper.NewAccountKeeper(
		appCodec,
		app.keys[authtypes.StoreKey],
		app.GetSubspace(authtypes.ModuleName),
		authtypes.ProtoBaseAccount,
		MacPerms(),
	)

	// add authz keeper
	app.Keeper.authz = authzkeeper.NewKeeper(app.keys[authzkeeper.StoreKey], appCodec, app.MsgServiceRouter())

	app.Keeper.Bank = bankkeeper.NewBaseKeeper(
		appCodec,
		app.keys[banktypes.StoreKey],
		app.Keeper.acct,
		app.GetSubspace(banktypes.ModuleName),
		app.BlockedAddrs(),
	)

	skeeper := stakingkeeper.NewKeeper(
		appCodec,
		app.keys[stakingtypes.StoreKey],
		app.Keeper.acct,
		app.Keeper.Bank,
		app.GetSubspace(stakingtypes.ModuleName),
	)

	app.Keeper.mint = mintkeeper.NewKeeper(
		appCodec,
		app.keys[minttypes.StoreKey],
		app.GetSubspace(minttypes.ModuleName),
		&skeeper,
		app.Keeper.acct,
		app.Keeper.Bank,
		authtypes.FeeCollectorName,
	)

	app.Keeper.distr = distrkeeper.NewKeeper(
		appCodec,
		app.keys[distrtypes.StoreKey],
		app.GetSubspace(distrtypes.ModuleName),
		app.Keeper.acct,
		app.Keeper.Bank,
		&skeeper,
		authtypes.FeeCollectorName,
		app.ModuleAccountAddrs(),
	)

	app.Keeper.slashing = slashingkeeper.NewKeeper(
		appCodec,
		app.keys[slashingtypes.StoreKey],
		&skeeper,
		app.GetSubspace(slashingtypes.ModuleName),
	)

	app.Keeper.crisis = crisiskeeper.NewKeeper(
		app.GetSubspace(crisistypes.ModuleName),
		invCheckPeriod,
		app.Keeper.Bank,
		authtypes.FeeCollectorName,
	)

	app.Keeper.Upgrade = upgradekeeper.NewKeeper(
		skipUpgradeHeights,
		app.keys[upgradetypes.StoreKey],
		appCodec,
		homePath,
		app.BaseApp,
	)

	// register the staking hooks
	// NOTE: stakingKeeper above is passed by reference, so that it will contain these hooks
	app.Keeper.staking = *skeeper.SetHooks(
		stakingtypes.NewMultiStakingHooks(
			app.Keeper.distr.Hooks(),
			app.Keeper.slashing.Hooks(),
		),
	)

	// register IBC Keeper
	app.Keeper.IBC = ibckeeper.NewKeeper(
		appCodec,
		app.keys[ibchost.StoreKey],
		app.GetSubspace(ibchost.ModuleName),
		app.Keeper.staking,
		app.Keeper.Upgrade,
		scopedIBCKeeper,
	)

	// register the proposal types
	govRouter := govtypes.NewRouter()
	govRouter.AddRoute(govtypes.RouterKey, govtypes.ProposalHandler).
		AddRoute(paramproposal.RouterKey, params.NewParamChangeProposalHandler(app.Keeper.params)).
		AddRoute(distrtypes.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.Keeper.distr)).
		AddRoute(upgradetypes.RouterKey, upgrade.NewSoftwareUpgradeProposalHandler(app.Keeper.Upgrade)).
		AddRoute(ibcclienttypes.RouterKey, ibcclient.NewClientProposalHandler(app.Keeper.IBC.ClientKeeper))

	app.Keeper.gov = govkeeper.NewKeeper(
		appCodec,
		app.keys[govtypes.StoreKey],
		app.GetSubspace(govtypes.ModuleName),
		app.Keeper.acct,
		app.Keeper.Bank,
		&skeeper,
		govRouter,
	)

	// register Transfer Keepers
	app.Keeper.transfer = ibctransferkeeper.NewKeeper(
		appCodec,
		app.keys[ibctransfertypes.StoreKey],
		app.GetSubspace(ibctransfertypes.ModuleName),
		app.Keeper.IBC.ChannelKeeper,
		app.Keeper.IBC.ChannelKeeper,
		&app.Keeper.IBC.PortKeeper,
		app.Keeper.acct,
		app.Keeper.Bank,
		scopedTransferKeeper,
	)

	transferModule := transfer.NewAppModule(app.Keeper.transfer)
	transferIBCModule := transfer.NewIBCModule(app.Keeper.transfer)

	app.Keeper.ICAController = icacontrollerkeeper.NewKeeper(
		appCodec,
		keys[icacontrollertypes.StoreKey],
		app.GetSubspace(icacontrollertypes.SubModuleName),
		app.Keeper.IBC.ChannelKeeper, // may be replaced with middleware such as ics29 fee
		app.Keeper.IBC.ChannelKeeper,
		&app.Keeper.IBC.PortKeeper,
		scopedICAControllerKeeper,
		app.MsgServiceRouter(),
	)

	app.Keeper.icaAuth = icaauthkeeper.NewKeeper(
		appCodec,
		keys[icaauthtypes.StoreKey],
		app.Keeper.ICAController,
		scopedICAAuthKeeper,
	)

	app.Keeper.icaHost = icahostkeeper.NewKeeper(
		appCodec,
		keys[icahosttypes.StoreKey],
		app.GetSubspace(icahosttypes.SubModuleName),
		app.Keeper.IBC.ChannelKeeper,
		&app.Keeper.IBC.PortKeeper,
		app.Keeper.acct,
		scopedICAHostKeeper,
		app.MsgServiceRouter(),
	)

	icaModule := ica.NewAppModule(&app.Keeper.ICAController, &app.Keeper.icaHost)
	icaAuthModule := icaauth.NewAppModule(appCodec, app.Keeper.icaAuth)

	icaHostIBCModule := icahost.NewIBCModule(app.Keeper.icaHost)
	icaAuthIBCModule := icaauth.NewIBCModule(app.Keeper.icaAuth)
	icaControllerIBCModule := icacontroller.NewIBCModule(app.Keeper.ICAController, icaAuthIBCModule)

	// Create static IBC router, add transfer route, then set and seal it
	ibcRouter := porttypes.NewRouter()
	ibcRouter.
		AddRoute(icacontrollertypes.SubModuleName, icaControllerIBCModule).
		AddRoute(icaauthtypes.ModuleName, icaControllerIBCModule).
		AddRoute(icahosttypes.SubModuleName, icaHostIBCModule).
		AddRoute(ibctransfertypes.ModuleName, transferIBCModule)

	app.Keeper.IBC.SetRouter(ibcRouter)

	// create evidence keeper with evidence router
	evidenceKeeper := evidencekeeper.NewKeeper(
		appCodec,
		app.keys[evidencetypes.StoreKey],
		&app.Keeper.staking,
		app.Keeper.slashing,
	)

	// if evidence needs to be handled for the app, set routes in router here and seal
	app.Keeper.evidence = *evidenceKeeper

	app.setAkashKeepers()

	// NOTE: we may consider parsing `appOpts` inside module constructors. For the moment
	// we prefer to be more strict in what arguments the modules expect.
	var skipGenesisInvariants = cast.ToBool(appOpts.Get(crisis.FlagSkipGenesisInvariants))

	app.mm = module.NewManager(
		append([]module.AppModule{
			genutil.NewAppModule(app.Keeper.acct, app.Keeper.staking, app.BaseApp.DeliverTx, encodingConfig.TxConfig),
			auth.NewAppModule(appCodec, app.Keeper.acct, nil),
			authzmodule.NewAppModule(appCodec, app.Keeper.authz, app.Keeper.acct, app.Keeper.Bank, app.interfaceRegistry),
			vesting.NewAppModule(app.Keeper.acct, app.Keeper.Bank),
			bank.NewAppModule(appCodec, app.Keeper.Bank, app.Keeper.acct),
			capability.NewAppModule(appCodec, *app.Keeper.cap),
			crisis.NewAppModule(&app.Keeper.crisis, skipGenesisInvariants),
			gov.NewAppModule(appCodec, app.Keeper.gov, app.Keeper.acct, app.Keeper.Bank),
			mint.NewAppModule(appCodec, app.Keeper.mint, app.Keeper.acct),
			// todo ovrclk/engineering#603
			// mint.NewAppModule(appCodec, app.keeper.mint, app.keeper.acct, nil),
			slashing.NewAppModule(appCodec, app.Keeper.slashing, app.Keeper.acct, app.Keeper.Bank, app.Keeper.staking),
			distr.NewAppModule(appCodec, app.Keeper.distr, app.Keeper.acct, app.Keeper.Bank, app.Keeper.staking),
			staking.NewAppModule(appCodec, app.Keeper.staking, app.Keeper.acct, app.Keeper.Bank),
			upgrade.NewAppModule(app.Keeper.Upgrade),
			evidence.NewAppModule(app.Keeper.evidence),
			ibc.NewAppModule(app.Keeper.IBC),
			params.NewAppModule(app.Keeper.params),
			transferModule,
			icaModule,
			icaAuthModule,
		}, app.akashAppModules()...)...,
	)

	// During begin block slashing happens after distr.BeginBlocker so that
	// there is nothing left over in the validator fee pool, to keep the
	// CanWithdrawInvariant invariant.
	// NOTE: staking module is required if HistoricalEntries param > 0
	// NOTE: capability module's beginblocker must come before any modules using capabilities (e.g. IBC)
	// NOTE: As of v0.45.0 of cosmos SDK, all modules need to be here.
	app.mm.SetOrderBeginBlockers(
		app.akashBeginBlockModules()...,
	)
	app.mm.SetOrderEndBlockers(
		app.akashEndBlockModules()...,
	)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		app.akashInitGenesisOrder()...,
	)

	app.mm.RegisterInvariants(&app.Keeper.crisis)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter(), encodingConfig.Amino)
	app.mm.RegisterServices(app.configurator)

	// add test gRPC service for testing gRPC queries in isolation
	testdata.RegisterQueryServer(app.GRPCQueryRouter(), testdata.QueryImpl{})

	app.sm = module.NewSimulationManager(
		append([]module.AppModuleSimulation{
			auth.NewAppModule(appCodec, app.Keeper.acct, authsims.RandomGenesisAccounts),
			authzmodule.NewAppModule(appCodec, app.Keeper.authz, app.Keeper.acct, app.Keeper.Bank, app.interfaceRegistry),
			bank.NewAppModule(appCodec, app.Keeper.Bank, app.Keeper.acct),
			capability.NewAppModule(appCodec, *app.Keeper.cap),
			gov.NewAppModule(appCodec, app.Keeper.gov, app.Keeper.acct, app.Keeper.Bank),
			mint.NewAppModule(appCodec, app.Keeper.mint, app.Keeper.acct),
			// todo ovrclk/engineering#603
			// mint.NewAppModule(appCodec, app.keeper.mint, app.keeper.acct, nil),
			staking.NewAppModule(appCodec, app.Keeper.staking, app.Keeper.acct, app.Keeper.Bank),
			distr.NewAppModule(appCodec, app.Keeper.distr, app.Keeper.acct, app.Keeper.Bank, app.Keeper.staking),
			slashing.NewAppModule(appCodec, app.Keeper.slashing, app.Keeper.acct, app.Keeper.Bank, app.Keeper.staking),
			params.NewAppModule(app.Keeper.params),
			evidence.NewAppModule(app.Keeper.evidence),
			ibc.NewAppModule(app.Keeper.IBC),
			transferModule,
			NewICAHostSimModule(icaModule, appCodec),
		},
			app.akashSimModules()...,
		)...,
	)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)
	app.MountMemoryStores(memkeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)

	handler, err := ante.NewAnteHandler(ante.HandlerOptions{
		AccountKeeper:   app.Keeper.acct,
		BankKeeper:      app.Keeper.Bank,
		SignModeHandler: encodingConfig.TxConfig.SignModeHandler(),
		SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
	})
	if err != nil {
		panic(err)
	}
	app.SetAnteHandler(handler)

	app.SetEndBlocker(app.EndBlocker)

	// register the upgrade handler
	app.registerUpgradeHandlers(icaModule)

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			tmos.Exit("app initialization:" + err.Error())
		}
	}

	app.Keeper.scopedIBC = scopedIBCKeeper
	app.Keeper.scopedTransfer = scopedTransferKeeper
	app.Keeper.scopedICAController = scopedICAControllerKeeper
	app.Keeper.scopedICAHost = scopedICAHostKeeper
	app.Keeper.scopedICAAuth = scopedICAAuthKeeper

	return app
}

func (app *AkashApp) registerUpgradeHandlers(icaModule ica.AppModule) {
	handlers := app.loadUpgradeHandlers(icaModule)

	for name, fn := range handlers {
		if fn.handler == nil {
			panic(fmt.Sprintf("upgrade \"%s\" does not have handler set", name))
		}

		app.Keeper.Upgrade.SetUpgradeHandler(name, fn.handler)
	}

	upgradeInfo, err := app.Keeper.Upgrade.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}

	if funcs, exists := handlers[upgradeInfo.Name]; exists && funcs.storeLoader != nil && !app.Keeper.Upgrade.IsSkipHeight(upgradeInfo.Height) {
		app.SetStoreLoader(funcs.storeLoader(upgradeInfo.Height))
	}
}

// Name returns the name of the App
func (app *AkashApp) Name() string { return app.BaseApp.Name() }

// InitChainer application update at chain initialization
func (app *AkashApp) InitChainer(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	if err := tmjson.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	app.Keeper.Upgrade.SetModuleVersionMap(ctx, app.mm.GetVersionMap())
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
func (app *AkashApp) AppCodec() codec.Codec {
	return app.appCodec
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *AkashApp) ModuleAccountAddrs() map[string]bool {
	return MacAddrs()
}

// BlockedAddrs returns all the app's module account addresses that are not
// allowed to receive external tokens.
func (app *AkashApp) BlockedAddrs() map[string]bool {
	perms := MacPerms()
	blockedAddrs := make(map[string]bool)
	for acc := range perms {
		blockedAddrs[authtypes.NewModuleAddress(acc).String()] = !allowedReceivingModAcc[acc]
	}

	return blockedAddrs
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
	subspace, _ := app.Keeper.params.GetSubspace(moduleName)
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
	// Register legacy tx routes
	authrest.RegisterTxRoutes(clientCtx, apiSvr.Router)
	// Register new tx routes from grpc-gateway
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	// Register new tendermint queries routes from grpc-gateway.
	tmservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register legacy and grpc-gateway routes for all modules.
	ModuleBasics().RegisterRESTRoutes(clientCtx, apiSvr.Router)
	ModuleBasics().RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// register swagger API from root so that other applications can override easily
	if apiConfig.Swagger {
		RegisterSwaggerAPI(clientCtx, apiSvr.Router)
	}
}

// RegisterTxService implements the Application.RegisterTxService method.
func (app *AkashApp) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.BaseApp.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *AkashApp) RegisterTendermintService(clientCtx client.Context) {
	tmservice.RegisterTendermintService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.interfaceRegistry)
}

// RegisterSwaggerAPI registers swagger route with API Server
func RegisterSwaggerAPI(_ client.Context, rtr *mux.Router) {
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

func (app *AkashApp) GetBaseApp() *bam.BaseApp {
	return app.BaseApp
}

func (app *AkashApp) GetStakingKeeper() stakingkeeper.Keeper {
	return app.Keeper.staking
}

func (app *AkashApp) GetIBCKeeper() *ibckeeper.Keeper {
	return app.Keeper.IBC
}

func (app *AkashApp) GetScopedIBCKeeper() capabilitykeeper.ScopedKeeper {
	return app.Keeper.scopedIBC
}

func (app *AkashApp) GetTxConfig() client.TxConfig {
	cfg := MakeEncodingConfig()
	return cfg.TxConfig
}

// initParamsKeeper init params keeper and its subspaces
func initParamsKeeper(appCodec codec.BinaryCodec, legacyAmino *codec.LegacyAmino, key, tkey sdk.StoreKey) paramskeeper.Keeper {
	paramsKeeper := paramskeeper.NewKeeper(appCodec, legacyAmino, key, tkey)

	paramsKeeper.Subspace(authtypes.ModuleName)
	paramsKeeper.Subspace(banktypes.ModuleName)
	paramsKeeper.Subspace(stakingtypes.ModuleName)
	paramsKeeper.Subspace(minttypes.ModuleName)
	paramsKeeper.Subspace(distrtypes.ModuleName)
	paramsKeeper.Subspace(slashingtypes.ModuleName)
	paramsKeeper.Subspace(govtypes.ModuleName).WithKeyTable(govtypes.ParamKeyTable())
	paramsKeeper.Subspace(crisistypes.ModuleName)
	paramsKeeper.Subspace(ibctransfertypes.ModuleName)
	paramsKeeper.Subspace(ibchost.ModuleName)
	paramsKeeper.Subspace(icacontrollertypes.SubModuleName)
	paramsKeeper.Subspace(icahosttypes.SubModuleName)

	return akashSubspaces(paramsKeeper)
}
