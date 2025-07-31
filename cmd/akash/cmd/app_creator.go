package cmd

import (
	"errors"
	"io"
	"path/filepath"

	"cosmossdk.io/store"
	"github.com/spf13/cast"
	"github.com/spf13/viper"

	tmtypes "github.com/cometbft/cometbft/types"

	"cosmossdk.io/log"
	"cosmossdk.io/store/snapshots"
	snapshottypes "cosmossdk.io/store/snapshots/types"
	storetypes "cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"

	cflags "pkg.akt.dev/go/cli/flags"
	"pkg.akt.dev/go/sdkutil"

	akash "pkg.akt.dev/node/app"
)

type appCreator struct {
	encCfg sdkutil.EncodingConfig
}

func (a appCreator) newApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts servertypes.AppOptions,
) servertypes.Application {
	var cache storetypes.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(cflags.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(cflags.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := sdkserver.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	homeDir := cast.ToString(appOpts.Get(cflags.FlagHome))
	chainID := cast.ToString(appOpts.Get(cflags.FlagChainID))
	if chainID == "" {
		// fallback to genesis chain-id
		genDocFile := filepath.Join(homeDir, cast.ToString(appOpts.Get("genesis_file")))
		appGenesis, err := tmtypes.GenesisDocFromFile(genDocFile)
		if err != nil {
			panic(err)
		}

		chainID = appGenesis.ChainID
	}

	snapshotDir := filepath.Join(homeDir, "data", "snapshots")
	snapshotDB, err := dbm.NewDB("metadata", sdkserver.GetAppDBBackend(appOpts), snapshotDir)
	if err != nil {
		panic(err)
	}

	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}

	// BaseApp Opts
	snapshotOptions := snapshottypes.NewSnapshotOptions(
		cast.ToUint64(appOpts.Get(cflags.FlagStateSyncSnapshotInterval)),
		cast.ToUint32(appOpts.Get(cflags.FlagStateSyncSnapshotKeepRecent)),
	)

	baseAppOptions := []func(*baseapp.BaseApp){
		baseapp.SetChainID(chainID),
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(cflags.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(cflags.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(cflags.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(cflags.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(cflags.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(cflags.FlagIndexEvents))),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(cast.ToInt(appOpts.Get(cflags.FlagIAVLCacheSize))),
	}

	return akash.NewApp(
		logger, db, traceStore, true, cast.ToUint(appOpts.Get(cflags.FlagInvCheckPeriod)), skipUpgradeHeights,
		a.encCfg,
		appOpts,
		baseAppOptions...,
	)
}

func (a appCreator) appExport(
	logger log.Logger,
	db dbm.DB,
	tio io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	var akashApp *akash.AkashApp

	homePath, ok := appOpts.Get(cflags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home is not set")
	}
	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}
	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(cflags.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts

	if height != -1 {
		akashApp = akash.NewApp(logger, db, tio, false, uint(1), map[int64]bool{}, a.encCfg, appOpts)

		if err := akashApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	} else {
		akashApp = akash.NewApp(logger, db, tio, true, uint(1), map[int64]bool{}, a.encCfg, appOpts)
	}

	return akashApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}

// newTestnetApp starts by running the normal newApp method. From there, the app interface returned is modified in order
// for a testnet to be created from the provided app.
func (a appCreator) newTestnetApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	// Create an app and type cast to an AkashApp
	app := a.newApp(logger, db, traceStore, appOpts)
	akashApp, ok := app.(*akash.AkashApp)
	if !ok {
		panic("app created from newApp is not of type AkashApp")
	}

	tcfg, valid := appOpts.Get(cflags.KeyTestnetConfig).(akash.TestnetConfig)
	if !valid {
		panic("cflags.KeyTestnetConfig is not of type akash.TestnetConfig")
	}

	// Make modifications to the normal AkashApp required to run the network locally
	return akash.InitAkashAppForTestnet(akashApp, db, tcfg)
}
