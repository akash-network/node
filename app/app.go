package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cosmos/cosmos-sdk/x/feegrant"
	feegrantkeeper "github.com/cosmos/cosmos-sdk/x/feegrant/keeper"
	feegrantmodule "github.com/cosmos/cosmos-sdk/x/feegrant/module"
	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/cast"

	abci "github.com/tendermint/tendermint/abci/types"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmtypes "github.com/tendermint/tendermint/types"
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
	"github.com/cosmos/ibc-go/v4/modules/apps/transfer"
	ibc "github.com/cosmos/ibc-go/v4/modules/core"
	porttypes "github.com/cosmos/ibc-go/v4/modules/core/05-port/types"

	icacontrollertypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/host/types"
	ibctransferkeeper "github.com/cosmos/ibc-go/v4/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v4/modules/apps/transfer/types"
	ibcclient "github.com/cosmos/ibc-go/v4/modules/core/02-client"
	ibcclienttypes "github.com/cosmos/ibc-go/v4/modules/core/02-client/types"
	ibchost "github.com/cosmos/ibc-go/v4/modules/core/24-host"
	ibckeeper "github.com/cosmos/ibc-go/v4/modules/core/keeper"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"

	// unnamed import of statik for swagger UI support
	_ "github.com/akash-network/node/client/docs/statik"
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
	*bam.BaseApp
	apptypes.App
	cdc               *codec.LegacyAmino
	appCodec          codec.Codec
	interfaceRegistry codectypes.InterfaceRegistry

	invCheckPeriod uint

	keys    map[string]*sdk.KVStoreKey
	tkeys   map[string]*sdk.TransientStoreKey
	memkeys map[string]*sdk.MemoryStoreKey

	// simulation manager
	sm *module.SimulationManager
}

// https://github.com/cosmos/sdk-tutorials/blob/c6754a1e313eb1ed973c5c91dcc606f2fd288811/app.go#L73

// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger,
	db dbm.DB,
	tio io.Writer,
	loadLatest bool,
	invCheckPeriod uint,
	skipUpgradeHeights map[int64]bool,
	homePath string,
	appOpts servertypes.AppOptions,
	options ...func(*bam.BaseApp),
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
	app.Configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())

	app.Keepers.Cosmos.Params = initParamsKeeper(appCodec, cdc, app.keys[paramstypes.StoreKey], tkeys[paramstypes.TStoreKey])

	// set the BaseApp's parameter store
	bapp.SetParamStore(app.Keepers.Cosmos.Params.Subspace(bam.Paramspace).WithKeyTable(paramskeeper.ConsensusParamsKeyTable()))

	// add capability keeper and ScopeToModule for ibc module
	app.Keepers.Cosmos.Cap = capabilitykeeper.NewKeeper(
		appCodec,
		app.keys[capabilitytypes.StoreKey],
		app.memkeys[capabilitytypes.MemStoreKey],
	)

	scopedIBCKeeper := app.Keepers.Cosmos.Cap.ScopeToModule(ibchost.ModuleName)
	scopedTransferKeeper := app.Keepers.Cosmos.Cap.ScopeToModule(ibctransfertypes.ModuleName)

	// seal the capability keeper so all persistent capabilities are loaded in-memory and prevent
	// any further modules from creating scoped sub-keepers.
	app.Keepers.Cosmos.Cap.Seal()

	app.Keepers.Cosmos.Acct = authkeeper.NewAccountKeeper(
		appCodec,
		app.keys[authtypes.StoreKey],
		app.GetSubspace(authtypes.ModuleName),
		authtypes.ProtoBaseAccount,
		MacPerms(),
	)

	// add authz keeper
	app.Keepers.Cosmos.Authz = authzkeeper.NewKeeper(app.keys[authzkeeper.StoreKey], appCodec, app.MsgServiceRouter())

	app.Keepers.Cosmos.FeeGrant = feegrantkeeper.NewKeeper(
		appCodec,
		keys[feegrant.StoreKey],
		app.Keepers.Cosmos.Acct,
	)

	app.Keepers.Cosmos.Bank = bankkeeper.NewBaseKeeper(
		appCodec,
		app.keys[banktypes.StoreKey],
		app.Keepers.Cosmos.Acct,
		app.GetSubspace(banktypes.ModuleName),
		app.BlockedAddrs(),
	)

	skeeper := stakingkeeper.NewKeeper(
		appCodec,
		app.keys[stakingtypes.StoreKey],
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		app.GetSubspace(stakingtypes.ModuleName),
	)

	app.Keepers.Cosmos.Mint = mintkeeper.NewKeeper(
		appCodec,
		app.keys[minttypes.StoreKey],
		app.GetSubspace(minttypes.ModuleName),
		&skeeper,
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		authtypes.FeeCollectorName,
	)

	app.Keepers.Cosmos.Distr = distrkeeper.NewKeeper(
		appCodec,
		app.keys[distrtypes.StoreKey],
		app.GetSubspace(distrtypes.ModuleName),
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		&skeeper,
		authtypes.FeeCollectorName,
		app.ModuleAccountAddrs(),
	)

	app.Keepers.Cosmos.Slashing = slashingkeeper.NewKeeper(
		appCodec,
		app.keys[slashingtypes.StoreKey],
		&skeeper,
		app.GetSubspace(slashingtypes.ModuleName),
	)

	app.Keepers.Cosmos.Crisis = crisiskeeper.NewKeeper(
		app.GetSubspace(crisistypes.ModuleName),
		invCheckPeriod,
		app.Keepers.Cosmos.Bank,
		authtypes.FeeCollectorName,
	)

	app.Keepers.Cosmos.Upgrade = upgradekeeper.NewKeeper(skipUpgradeHeights, app.keys[upgradetypes.StoreKey], appCodec, homePath, app.BaseApp)

	// register the staking hooks
	// NOTE: stakingKeeper above is passed by reference, so that it will contain these hooks
	app.Keepers.Cosmos.Staking = *skeeper.SetHooks(
		stakingtypes.NewMultiStakingHooks(
			app.Keepers.Cosmos.Distr.Hooks(),
			app.Keepers.Cosmos.Slashing.Hooks(),
		),
	)

	// register IBC Keeper
	app.Keepers.Cosmos.IBC = ibckeeper.NewKeeper(
		appCodec,
		app.keys[ibchost.StoreKey],
		app.GetSubspace(ibchost.ModuleName),
		app.Keepers.Cosmos.Staking,
		app.Keepers.Cosmos.Upgrade,
		scopedIBCKeeper,
	)

	// register the proposal types
	govRouter := govtypes.NewRouter()
	govRouter.AddRoute(govtypes.RouterKey, govtypes.ProposalHandler).
		AddRoute(
			paramproposal.RouterKey,
			params.NewParamChangeProposalHandler(app.Keepers.Cosmos.Params),
		).
		AddRoute(
			distrtypes.RouterKey,
			distr.NewCommunityPoolSpendProposalHandler(app.Keepers.Cosmos.Distr),
		).
		AddRoute(
			upgradetypes.RouterKey,
			upgrade.NewSoftwareUpgradeProposalHandler(app.Keepers.Cosmos.Upgrade),
		).
		AddRoute(
			ibcclienttypes.RouterKey,
			ibcclient.NewClientProposalHandler(app.Keepers.Cosmos.IBC.ClientKeeper),
		)

	app.Keepers.Cosmos.Gov = govkeeper.NewKeeper(
		appCodec,
		app.keys[govtypes.StoreKey],
		app.GetSubspace(govtypes.ModuleName),
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		&skeeper,
		govRouter,
	)

	// register Transfer Keepers
	app.Keepers.Cosmos.Transfer = ibctransferkeeper.NewKeeper(
		appCodec,
		app.keys[ibctransfertypes.StoreKey],
		app.GetSubspace(ibctransfertypes.ModuleName),
		app.Keepers.Cosmos.IBC.ChannelKeeper,
		app.Keepers.Cosmos.IBC.ChannelKeeper,
		&app.Keepers.Cosmos.IBC.PortKeeper,
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		scopedTransferKeeper,
	)

	transferModule := transfer.NewAppModule(app.Keepers.Cosmos.Transfer)
	transferIBCModule := transfer.NewIBCModule(app.Keepers.Cosmos.Transfer)

	// Create static IBC router, add transfer route, then set and seal it
	ibcRouter := porttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, transferIBCModule)

	app.Keepers.Cosmos.IBC.SetRouter(ibcRouter)

	// create evidence keeper with evidence router
	evidenceKeeper := evidencekeeper.NewKeeper(
		appCodec,
		app.keys[evidencetypes.StoreKey],
		&app.Keepers.Cosmos.Staking,
		app.Keepers.Cosmos.Slashing,
	)

	// if evidence needs to be handled for the app, set routes in router here and seal
	app.Keepers.Cosmos.Evidence = *evidenceKeeper

	app.setAkashKeepers()

	// NOTE: we may consider parsing `appOpts` inside module constructors. For the moment
	// we prefer to be more strict in what arguments the modules expect.
	skipGenesisInvariants := cast.ToBool(appOpts.Get(crisis.FlagSkipGenesisInvariants))

	app.MM = module.NewManager(
		append([]module.AppModule{
			genutil.NewAppModule(app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Staking, app.BaseApp.DeliverTx, encodingConfig.TxConfig),
			auth.NewAppModule(appCodec, app.Keepers.Cosmos.Acct, nil),
			authzmodule.NewAppModule(appCodec, app.Keepers.Cosmos.Authz, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.interfaceRegistry),
			feegrantmodule.NewAppModule(appCodec, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.FeeGrant, app.interfaceRegistry),
			vesting.NewAppModule(app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank),
			bank.NewAppModule(appCodec, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.Acct),
			capability.NewAppModule(appCodec, *app.Keepers.Cosmos.Cap),
			crisis.NewAppModule(&app.Keepers.Cosmos.Crisis, skipGenesisInvariants),
			gov.NewAppModule(appCodec, app.Keepers.Cosmos.Gov, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank),
			mint.NewAppModule(appCodec, app.Keepers.Cosmos.Mint, app.Keepers.Cosmos.Acct),
			// todo akash-network/support#4
			// mint.NewAppModule(appCodec, app.Keepers.Cosmos.Mint, app.Keepers.Cosmos.Acct, nil),
			slashing.NewAppModule(appCodec, app.Keepers.Cosmos.Slashing, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.Staking),
			distr.NewAppModule(appCodec, app.Keepers.Cosmos.Distr, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.Staking),
			staking.NewAppModule(appCodec, app.Keepers.Cosmos.Staking, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank),
			upgrade.NewAppModule(app.Keepers.Cosmos.Upgrade),
			evidence.NewAppModule(app.Keepers.Cosmos.Evidence),
			ibc.NewAppModule(app.Keepers.Cosmos.IBC),
			params.NewAppModule(app.Keepers.Cosmos.Params),
			transferModule,
		}, app.akashAppModules()...)...,
	)

	// During begin block slashing happens after distr.BeginBlocker so that
	// there is nothing left over in the validator fee pool, to keep the
	// CanWithdrawInvariant invariant.
	// NOTE: staking module is required if HistoricalEntries param > 0
	// NOTE: capability module's beginblocker must come before any modules using capabilities (e.g. IBC)
	// NOTE: As of v0.45.0 of cosmos SDK, all modules need to be here.
	app.MM.SetOrderBeginBlockers(
		app.akashBeginBlockModules()...,
	)
	app.MM.SetOrderEndBlockers(
		app.akashEndBlockModules()...,
	)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.MM.SetOrderInitGenesis(
		app.akashInitGenesisOrder()...,
	)

	app.MM.RegisterInvariants(&app.Keepers.Cosmos.Crisis)
	app.MM.RegisterRoutes(app.Router(), app.QueryRouter(), encodingConfig.Amino)
	app.MM.RegisterServices(app.Configurator)

	// add test gRPC service for testing gRPC queries in isolation
	testdata.RegisterQueryServer(app.GRPCQueryRouter(), testdata.QueryImpl{})

	app.sm = module.NewSimulationManager(
		append([]module.AppModuleSimulation{
			auth.NewAppModule(appCodec, app.Keepers.Cosmos.Acct, authsims.RandomGenesisAccounts),
			authzmodule.NewAppModule(appCodec, app.Keepers.Cosmos.Authz, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.interfaceRegistry),
			bank.NewAppModule(appCodec, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.Acct),
			feegrantmodule.NewAppModule(appCodec, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.FeeGrant, app.interfaceRegistry),
			capability.NewAppModule(appCodec, *app.Keepers.Cosmos.Cap),
			gov.NewAppModule(appCodec, app.Keepers.Cosmos.Gov, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank),
			mint.NewAppModule(appCodec, app.Keepers.Cosmos.Mint, app.Keepers.Cosmos.Acct),
			// todo akash-network/support#4
			// mint.NewAppModule(appCodec, app.Keepers.Cosmos.Mint, app.Keepers.Cosmos.Acct, nil),
			staking.NewAppModule(appCodec, app.Keepers.Cosmos.Staking, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank),
			distr.NewAppModule(appCodec, app.Keepers.Cosmos.Distr, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.Staking),
			slashing.NewAppModule(appCodec, app.Keepers.Cosmos.Slashing, app.Keepers.Cosmos.Acct, app.Keepers.Cosmos.Bank, app.Keepers.Cosmos.Staking),
			params.NewAppModule(app.Keepers.Cosmos.Params),
			evidence.NewAppModule(app.Keepers.Cosmos.Evidence),
			ibc.NewAppModule(app.Keepers.Cosmos.IBC),
			transferModule,
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

	anteOpts := HandlerOptions{
		HandlerOptions: ante.HandlerOptions{
			AccountKeeper:   app.Keepers.Cosmos.Acct,
			BankKeeper:      app.Keepers.Cosmos.Bank,
			SignModeHandler: encodingConfig.TxConfig.SignModeHandler(),
			SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
		},
		CDC:            app.appCodec,
		AStakingKeeper: app.Keepers.Akash.Staking,
		GovKeeper:      &app.Keepers.Cosmos.Gov,
		AGovKeeper:     app.Keepers.Akash.Gov,
	}

	handler, err := NewAnteHandler(anteOpts)
	if err != nil {
		panic(err)
	}
	app.SetAnteHandler(handler)

	app.SetEndBlocker(app.EndBlocker)

	// register the upgrade handler
	if err = app.registerUpgradeHandlers(); err != nil {
		panic(err)
	}

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			tmos.Exit("app initialization:" + err.Error())
		}
	}

	app.Keepers.Cosmos.ScopedIBCKeeper = scopedIBCKeeper
	app.Keepers.Cosmos.ScopedTransferKeeper = scopedTransferKeeper

	return app
}

func getGenesisTime(appOpts servertypes.AppOptions, homePath string) time.Time { // nolint: unused,deadcode
	if v := appOpts.Get("GenesisTime"); v != nil {
		// in tests, GenesisTime is supplied using appOpts
		genTime, ok := v.(time.Time)
		if !ok {
			panic("expected GenesisTime to be a Time value")
		}
		return genTime
	}

	genDoc, err := tmtypes.GenesisDocFromFile(filepath.Join(homePath, "config/genesis.json"))
	if err != nil {
		panic(err)
	}

	return genDoc.GenesisTime
}

// MakeCodecs constructs the *std.Codec and *codec.LegacyAmino instances used by
// simapp. It is useful for tests and clients who do not want to construct the
// full simapp
func MakeCodecs() (codec.Codec, *codec.LegacyAmino) {
	config := MakeEncodingConfig()
	return config.Marshaler, config.Amino
}

// Name returns the name of the App
func (app *AkashApp) Name() string { return app.BaseApp.Name() }

// InitChainer application update at chain initialization
func (app *AkashApp) InitChainer(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	if err := tmjson.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	app.Keepers.Cosmos.Upgrade.SetModuleVersionMap(ctx, app.MM.GetVersionMap())
	return app.MM.InitGenesis(ctx, app.appCodec, genesisState)
}

// BeginBlocker is a function in which application updates every begin block
func (app *AkashApp) BeginBlocker(ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	if patch, exists := utypes.GetHeightPatchesList()[ctx.BlockHeight()]; exists {
		app.Logger().Info(fmt.Sprintf("found patch %s for current height %d. applying...", patch.Name(), ctx.BlockHeight()))
		patch.Begin(ctx, &app.Keepers)
		app.Logger().Info(fmt.Sprintf("patch %s applied successfully at height %d", patch.Name(), ctx.BlockHeight()))
	}

	return app.MM.BeginBlock(ctx, req)
}

// EndBlocker is a function in which application updates every end block
func (app *AkashApp) EndBlocker(
	ctx sdk.Context, req abci.RequestEndBlock,
) abci.ResponseEndBlock {
	return app.MM.EndBlock(ctx, req)
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
	subspace, _ := app.Keepers.Cosmos.Params.GetSubspace(moduleName)
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

// initParamsKeeper init params keeper and its subspaces
func initParamsKeeper(appCodec codec.BinaryCodec, legacyAmino *codec.LegacyAmino, key,
	tkey sdk.StoreKey,
) paramskeeper.Keeper {
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
