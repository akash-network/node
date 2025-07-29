package types

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	evidencekeeper "cosmossdk.io/x/evidence/keeper"
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	feegrantkeeper "cosmossdk.io/x/feegrant/keeper"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensusparamkeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	paramproposal "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	capabilitykeeper "github.com/cosmos/ibc-go/modules/capability/keeper"
	capabilitytypes "github.com/cosmos/ibc-go/modules/capability/types"
	icacontrollertypes "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/host/types"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	ibctransferkeeper "github.com/cosmos/ibc-go/v8/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	ibcclient "github.com/cosmos/ibc-go/v8/modules/core/02-client"
	ibcclienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	ibcconnectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	porttypes "github.com/cosmos/ibc-go/v8/modules/core/05-port/types"
	ibcexported "github.com/cosmos/ibc-go/v8/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v8/modules/core/keeper"
	etypes "pkg.akt.dev/go/node/escrow/v1"

	atypes "pkg.akt.dev/go/node/audit/v1"
	ctypes "pkg.akt.dev/go/node/cert/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	agovtypes "pkg.akt.dev/go/node/gov/v1beta3"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	astakingtypes "pkg.akt.dev/go/node/staking/v1beta3"
	ttypes "pkg.akt.dev/go/node/take/v1"
	"pkg.akt.dev/go/sdkutil"

	akeeper "pkg.akt.dev/node/x/audit/keeper"
	ckeeper "pkg.akt.dev/node/x/cert/keeper"
	dkeeper "pkg.akt.dev/node/x/deployment/keeper"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
	mhooks "pkg.akt.dev/node/x/market/hooks"
	mkeeper "pkg.akt.dev/node/x/market/keeper"
	pkeeper "pkg.akt.dev/node/x/provider/keeper"
	tkeeper "pkg.akt.dev/node/x/take/keeper"
)

const (
	AccountAddressPrefix = "akash"
)

var ErrEmptyFieldName = errors.New("empty field name")

type AppKeepers struct {
	Cosmos struct {
		Acct                 authkeeper.AccountKeeper
		Authz                authzkeeper.Keeper
		FeeGrant             feegrantkeeper.Keeper
		Bank                 bankkeeper.Keeper
		Cap                  *capabilitykeeper.Keeper
		Staking              *stakingkeeper.Keeper
		Slashing             slashingkeeper.Keeper
		Mint                 mintkeeper.Keeper
		Distr                distrkeeper.Keeper
		Gov                  *govkeeper.Keeper
		Crisis               *crisiskeeper.Keeper
		Upgrade              *upgradekeeper.Keeper
		Params               paramskeeper.Keeper
		ConsensusParams      *consensusparamkeeper.Keeper
		IBC                  *ibckeeper.Keeper
		Evidence             *evidencekeeper.Keeper
		Transfer             ibctransferkeeper.Keeper
		ScopedIBCKeeper      capabilitykeeper.ScopedKeeper
		ScopedTransferKeeper capabilitykeeper.ScopedKeeper
	}

	Akash struct {
		Escrow     ekeeper.Keeper
		Deployment dkeeper.IKeeper
		Take       tkeeper.IKeeper
		Market     mkeeper.IKeeper
		Provider   pkeeper.IKeeper
		Audit      akeeper.Keeper
		Cert       ckeeper.Keeper
	}
}

type App struct {
	Keepers      AppKeepers
	Configurator module.Configurator
	MM           *module.Manager
	Log          log.Logger

	// keys to access the substores
	kOnce   sync.Once
	keys    map[string]*storetypes.KVStoreKey
	tkeys   map[string]*storetypes.TransientStoreKey
	memKeys map[string]*storetypes.MemoryStoreKey
}

func (app *App) GenerateKeys() {
	// Define what keys will be used in the cosmos-sdk key/value store.
	// Cosmos-SDK modules each have a "key" that allows the application to reference what they've stored on the chain.
	app.kOnce.Do(func() {
		app.keys = storetypes.NewKVStoreKeys(kvStoreKeys()...)

		// Define transient store keys
		app.tkeys = storetypes.NewTransientStoreKeys(transientStoreKeys()...)

		// MemKeys are for information that is stored only in RAM.
		app.memKeys = storetypes.NewMemoryStoreKeys(memStoreKeys()...)
	})
}

// GetSubspace gets existing substore from keeper.
func (app *App) GetSubspace(moduleName string) paramstypes.Subspace {
	subspace, found := app.Keepers.Cosmos.Params.GetSubspace(moduleName)
	if !found {
		panic(fmt.Sprintf("params subspace \"%s\" not found", moduleName))
	}
	return subspace
}

// GetKVStoreKey gets KV Store keys.
func (app *App) GetKVStoreKey() map[string]*storetypes.KVStoreKey {
	return app.keys
}

// GetTransientStoreKey gets Transient Store keys.
func (app *App) GetTransientStoreKey() map[string]*storetypes.TransientStoreKey {
	return app.tkeys
}

// GetMemoryStoreKey get memory Store keys.
func (app *App) GetMemoryStoreKey() map[string]*storetypes.MemoryStoreKey {
	return app.memKeys
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *App) GetKey(storeKey string) *storetypes.KVStoreKey {
	return app.keys[storeKey]
}

// GetTKey returns the TransientStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *App) GetTKey(storeKey string) *storetypes.TransientStoreKey {
	return app.tkeys[storeKey]
}

// GetMemKey returns the MemStoreKey for the provided mem key.
//
// NOTE: This is solely used for testing purposes.
func (app *App) GetMemKey(storeKey string) *storetypes.MemoryStoreKey {
	return app.memKeys[storeKey]
}

// InitSpecialKeepers initiates special keepers (crisis appkeeper, upgradekeeper, params keeper)
func (app *App) InitSpecialKeepers(
	cdc codec.Codec,
	legacyAmino *codec.LegacyAmino,
	bApp *baseapp.BaseApp,
	invCheckPeriod uint,
	skipUpgradeHeights map[int64]bool,
	homePath string) {

	app.GenerateKeys()

	app.Keepers.Cosmos.Params = initParamsKeeper(
		cdc,
		legacyAmino,
		app.keys[paramstypes.StoreKey],
		app.tkeys[paramstypes.TStoreKey],
	)

	// set the BaseApp's parameter store
	{
		keeper := consensusparamkeeper.NewKeeper(
			cdc,
			runtime.NewKVStoreService(app.keys[consensusparamtypes.StoreKey]),
			authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			runtime.EventService{},
		)

		app.Keepers.Cosmos.ConsensusParams = &keeper
	}

	bApp.SetParamStore(app.Keepers.Cosmos.ConsensusParams.ParamsStore)

	// add capability keeper and ScopeToModule for ibc module
	app.Keepers.Cosmos.Cap = capabilitykeeper.NewKeeper(
		cdc,
		app.keys[capabilitytypes.StoreKey],
		app.memKeys[capabilitytypes.MemStoreKey],
	)

	app.Keepers.Cosmos.ScopedIBCKeeper = app.Keepers.Cosmos.Cap.ScopeToModule(ibcexported.ModuleName)
	app.Keepers.Cosmos.ScopedTransferKeeper = app.Keepers.Cosmos.Cap.ScopeToModule(ibctransfertypes.ModuleName)

	// seal the capability keeper so all persistent capabilities are loaded in-memory and prevent
	// any further modules from creating scoped sub-keepers.
	app.Keepers.Cosmos.Cap.Seal()

	app.Keepers.Cosmos.Crisis = crisiskeeper.NewKeeper(
		cdc, runtime.NewKVStoreService(app.keys[crisistypes.StoreKey]),
		invCheckPeriod,
		app.Keepers.Cosmos.Bank,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
	)

	app.Keepers.Cosmos.Upgrade = upgradekeeper.NewKeeper(
		skipUpgradeHeights,
		runtime.NewKVStoreService(app.GetKey(upgradetypes.StoreKey)),
		cdc,
		homePath,
		bApp,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
}

func (app *App) InitNormalKeepers(
	cdc codec.Codec,
	encodingConfig sdkutil.EncodingConfig,
	bApp *baseapp.BaseApp,
	maccPerms map[string][]string,
	blockedAddresses map[string]bool) {

	legacyAmino := encodingConfig.Amino

	app.Keepers.Cosmos.Acct = authkeeper.NewAccountKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		AccountAddressPrefix,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.Keepers.Cosmos.Bank = bankkeeper.NewBaseKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[banktypes.StoreKey]),
		app.Keepers.Cosmos.Acct,
		blockedAddresses,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.Log,
	)

	// add authz keeper
	app.Keepers.Cosmos.Authz = authzkeeper.NewKeeper(
		runtime.NewKVStoreService(app.keys[authzkeeper.StoreKey]),
		cdc,
		bApp.MsgServiceRouter(),
		app.Keepers.Cosmos.Acct,
	)

	app.Keepers.Cosmos.Staking = stakingkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[stakingtypes.StoreKey]),
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	)

	app.Keepers.Cosmos.Distr = distrkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[distrtypes.StoreKey]),
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		app.Keepers.Cosmos.Staking,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.Keepers.Cosmos.Slashing = slashingkeeper.NewKeeper(
		cdc,
		legacyAmino,
		runtime.NewKVStoreService(app.keys[slashingtypes.StoreKey]),
		app.Keepers.Cosmos.Staking,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// register IBC Keeper
	app.Keepers.Cosmos.IBC = ibckeeper.NewKeeper(
		cdc,
		app.keys[ibcexported.StoreKey],
		app.GetSubspace(ibcexported.ModuleName),
		app.Keepers.Cosmos.Staking,
		app.Keepers.Cosmos.Upgrade,
		app.Keepers.Cosmos.ScopedIBCKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// create evidence keeper with evidence router
	app.Keepers.Cosmos.Evidence = evidencekeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[evidencetypes.StoreKey]),
		app.Keepers.Cosmos.Staking,
		app.Keepers.Cosmos.Slashing,
		addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		runtime.ProvideCometInfoService(),
	)

	app.Keepers.Cosmos.Mint = mintkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[minttypes.StoreKey]),
		app.Keepers.Cosmos.Staking,
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// register the proposal types
	govRouter := govtypesv1.NewRouter()
	govRouter.
		AddRoute(
			govtypes.RouterKey,
			govtypesv1.ProposalHandler,
		).
		AddRoute(
			paramproposal.RouterKey,
			params.NewParamChangeProposalHandler(app.Keepers.Cosmos.Params),
		).
		//AddRoute(
		//	upgradetypes.RouterKey,
		//	upgrade.NewSoftwareUpgradeProposalHandler(app.Keepers.Cosmos.Upgrade),
		//).
		AddRoute(
			ibcclienttypes.RouterKey,
			ibcclient.NewClientProposalHandler(app.Keepers.Cosmos.IBC.ClientKeeper),
		).
		AddRoute(
			ibcexported.RouterKey,
			ibcclient.NewClientProposalHandler(app.Keepers.Cosmos.IBC.ClientKeeper),
		)
	// AddRoute(
	// 	astakingtypes.RouterKey,
	// 	ibcclient.NewClientProposalHandler(app.Keepers.Cosmos.IBC.ClientKeeper),
	// )

	govConfig := govtypes.DefaultConfig()
	// Set the maximum metadata length for government-related configurations to 10,200, deviating from the default value of 256.
	govConfig.MaxMetadataLen = 10200

	app.Keepers.Cosmos.Gov = govkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[govtypes.StoreKey]),
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		app.Keepers.Cosmos.Staking,
		app.Keepers.Cosmos.Distr,
		bApp.MsgServiceRouter(),
		govConfig,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.Keepers.Cosmos.Gov.SetLegacyRouter(govRouter)

	app.Keepers.Cosmos.FeeGrant = feegrantkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(app.keys[feegrant.StoreKey]),
		app.Keepers.Cosmos.Acct,
	)

	// register Transfer Keepers
	app.Keepers.Cosmos.Transfer = ibctransferkeeper.NewKeeper(
		cdc,
		app.keys[ibctransfertypes.StoreKey],
		app.GetSubspace(ibctransfertypes.ModuleName),
		app.Keepers.Cosmos.IBC.ChannelKeeper,
		app.Keepers.Cosmos.IBC.ChannelKeeper,
		app.Keepers.Cosmos.IBC.PortKeeper,
		app.Keepers.Cosmos.Acct,
		app.Keepers.Cosmos.Bank,
		app.Keepers.Cosmos.ScopedTransferKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	transferIBCModule := transfer.NewIBCModule(app.Keepers.Cosmos.Transfer)

	// Create static IBC router, add transfer route, then set and seal it
	ibcRouter := porttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, transferIBCModule)

	app.Keepers.Cosmos.IBC.SetRouter(ibcRouter)

	// initialize the auction keeper
	//{
	//
	//	auctionKeeper := auctionkeeper.NewKeeper(
	//		cdc,
	//		app.keys[auctiontypes.StoreKey],
	//		app.Keepers.Cosmos.Acct,
	//		app.Keepers.Cosmos.Bank,
	//		app.Keepers.Cosmos.Distr,
	//		app.Keepers.Cosmos.Staking,
	//		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	//	)
	//	app.Keepers.External.Auction = &auctionKeeper
	//}

	app.Keepers.Akash.Take = tkeeper.NewKeeper(
		cdc,
		app.keys[ttypes.StoreKey],
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.Keepers.Akash.Escrow = ekeeper.NewKeeper(
		cdc,
		app.keys[etypes.StoreKey],
		app.Keepers.Cosmos.Bank,
		app.Keepers.Akash.Take,
		app.Keepers.Cosmos.Authz,
		app.Keepers.Cosmos.Distr.FeePool,
	)

	app.Keepers.Akash.Deployment = dkeeper.NewKeeper(
		cdc,
		app.keys[dtypes.StoreKey],
		app.Keepers.Akash.Escrow,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.Keepers.Akash.Market = mkeeper.NewKeeper(
		cdc,
		app.keys[mtypes.StoreKey],
		app.Keepers.Akash.Escrow,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.Keepers.Akash.Provider = pkeeper.NewKeeper(
		cdc,
		app.keys[ptypes.StoreKey],
	)

	app.Keepers.Akash.Audit = akeeper.NewKeeper(
		cdc,
		app.keys[atypes.StoreKey],
	)

	app.Keepers.Akash.Cert = ckeeper.NewKeeper(
		cdc,
		app.keys[ctypes.StoreKey],
	)
}

func (app *App) SetupHooks() {
	// register the staking hooks
	app.Keepers.Cosmos.Staking.SetHooks(
		stakingtypes.NewMultiStakingHooks(
			app.Keepers.Cosmos.Distr.Hooks(),
			app.Keepers.Cosmos.Slashing.Hooks(),
		),
	)

	app.Keepers.Cosmos.Gov.SetHooks(
		govtypes.NewMultiGovHooks(
		// insert governance hooks receivers here
		),
	)

	hook := mhooks.New(
		app.Keepers.Akash.Deployment,
		app.Keepers.Akash.Market,
	)

	app.Keepers.Akash.Escrow.AddOnAccountClosedHook(hook.OnEscrowAccountClosed)
	app.Keepers.Akash.Escrow.AddOnPaymentClosedHook(hook.OnEscrowPaymentClosed)
}

// initParamsKeeper init params keeper and its subspaces
func initParamsKeeper(appCodec codec.BinaryCodec, legacyAmino *codec.LegacyAmino, key, tkey storetypes.StoreKey) paramskeeper.Keeper {
	paramsKeeper := paramskeeper.NewKeeper(appCodec, legacyAmino, key, tkey)

	ibctable := ibcclienttypes.ParamKeyTable()
	ibctable.RegisterParamSet(&ibcconnectiontypes.Params{})

	paramsKeeper.Subspace(authtypes.ModuleName).WithKeyTable(authtypes.ParamKeyTable())         // nolint: staticcheck
	paramsKeeper.Subspace(banktypes.ModuleName).WithKeyTable(banktypes.ParamKeyTable())         // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(stakingtypes.ModuleName).WithKeyTable(stakingtypes.ParamKeyTable())   // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(minttypes.ModuleName).WithKeyTable(minttypes.ParamKeyTable())         // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(distrtypes.ModuleName).WithKeyTable(distrtypes.ParamKeyTable())       // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(slashingtypes.ModuleName).WithKeyTable(slashingtypes.ParamKeyTable()) // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(govtypes.ModuleName).WithKeyTable(govv1.ParamKeyTable())              // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(crisistypes.ModuleName).WithKeyTable(crisistypes.ParamKeyTable())     // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(ibctransfertypes.ModuleName).WithKeyTable(ibctransfertypes.ParamKeyTable())
	paramsKeeper.Subspace(ibcexported.ModuleName).WithKeyTable(ibctable)
	paramsKeeper.Subspace(icacontrollertypes.SubModuleName)
	paramsKeeper.Subspace(icahosttypes.SubModuleName)

	// akash params subspaces
	paramsKeeper.Subspace(dtypes.ModuleName)
	paramsKeeper.Subspace(mtypes.ModuleName)
	paramsKeeper.Subspace(astakingtypes.ModuleName).WithKeyTable(astakingtypes.ParamKeyTable()) // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(agovtypes.ModuleName).WithKeyTable(agovtypes.ParamKeyTable())         // nolint: staticcheck // SA1019
	paramsKeeper.Subspace(ttypes.ModuleName).WithKeyTable(ttypes.ParamKeyTable())               // nolint: staticcheck // SA1019

	return paramsKeeper
}

func kvStoreKeys() []string {
	keys := []string{
		consensusparamtypes.StoreKey,
		authtypes.StoreKey,
		feegrant.StoreKey,
		authzkeeper.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		minttypes.StoreKey,
		distrtypes.StoreKey,
		slashingtypes.StoreKey,
		govtypes.StoreKey,
		paramstypes.StoreKey,
		ibcexported.StoreKey,
		upgradetypes.StoreKey,
		evidencetypes.StoreKey,
		ibctransfertypes.StoreKey,
		capabilitytypes.StoreKey,
	}

	keys = append(keys, akashKVStoreKeys()...,
	)
	return keys
}

func akashKVStoreKeys() []string {
	return []string{
		ttypes.StoreKey,
		etypes.StoreKey,
		dtypes.StoreKey,
		mtypes.StoreKey,
		ptypes.StoreKey,
		atypes.StoreKey,
		ctypes.StoreKey,
		astakingtypes.StoreKey,
	}
}

func transientStoreKeys() []string {
	return []string{
		paramstypes.TStoreKey,
	}
}

func memStoreKeys() []string {
	return []string{
		capabilitytypes.MemStoreKey,
	}
}

// FindStructField if an interface is either a struct or a pointer to a struct
// and has the defined member field, if error is nil, the given
// fieldName exists and is accessible with reflect.
func FindStructField[C any](obj interface{}, fieldName string) (C, error) {
	if fieldName == "" {
		return *new(C), ErrEmptyFieldName
	}
	rValue := reflect.ValueOf(obj)

	if rValue.Type().Kind() != reflect.Ptr {
		pValue := reflect.New(reflect.TypeOf(obj))
		pValue.Elem().Set(rValue)
		rValue = pValue
	}

	field := rValue.Elem().FieldByName(fieldName)
	if !field.IsValid() {
		return *new(C), fmt.Errorf("interface `%s` does not have the field `%s`", // nolint: goerr113
			rValue.Type(),
			fieldName)
	}

	res, valid := field.Interface().(C)
	if !valid {
		return *new(C), fmt.Errorf( // nolint: goerr113
			"object's `%s` expected type `%s` does not match actual `%s`",
			fieldName,
			reflect.TypeOf(*new(C)), field.Type().String())
	}

	return res, nil
}
