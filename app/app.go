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
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/mint"

	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
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
	denom   = "akt"
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

		params.AppModuleBasic{},

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
		deployment deployment.Keeper
		market     market.Keeper
		provider   provider.Keeper
	}

	mm *module.Manager
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
	codec.RegisterCrypto(cdc)
	codec.RegisterEvidences(cdc)

	return cdc.Seal()
}

// https://github.com/cosmos/sdk-tutorials/blob/c6754a1e313eb1ed973c5c91dcc606f2fd288811/app.go#L73

// NewApp creates and returns a new Akash App.
func NewApp(
	logger log.Logger, db dbm.DB, tio io.Writer, options ...func(*bam.BaseApp),
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
		deployment.StoreKey,
		market.StoreKey,
		provider.StoreKey,
	)

	tkeys := sdk.NewTransientStoreKeys(params.TStoreKey)

	bapp := bam.NewBaseApp(appName, logger, db, auth.DefaultTxDecoder(cdc), options...)
	bapp.SetCommitMultiStoreTracer(tio)
	bapp.SetAppVersion(version.Version)

	app := &AkashApp{
		BaseApp: bapp,
		cdc:     cdc,
		keys:    keys,
		tkeys:   tkeys,
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
		skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
		macAddrs(),
	)

	app.keeper.slashing = slashing.NewKeeper(
		cdc,
		keys[slashing.StoreKey],
		skeeper,
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
		&app.keeper.staking,
		app.keeper.supply,
		auth.FeeCollectorName,
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

	app.mm.SetOrderBeginBlockers(mint.ModuleName, distr.ModuleName, slashing.ModuleName)
	app.mm.SetOrderEndBlockers(staking.ModuleName, deployment.ModuleName, market.ModuleName)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		distr.ModuleName,
		staking.ModuleName,
		auth.ModuleName,
		bank.ModuleName,
		slashing.ModuleName,
		mint.ModuleName,
		supply.ModuleName,
		genutil.ModuleName,

		// akash
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
	)

	app.mm.RegisterRoutes(app.Router(), app.QueryRouter())

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)

	// initialize BaseApp
	app.SetInitChainer(app.initChainer)
	app.SetBeginBlocker(app.beginBlocker)

	app.SetAnteHandler(
		auth.NewAnteHandler(
			app.keeper.acct,
			app.keeper.supply,
			auth.DefaultSigVerificationGasConsumer,
		),
	)

	app.SetEndBlocker(app.endBlocker)

	err := app.LoadLatestVersion(app.keys[bam.MainStoreKey])
	if err != nil {
		tmos.Exit("app initialization:" + err.Error())
	}

	return app
}

func (app *AkashApp) initChainer(
	ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState simapp.GenesisState
	app.cdc.MustUnmarshalJSON(req.AppStateBytes, &genesisState)

	return app.mm.InitGenesis(ctx, genesisState)
}

// application updates every begin block
func (app *AkashApp) beginBlocker(
	ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	return app.mm.BeginBlock(ctx, req)
}

// application updates every end block
func (app *AkashApp) endBlocker(
	ctx sdk.Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
	return app.mm.EndBlock(ctx, req)
}

// LoadHeight method of AkashApp loads baseapp application version with given height
func (app *AkashApp) LoadHeight(height int64) error {
	return app.LoadVersion(height, app.keys[bam.MainStoreKey])
}

// ExportAppStateAndValidators returns application state json and slice of validators
func (app *AkashApp) ExportAppStateAndValidators(
	forZeroHeight bool, jailWhiteList []string,
) (appState json.RawMessage, validators []tmtypes.GenesisValidator, err error) {

	// as if they could withdraw from the start of the next block
	ctx := app.NewContext(true, abci.Header{Height: app.LastBlockHeight()})

	genState := app.mm.ExportGenesis(ctx)
	appState, err = codec.MarshalJSONIndent(app.cdc, genState)
	if err != nil {
		return nil, nil, err
	}

	validators = staking.WriteValidators(ctx, app.keeper.staking)

	return appState, validators, nil
}

func init() {
	setGenesisDefaults()
}

func setGenesisDefaults() {
	staking.DefaultGenesisState = stakingGenesisState
}

func stakingGenesisState() stakingtypes.GenesisState {
	genesisState := stakingtypes.DefaultGenesisState()
	genesisState.Params.BondDenom = denom
	return genesisState
}
