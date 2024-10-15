package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"time"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	reflectionv1 "cosmossdk.io/api/cosmos/reflection/v1"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v7/modules/core/exported"
	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"
	"github.com/skip-mev/block-sdk/block"
	"github.com/skip-mev/block-sdk/block/base"
	"github.com/spf13/cast"
	audittypes "pkg.akt.dev/go/node/audit/v1"
	certtypes "pkg.akt.dev/go/node/cert/v1"
	deploymenttypes "pkg.akt.dev/go/node/deployment/v1"
	escrowtypes "pkg.akt.dev/go/node/escrow/v1"
	inflationtypes "pkg.akt.dev/go/node/inflation/v1beta3"
	markettypes "pkg.akt.dev/go/node/market/v1beta5"
	providertypes "pkg.akt.dev/go/node/provider/v1beta4"
	taketypes "pkg.akt.dev/go/node/take/v1"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	cmos "github.com/cometbft/cometbft/libs/os"
	"github.com/skip-mev/block-sdk/abci/checktx"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
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
	"github.com/cosmos/cosmos-sdk/x/crisis"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/ibc-go/v7/testing/simapp"

	cflags "pkg.akt.dev/go/cli/flags"

	"pkg.akt.dev/node/app/params"
	apptypes "pkg.akt.dev/node/app/types"
	utypes "pkg.akt.dev/node/upgrades/types"
	agov "pkg.akt.dev/node/x/gov"
	astaking "pkg.akt.dev/node/x/staking"

	// unnamed import of statik for swagger UI support
	_ "pkg.akt.dev/node/client/docs/statik"
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
	checkTxHandler    checktx.CheckTx
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
	encodingConfig params.EncodingConfig,
	appOpts servertypes.AppOptions,
	options ...func(*baseapp.BaseApp),
) *AkashApp {
	// find out the genesis time, to be used later in inflation calculation
	// genesisTime := getGenesisTime(appOpts, homePath)

	appCodec := encodingConfig.Marshaler
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

	app := &AkashApp{
		BaseApp:           bapp,
		App:               &apptypes.App{},
		aminoCdc:          aminoCdc,
		cdc:               appCodec,
		txConfig:          txConfig,
		interfaceRegistry: interfaceRegistry,
		invCheckPeriod:    invCheckPeriod,
	}

	app.InitSpecialKeepers(
		app.cdc,
		aminoCdc,
		app.BaseApp,
		invCheckPeriod,
		skipUpgradeHeights,
		homePath,
	)

	// register the upgrade handler
	if err := app.registerUpgradeHandlers(); err != nil {
		panic(err)
	}

	app.InitNormalKeepers(
		app.cdc,
		encodingConfig,
		app.BaseApp,
		ModuleAccountPerms(),
		app.BlockedAddrs(),
	)

	// TODO: There is a bug here, where we register the govRouter routes in InitNormalKeepers and then
	// call setupHooks afterwards. Therefore, if a gov proposal needs to call a method and that method calls a
	// hook, we will get a nil pointer dereference error due to the hooks in the keeper not being
	// setup yet. I will refrain from creating an issue in the sdk for now until after we unfork to 0.47,
	// because I believe the concept of Routes is going away.
	app.SetupHooks()

	// NOTE: we may consider parsing `appOpts` inside module constructors. For the moment
	// we prefer to be more strict in what arguments the modules expect.
	skipGenesisInvariants := cast.ToBool(appOpts.Get(crisis.FlagSkipGenesisInvariants))

	// NOTE: All module / keeper changes should happen prior to this module.NewManager line being called.
	// However, in the event any changes do need to happen after this call, ensure that that keeper
	// is only passed in its keeper form (not de-ref'd anywhere)
	//
	// Generally NewAppModule will require the keeper that module defines to be passed in as an exact struct,
	// but should take in every other keeper as long as it matches a certain interface. (So no need to be de-ref'd)
	//
	// Any time a module requires a keeper de-ref'd that's not its native one,
	// its code-smell and should probably change. We should get the staking keeper dependencies fixed.
	app.MM = module.NewManager(appModules(app, encodingConfig, skipGenesisInvariants)...)

	// During begin block slashing happens after distr.BeginBlocker so that
	// there is nothing left over in the validator fee pool, to keep the
	// CanWithdrawInvariant invariant.
	// NOTE: staking module is required if HistoricalEntries param > 0
	// NOTE: capability module's begin-blocker must come before any modules using capabilities (e.g. IBC)

	// Tell the app's module manager how to set the order of BeginBlockers, which are run at the beginning of every block.
	app.MM.SetOrderBeginBlockers(orderBeginBlockers(app.MM.ModuleNames())...)

	// Tell the app's module manager how to set the order of EndBlockers, which are run at the end of every block.
	app.MM.SetOrderEndBlockers(OrderEndBlockers(app.MM.ModuleNames())...)

	app.MM.SetOrderInitGenesis(OrderInitGenesis(app.MM.ModuleNames())...)

	app.MM.RegisterInvariants(app.Keepers.Cosmos.Crisis)

	app.Configurator = module.NewConfigurator(app.AppCodec(), app.MsgServiceRouter(), app.GRPCQueryRouter())
	app.MM.RegisterServices(app.Configurator)

	app.sm = module.NewSimulationManager(appSimModules(app)...)
	app.sm.RegisterStoreDecoders()

	autocliv1.RegisterQueryServer(app.GRPCQueryRouter(), runtimeservices.NewAutoCLIQueryService(app.MM.Modules))

	reflectionSvc := getReflectionService()
	reflectionv1.RegisterReflectionServiceServer(app.GRPCQueryRouter(), reflectionSvc)

	// initialize lanes + mempool
	mevLane, defaultLane := CreateLanes(app, txConfig)

	// create the mempool
	lanedMempool, err := block.NewLanedMempool(
		app.Logger(),
		[]block.Lane{mevLane, defaultLane},
	)
	if err != nil {
		panic(err)
	}

	// set the mempool
	app.SetMempool(lanedMempool)

	// initialize stores
	app.MountKVStores(app.GetKVStoreKey())
	app.MountTransientStores(app.GetTransientStoreKey())
	app.MountMemoryStores(app.GetMemoryStoreKey())

	anteOpts := HandlerOptions{
		HandlerOptions: ante.HandlerOptions{
			AccountKeeper:   app.Keepers.Cosmos.Acct,
			BankKeeper:      app.Keepers.Cosmos.Bank,
			FeegrantKeeper:  app.Keepers.Cosmos.FeeGrant,
			SignModeHandler: encodingConfig.TxConfig.SignModeHandler(),
			SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
		},
		CDC: app.cdc,
		// AStakingKeeper: app.Keepers.Akash.Staking,
		GovKeeper: app.Keepers.Cosmos.Gov,
		// AGovKeeper:     app.Keepers.Akash.Gov,
		// BlockSDK: BlockSDKAnteHandlerParams{
		// 	mevLane:       mevLane,
		// 	auctionKeeper: *app.Keepers.External.Auction,
		// 	txConfig:      txConfig,
		// },
	}

	anteHandler, err := NewAnteHandler(anteOpts)
	if err != nil {
		panic(err)
	}

	// update ante-handlers on lanes
	opt := []base.LaneOption{
		base.WithAnteHandler(anteHandler),
	}

	mevLane.WithOptions(opt...)
	defaultLane.WithOptions(opt...)

	// check-tx
	mevCheckTxHandler := checktx.NewMEVCheckTxHandler(
		app,
		txConfig.TxDecoder(),
		mevLane,
		anteHandler,
		app.BaseApp.CheckTx,
		app.ChainID(),
	)

	// wrap checkTxHandler with mempool parity handler
	parityCheckTx := checktx.NewMempoolParityCheckTx(
		app.Logger(),
		lanedMempool,
		txConfig.TxDecoder(),
		mevCheckTxHandler.CheckTx(),
	)

	app.SetCheckTx(parityCheckTx.CheckTx())

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)

	app.SetAnteHandler(anteHandler)
	app.SetEndBlocker(app.EndBlocker)

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
	// ord := partialord.NewPartialOrdering(allModuleNames)
	// // Upgrades should be run VERY first
	// ord.FirstElements(
	// 	upgradetypes.ModuleName,
	// 	capabilitytypes.ModuleName,
	// )
	//
	// // Staking ordering
	// // TODO: Perhaps this can be relaxed, left to future work to analyze.
	// ord.Sequence(
	// 	banktypes.ModuleName,
	// 	paramstypes.ModuleName,
	// 	govtypes.ModuleName,
	// 	minttypes.ModuleName,
	// 	distrtypes.ModuleName,
	// 	slashingtypes.ModuleName,
	// 	evidencetypes.ModuleName,
	// 	stakingtypes.ModuleName,
	// )
	// // TODO: This can almost certainly be un-constrained, but we keep the constraint to match prior functionality.
	// // IBChost came after staking.
	// ord.Sequence(
	// 	stakingtypes.ModuleName,
	// 	ibchost.ModuleName,
	// 	feegrant.ModuleName,
	// )
	//
	//
	// // We leave downtime-detector un-constrained.
	// // every remaining module's begin block is a no-op.
	// return ord.TotalOrdering()
}

// OrderEndBlockers returns EndBlockers (crisis, govtypes, staking) with no relative order.
func OrderEndBlockers(_ []string) []string {
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
	// ord := partialord.NewPartialOrdering(allModuleNames)
	//
	// // Staking must be after gov.
	// ord.FirstElements(govtypes.ModuleName)
	// ord.LastElements(stakingtypes.ModuleName)
	//
	// // only Akash modules with endblock code are: twap, crisis, govtypes, staking
	// // we don't care about the relative ordering between them.
	// return ord.TotalOrdering()
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
	cfg := MakeEncodingConfig()
	return cfg.Marshaler, cfg.Amino
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
	return app.MM.InitGenesis(ctx, app.cdc, genesisState)
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
	tmservice.RegisterGRPCGatewayRoutes(cctx, apiSvr.GRPCGatewayRouter)

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
	authtx.RegisterTxService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.BaseApp.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *AkashApp) RegisterTendermintService(cctx client.Context) {
	tmservice.RegisterTendermintService(
		cctx,
		app.BaseApp.GRPCQueryRouter(),
		app.interfaceRegistry,
		app.Query)
}

// RegisterNodeService registers the node gRPC Query service.
func (app *AkashApp) RegisterNodeService(clientCtx client.Context) {
	nodeservice.RegisterNodeService(clientCtx, app.GRPCQueryRouter())
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

// CheckTx will check the transaction with the provided checkTxHandler. We override the default
// handler so that we can verify bid transactions before they are inserted into the mempool.
// With the BlockSDK CheckTx, we can verify the bid transaction and all of the bundled transactions
// before inserting the bid transaction into the mempool.
func (app *AkashApp) CheckTx(req abci.RequestCheckTx) abci.ResponseCheckTx {
	return app.checkTxHandler(req)
}

// SetCheckTx sets the checkTxHandler for the app.
func (app *AkashApp) SetCheckTx(handler checktx.CheckTx) {
	app.checkTxHandler = handler
}

// ChainID gets chainID from private fields of BaseApp
// Should be removed once SDK 0.50.x will be adopted
func (app *AkashApp) ChainID() string {
	field := reflect.ValueOf(app.BaseApp).Elem().FieldByName("chainID")
	return field.String()
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
