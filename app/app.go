package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/cast"

	abci "github.com/cometbft/cometbft/abci/types"
	tmjson "github.com/cometbft/cometbft/libs/json"
	cmos "github.com/cometbft/cometbft/libs/os"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	reflectionv1 "cosmossdk.io/api/cosmos/reflection/v1"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	runtimeservices "github.com/cosmos/cosmos-sdk/runtime/services"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v10/modules/core/exported"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"

	cflags "pkg.akt.dev/go/cli/flags"
	audittypes "pkg.akt.dev/go/node/audit/v1"
	certtypes "pkg.akt.dev/go/node/cert/v1"
	deploymenttypes "pkg.akt.dev/go/node/deployment/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	markettypes "pkg.akt.dev/go/node/market/v1"
	providertypes "pkg.akt.dev/go/node/provider/v1beta4"
	taketypes "pkg.akt.dev/go/node/take/v1"
	"pkg.akt.dev/go/sdkutil"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	awasm "pkg.akt.dev/node/v2/x/wasm"
	// unnamed import of statik for swagger UI support
	_ "pkg.akt.dev/node/v2/client/docs/statik"
)

const (
	AppName = "akash"
)

var (
	DefaultHome = os.ExpandEnv("$HOME/.akash")

	_ runtime.AppI            = (*AkashApp)(nil)
	_ servertypes.Application = (*AkashApp)(nil)

	// module accounts that are allowed to receive tokens
	allowedReceivingModAcc = map[string]bool{}
)

// AkashApp extends ABCI application
type AkashApp struct {
	*baseapp.BaseApp
	*apptypes.App

	aminoCdc          *codec.LegacyAmino
	cdc               codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry
	sm                *module.SimulationManager
	invCheckPeriod    uint
}

// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger,
	db dbm.DB,
	tio io.Writer,
	loadLatest bool,
	invCheckPeriod uint,
	skipUpgradeHeights map[int64]bool,
	encodingConfig sdkutil.EncodingConfig,
	appOpts servertypes.AppOptions,
	options ...func(*baseapp.BaseApp),
) *AkashApp {
	appCodec := encodingConfig.Codec
	aminoCdc := encodingConfig.Amino
	interfaceRegistry := encodingConfig.InterfaceRegistry
	txConfig := encodingConfig.TxConfig

	bapp := baseapp.NewBaseApp(AppName, logger, db, txConfig.TxDecoder(), options...)

	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetVersion(version.Version)
	bapp.SetInterfaceRegistry(interfaceRegistry)
	bapp.SetTxEncoder(txConfig.TxEncoder())

	homePath := cast.ToString(appOpts.Get(cflags.FlagHome))
	if homePath == "" {
		homePath = DefaultHome
	}

	var wasmOpts []wasmkeeper.Option

	if val := appOpts.Get("wasm"); val != nil {
		if vl, valid := val.([]wasmkeeper.Option); valid {
			wasmOpts = append(wasmOpts, vl...)
		} else {
			panic(fmt.Sprintf("invalid type for aptOpts.Get(\"wasm\"). expected %s, actual %s", reflect.TypeOf(wasmOpts).String(), reflect.TypeOf(val).String()))
		}
	}

	app := &AkashApp{
		BaseApp: bapp,
		App: &apptypes.App{
			Log: logger,
		},
		aminoCdc:          aminoCdc,
		cdc:               appCodec,
		txConfig:          txConfig,
		interfaceRegistry: interfaceRegistry,
		invCheckPeriod:    invCheckPeriod,
	}

	wasmDir := filepath.Join(homePath, "wasm")
	wasmConfig, err := wasm.ReadNodeConfig(appOpts)
	if err != nil {
		panic(fmt.Sprintf("error while reading wasm config: %s", err))
	}

	// Memory limits - prevent DoS
	wasmConfig.MemoryCacheSize = 100 // 100 MB max
	// Query gas limit - prevent expensive queries
	wasmConfig.SmartQueryGasLimit = 3_000_000
	// Debug mode - MUST be false in production
	// Uncomment this for debugging contracts. In the future this could be made into a param passed by the tests
	wasmConfig.ContractDebugMode = false

	app.InitSpecialKeepers(
		app.cdc,
		aminoCdc,
		app.BaseApp,
		skipUpgradeHeights,
		homePath,
	)

	app.InitNormalKeepers(
		app.cdc,
		encodingConfig,
		app.BaseApp,
		ModuleAccountPerms(),
		wasmDir,
		wasmConfig,
		wasmOpts,
		app.BlockedAddrs(),
		invCheckPeriod,
	)

	// TODO: There is a bug here, where we register the govRouter routes in InitNormalKeepers and then
	// call setupHooks afterwards. Therefore, if a gov proposal needs to call a method and that method calls a
	// hook, we will get a nil pointer dereference error due to the hooks in the keeper not being
	// setup yet. I will refrain from creating an issue in the sdk for now until after we unfork to 0.47,
	// because I believe the concept of Routes is going away.
	app.SetupHooks()

	// NOTE: All module / keeper changes should happen prior to this module.NewManager line being called.
	// However, in the event any changes do need to happen after this call, ensure that that keeper
	// is only passed in its keeper form (not de-ref'd anywhere)
	//
	// Generally NewAppModule will require the keeper that module defines to be passed in as an exact struct,
	// but should take in every other keeper as long as it matches a certain interface. (So no need to be de-ref'd)
	//
	// Any time a module requires a keeper de-ref'd that's not its native one,
	// its code-smell and should probably change. We should get the staking keeper dependencies fixed.
	modules := appModules(app, encodingConfig)

	app.MM = module.NewManager(modules...)

	// During begin block slashing happens after distr.BeginBlocker so that
	// there is nothing left over in the validator fee pool, to keep the
	// CanWithdrawInvariant invariant.
	// NOTE: staking module is required if HistoricalEntries param > 0
	// NOTE: capability module's begin-blocker must come before any modules using capabilities (e.g. IBC)

	// Upgrades from v0.50.x onwards happen in pre block
	app.MM.SetOrderPreBlockers(
		upgradetypes.ModuleName,
		authtypes.ModuleName,
	)

	// Tell the app's module manager how to set the order of BeginBlockers, which are run at the beginning of every block.
	app.MM.SetOrderBeginBlockers(orderBeginBlockers(app.MM.ModuleNames())...)
	app.MM.SetOrderInitGenesis(OrderInitGenesis(app.MM.ModuleNames())...)

	app.Configurator = module.NewConfigurator(app.AppCodec(), app.MsgServiceRouter(), app.GRPCQueryRouter())
	err = app.MM.RegisterServices(app.Configurator)
	if err != nil {
		panic(err)
	}

	// register the upgrade handler
	if err := app.registerUpgradeHandlers(); err != nil {
		panic(err)
	}

	app.sm = module.NewSimulationManager(appSimModules(app, encodingConfig)...)
	app.sm.RegisterStoreDecoders()

	autocliv1.RegisterQueryServer(app.GRPCQueryRouter(), runtimeservices.NewAutoCLIQueryService(app.MM.Modules))

	reflectionSvc := getReflectionService()
	reflectionv1.RegisterReflectionServiceServer(app.GRPCQueryRouter(), reflectionSvc)

	// initialize stores
	app.MountKVStores(app.GetKVStoreKey())
	app.MountTransientStores(app.GetTransientStoreKey())

	anteOpts := HandlerOptions{
		HandlerOptions: ante.HandlerOptions{
			AccountKeeper:   app.Keepers.Cosmos.Acct,
			BankKeeper:      app.Keepers.Cosmos.Bank,
			FeegrantKeeper:  app.Keepers.Cosmos.FeeGrant,
			SignModeHandler: encodingConfig.TxConfig.SignModeHandler(),
			SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
		},
		CDC:       app.cdc,
		GovKeeper: app.Keepers.Cosmos.Gov,
	}

	anteHandler, err := NewAnteHandler(anteOpts)
	if err != nil {
		panic(err)
	}

	app.SetPrepareProposal(baseapp.NoOpPrepareProposal())

	// we use a no-op ProcessProposal, this way, we accept all proposals in avoidance
	// of liveness failures due to Prepare / Process inconsistency. In other words,
	// this ProcessProposal always returns ACCEPT.
	app.SetProcessProposal(baseapp.NoOpProcessProposal())

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PreBlocker)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetAnteHandler(anteHandler)
	app.SetEndBlocker(app.EndBlocker)
	app.SetPrecommiter(app.Precommitter)
	app.SetPrepareCheckStater(app.PrepareCheckStater)

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			cmos.Exit("app initialization:" + err.Error())
		}
	}

	return app
}

// orderBeginBlockers returns the order of BeginBlockers, by module name.
func orderBeginBlockers(_ []string) []string {
	return []string{
		upgradetypes.ModuleName,
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
		authtypes.ModuleName,
		authz.ModuleName,
		taketypes.ModuleName,
		emodule.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		stakingtypes.ModuleName,
		transfertypes.ModuleName,
		consensusparamtypes.ModuleName,
		ibctm.ModuleName,
		ibchost.ModuleName,
		feegrant.ModuleName,
		// akash wasm module must be prior wasm
		awasm.ModuleName,
		// wasm after ibc transfer
		wasmtypes.ModuleName,
	}
}

// OrderEndBlockers returns EndBlockers (crisis, govtypes, staking) with no relative order.
func OrderEndBlockers(_ []string) []string {
	return []string{
		govtypes.ModuleName,
		stakingtypes.ModuleName,
		upgradetypes.ModuleName,
		banktypes.ModuleName,
		paramstypes.ModuleName,
		deploymenttypes.ModuleName,
		providertypes.ModuleName,
		certtypes.ModuleName,
		markettypes.ModuleName,
		audittypes.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		taketypes.ModuleName,
		emodule.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		feegrant.ModuleName,
		// akash wasm module must be prior wasm
		awasm.ModuleName,
		// wasm after ibc transfer
		wasmtypes.ModuleName,
	}
}

func getGenesisTime(appOpts servertypes.AppOptions, homePath string) time.Time { // nolint: unused
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

// Name returns the name of the App
func (app *AkashApp) Name() string { return app.BaseApp.Name() }

// InitChainer application update at chain initialization
func (app *AkashApp) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState GenesisState
	if err := tmjson.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	err := app.Keepers.Cosmos.Upgrade.SetModuleVersionMap(ctx, app.MM.GetVersionMap())
	if err != nil {
		panic(err)
	}

	return app.MM.InitGenesis(ctx, app.cdc, genesisState)
}

// PreBlocker application updates before each begin block.
func (app *AkashApp) PreBlocker(ctx sdk.Context, _ *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
	// Set gas meter to the free gas meter.
	// This is because there is currently non-deterministic gas usage in the
	// pre-blocker, e.g. due to hydration of in-memory data structures.
	//
	// Note that we don't need to reset the gas meter after the pre-blocker
	// because Go is pass by value.
	ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())

	return app.MM.PreBlock(ctx)
}

// BeginBlocker is a function in which application updates every begin block
func (app *AkashApp) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	if patch, exists := utypes.GetHeightPatchesList()[ctx.BlockHeight()]; exists {
		app.Logger().Info(fmt.Sprintf("found patch %s for current height %d. applying...", patch.Name(), ctx.BlockHeight()))
		patch.Begin(ctx, &app.Keepers)
		app.Logger().Info(fmt.Sprintf("patch %s applied successfully at height %d", patch.Name(), ctx.BlockHeight()))
	}

	return app.MM.BeginBlock(ctx)
}

// EndBlocker is a function in which application updates every end block
func (app *AkashApp) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.MM.EndBlock(ctx)
}

// Precommitter application updates before the commital of a block after all transactions have been delivered.
func (app *AkashApp) Precommitter(ctx sdk.Context) {
	if err := app.MM.Precommit(ctx); err != nil {
		panic(err)
	}
}

func (app *AkashApp) PrepareCheckStater(ctx sdk.Context) {
	if err := app.MM.PrepareCheckState(ctx); err != nil {
		panic(err)
	}
}

// LegacyAmino returns AkashApp's amino codec.
func (app *AkashApp) LegacyAmino() *codec.LegacyAmino {
	return app.aminoCdc
}

// AppCodec returns AkashApp's app codec.
func (app *AkashApp) AppCodec() codec.Codec {
	return app.cdc
}

// TxConfig returns SimApp's TxConfig
func (app *AkashApp) TxConfig() client.TxConfig {
	return app.txConfig
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *AkashApp) ModuleAccountAddrs() map[string]bool {
	return ModuleAccountAddrs()
}

// BlockedAddrs returns all the app's module account addresses that are not
// allowed to receive external tokens.
func (app *AkashApp) BlockedAddrs() map[string]bool {
	perms := ModuleAccountAddrs()
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
	cctx := apiSvr.ClientCtx

	// Register new tx routes from grpc-gateway
	authtx.RegisterGRPCGatewayRoutes(cctx, apiSvr.GRPCGatewayRouter)
	// Register new tendermint queries routes from grpc-gateway.
	cmtservice.RegisterGRPCGatewayRoutes(cctx, apiSvr.GRPCGatewayRouter)

	// Register legacy and grpc-gateway routes for all modules.
	ModuleBasics().RegisterGRPCGatewayRoutes(cctx, apiSvr.GRPCGatewayRouter)

	// Register node gRPC service for grpc-gateway.
	nodeservice.RegisterGRPCGatewayRoutes(cctx, apiSvr.GRPCGatewayRouter)

	// register swagger API from root so that other applications can override easily
	if apiConfig.Swagger {
		RegisterSwaggerAPI(cctx, apiSvr.Router)
	}
}

// RegisterTxService implements the Application.RegisterTxService method.
func (app *AkashApp) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.GRPCQueryRouter(), clientCtx, app.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *AkashApp) RegisterTendermintService(cctx client.Context) {
	cmtservice.RegisterTendermintService(
		cctx,
		app.GRPCQueryRouter(),
		app.interfaceRegistry,
		app.Query)
}

// RegisterNodeService registers the node gRPC Query service.
func (app *AkashApp) RegisterNodeService(cctx client.Context, cfg config.Config) {
	nodeservice.RegisterNodeService(cctx, app.GRPCQueryRouter(), cfg)
}

// RegisterSwaggerAPI registers swagger route with API Server
func RegisterSwaggerAPI(_ client.Context, rtr *mux.Router) {
	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}

	staticServer := http.FileServer(statikFS)
	rtr.PathPrefix("/static/").Handler(http.StripPrefix("/static/", staticServer))
	rtr.PathPrefix("/swagger/").Handler(http.StripPrefix("/swagger/", staticServer))
}

// LoadHeight method of AkashApp loads baseapp application version with given height
func (app *AkashApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// cache the reflectionService to save us time within tests.
var cachedReflectionService *runtimeservices.ReflectionService

func getReflectionService() *runtimeservices.ReflectionService {
	if cachedReflectionService != nil {
		return cachedReflectionService
	}
	reflectionSvc, err := runtimeservices.NewReflectionService()
	if err != nil {
		panic(err)
	}
	cachedReflectionService = reflectionSvc
	return reflectionSvc
}

// NewProposalContext returns a context with a branched version of the state
// that is safe to query during ProcessProposal.
func (app *AkashApp) NewProposalContext(header tmproto.Header) sdk.Context {
	// use custom query multistore if provided
	ms := app.CommitMultiStore().CacheMultiStore()
	ctx := sdk.NewContext(ms, header, false, app.Logger()).
		WithBlockGasMeter(storetypes.NewInfiniteGasMeter()).
		WithBlockHeader(header)
	ctx = ctx.WithConsensusParams(app.GetConsensusParams(ctx))

	return ctx
}
