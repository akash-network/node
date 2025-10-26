package app_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdksim "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzkeys "github.com/cosmos/cosmos-sdk/x/authz/keeper/keys"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v10/modules/core/exported"

	atypes "pkg.akt.dev/go/node/audit/v1"
	ctypes "pkg.akt.dev/go/node/cert/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	taketypes "pkg.akt.dev/go/node/take/v1"
	"pkg.akt.dev/go/sdkutil"

	akash "pkg.akt.dev/node/app"
	"pkg.akt.dev/node/app/sim"
	simtestutil "pkg.akt.dev/node/testutil/sims"
)

// AppChainID hardcoded chainID for simulation
const (
	AppChainID = "akash-sim"
)

type storeKeyGetter interface {
	GetKey(string) *storetypes.KVStoreKey
}

type StoreKeysPrefixes struct {
	Key      string
	A        storeKeyGetter
	B        storeKeyGetter
	Prefixes [][]byte
}

func init() {
	sim.GetSimulatorFlags()
}

// fauxMerkleModeOpt returns a BaseApp option to use a dbStoreAdapter instead of
// an IAVLStore for faster simulation speed.
func fauxMerkleModeOpt(bapp *baseapp.BaseApp) {
	bapp.SetFauxMerkleMode()
}

// interBlockCacheOpt returns a BaseApp option function that sets the persistent
// inter-block write-through cache.
func interBlockCacheOpt() func(*baseapp.BaseApp) {
	return baseapp.SetInterBlockCache(store.NewCommitKVStoreCacheManager())
}

func simulateFromSeedFunc(t *testing.T, newApp *akash.AkashApp, config sdksim.Config) (bool, simulation.Params, error) {
	return simulation.SimulateFromSeed(
		t,
		os.Stdout,
		newApp.BaseApp,
		simtestutil.AppStateFn(newApp.AppCodec(), newApp.SimulationManager(), akash.NewDefaultGenesisState(newApp.AppCodec())),
		sdksim.RandomAccounts, // Replace it with own random account function if using keys other than secp256k1
		simtestutil.BuildSimulationOperations(newApp, newApp.AppCodec(), config, newApp.TxConfig()),
		newApp.ModuleAccountAddrs(),
		config,
		newApp.AppCodec(),
	)
}

func TestFullAppSimulation(t *testing.T) {
	config, db, dir, logger, skip, err := sim.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application simulation")
	}

	require.NoError(t, err, "simulation setup failed")

	encodingConfig := sdkutil.MakeEncodingConfig()

	akash.ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	defer func() {
		_ = db.Close()
		require.NoError(t, os.RemoveAll(dir))
	}()

	appOpts := viper.New()
	appOpts.Set("home", akash.DefaultHome)

	r := rand.New(rand.NewSource(config.Seed)) // nolint: gosec
	genTime := sdksim.RandTimestamp(r)

	appOpts.Set("GenesisTime", genTime)

	app1 := akash.NewApp(logger, db, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))

	require.Equal(t, "akash", app1.Name())

	fmt.Printf("config--------\n%v", config)
	// run randomized simulation
	_, simParams, simErr := simulateFromSeedFunc(t, app1, config)

	// export state and simParams before the simulation error is checked
	err = simtestutil.CheckExportSimulation(app1, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		sim.PrintStats(db)
	}
}

func TestAppImportExport(t *testing.T) {
	config, db, dir, logger, skip, err := sim.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application import/export simulation")
	}
	require.NoError(t, err, "simulation setup failed")

	defer func() {
		_ = db.Close()
		require.NoError(t, os.RemoveAll(dir))
	}()

	encodingConfig := sdkutil.MakeEncodingConfig()

	akash.ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	appOpts := viper.New()
	appOpts.Set("home", akash.DefaultHome)

	r := rand.New(rand.NewSource(config.Seed)) // nolint: gosec
	genTime := sdksim.RandTimestamp(r)

	appOpts.Set("GenesisTime", genTime)

	appA := akash.NewApp(logger, db, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))
	require.Equal(t, akash.AppName, appA.Name())

	// Run randomized simulation
	_, simParams, simErr := simulateFromSeedFunc(t, appA, config)

	// export state and simParams before the simulation error is checked
	err = simtestutil.CheckExportSimulation(appA, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		sim.PrintStats(db)
	}

	fmt.Printf("exporting genesis...\n")

	exported, err := appA.ExportAppStateAndValidators(false, []string{}, []string{})
	require.NoError(t, err)

	fmt.Printf("importing genesis...\n")

	_, newDB, newDir, _, _, err := sim.SetupSimulation("leveldb-app-sim-2", "Simulation-2")
	require.NoError(t, err, "simulation setup failed")

	defer func() {
		_ = newDB.Close()
		require.NoError(t, os.RemoveAll(newDir))
	}()

	appB := akash.NewApp(logger, newDB, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))
	require.Equal(t, akash.AppName, appB.Name())

	var genesisState akash.GenesisState
	err = json.Unmarshal(exported.AppState, &genesisState)
	require.NoError(t, err)

	ctxA := appA.NewContext(true)
	ctxB := appB.NewContext(true)

	_, err = appB.MM.InitGenesis(ctxB, appA.AppCodec(), genesisState)
	require.NoError(t, err)

	err = appB.StoreConsensusParams(ctxB, exported.ConsensusParams)
	require.NoError(t, err)

	fmt.Printf("comparing stores...\n")

	storeKeysPrefixes := []StoreKeysPrefixes{
		{
			consensustypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			authtypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			feegrant.StoreKey,
			appA,
			appB,
			[][]byte{
				feegrant.FeeAllowanceQueueKeyPrefix,
			},
		},
		{
			authzkeeper.StoreKey,
			appA,
			appB,
			[][]byte{
				authzkeys.GrantQueuePrefix,
				authzkeys.GranteeGranterKey,
				authzkeys.GranteeMsgTypeUrlKey,
			},
		},
		{
			banktypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			stakingtypes.StoreKey,
			appA,
			appB,
			[][]byte{
				stakingtypes.UnbondingQueueKey,
				stakingtypes.RedelegationQueueKey,
				stakingtypes.ValidatorQueueKey,
				stakingtypes.UnbondingIDKey,
				stakingtypes.UnbondingIndexKey,
				stakingtypes.UnbondingTypeKey,
				stakingtypes.ValidatorUpdatesKey,
				stakingtypes.HistoricalInfoKey,
			},
		},
		{
			minttypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			distrtypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			slashingtypes.StoreKey,
			appA,
			appB,
			[][]byte{
				slashingtypes.ValidatorMissedBlockBitmapKeyPrefix,
			},
		},
		{
			govtypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			paramtypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			ibchost.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			ibctransfertypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			upgradetypes.StoreKey,
			appA,
			appB,
			[][]byte{
				{upgradetypes.DoneByte},
				{upgradetypes.VersionMapByte},
				{upgradetypes.ProtocolVersionByte},
			},
		},
		{
			evidencetypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			atypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			ctypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			dtypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			mtypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			ptypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
		{
			taketypes.StoreKey,
			appA,
			appB,
			[][]byte{},
		},
	}

	for _, skp := range storeKeysPrefixes {
		storeKeyA := skp.A.GetKey(skp.Key)
		storeKeyB := skp.B.GetKey(skp.Key)

		storeA := ctxA.KVStore(storeKeyA)
		storeB := ctxB.KVStore(storeKeyB)

		failedKVAs, failedKVBs := simtestutil.DiffKVStores(storeA, storeB, skp.Prefixes)
		require.Equal(t, len(failedKVAs), len(failedKVBs), "unequal sets of key-values to compare %s, key stores %s and %s", skp.Key, storeKeyA, storeKeyB)

		t.Logf("compared %d different key/value pairs between %s and %s\n", len(failedKVAs), storeKeyA, storeKeyB)
		if !assert.Equal(t, 0, len(failedKVAs), simtestutil.GetSimulationLog(skp.Key, appA.SimulationManager().StoreDecoders, failedKVAs, failedKVBs)) {
			for _, v := range failedKVAs {
				t.Logf("store mismatch: %q\n", v)
			}
			t.FailNow()
		}
	}
}

func TestAppSimulationAfterImport(t *testing.T) {
	config, db, dir, logger, skip, err := sim.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application simulation after import")
	}

	require.NoError(t, err, "simulation setup failed")

	defer func() {
		_ = db.Close()
		require.NoError(t, os.RemoveAll(dir))
	}()

	encodingConfig := sdkutil.MakeEncodingConfig()
	akash.ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	appOpts := viper.New()

	appOpts.Set("home", akash.DefaultHome)

	r := rand.New(rand.NewSource(config.Seed)) // nolint: gosec
	genTime := sdksim.RandTimestamp(r)

	appOpts.Set("GenesisTime", genTime)

	app := akash.NewApp(logger, db, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))
	require.Equal(t, akash.AppName, app.Name())

	// Run randomized simulation
	stopEarly, simParams, simErr := simulateFromSeedFunc(t, app, config)

	// export state and simParams before the simulation error is checked
	err = simtestutil.CheckExportSimulation(app, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		sim.PrintStats(db)
	}

	if stopEarly {
		fmt.Println("can't export or import a zero-validator genesis, exiting test...")
		return
	}

	fmt.Printf("exporting genesis...\n")

	exported, err := app.ExportAppStateAndValidators(true, []string{}, []string{})
	require.NoError(t, err)

	fmt.Printf("importing genesis...\n")

	_, newDB, newDir, _, val, err := sim.SetupSimulation("leveldb-app-sim-2", "Simulation-2")
	require.NoError(t, err, "simulation setup failed", val)

	defer func() {
		_ = newDB.Close()
		require.NoError(t, os.RemoveAll(newDir))
	}()

	newApp := akash.NewApp(log.NewNopLogger(), newDB, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))
	require.Equal(t, akash.AppName, newApp.Name())

	_, err = newApp.InitChain(&abci.RequestInitChain{
		AppStateBytes: exported.AppState,
		ChainId:       AppChainID,
	})
	require.NoError(t, err)

	_, _, err = simulateFromSeedFunc(t, newApp, config)
	require.NoError(t, err)
}

func TestAppStateDeterminism(t *testing.T) {
	if !sim.FlagEnabledValue {
		t.Skip("skipping application simulation")
	}

	encodingConfig := sdkutil.MakeEncodingConfig()
	akash.ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	config := sim.NewConfigFromFlags()
	config.InitialBlockHeight = 1
	config.ExportParamsPath = ""
	config.ChainID = AppChainID
	config.GenesisTime = time.Now().UTC().Unix()

	numSeeds := 2
	numTimesToRunPerSeed := 3
	appHashList := make([]json.RawMessage, numTimesToRunPerSeed)

	for i := 0; i < numSeeds; i++ {
		config.Seed = rand.Int63() // nolint:gosec

		for j := 0; j < numTimesToRunPerSeed; j++ {
			var logger log.Logger
			if sim.FlagVerboseValue {
				logger = log.NewTestLogger(&testing.T{})
			} else {
				logger = log.NewNopLogger()
			}

			db := dbm.NewMemDB()

			appOpts := viper.New()
			appOpts.Set("home", akash.DefaultHome)

			r := rand.New(rand.NewSource(config.Seed)) // nolint: gosec
			genTime := sdksim.RandTimestamp(r)

			appOpts.Set("GenesisTime", genTime)

			app := akash.NewApp(logger, db, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, interBlockCacheOpt(), baseapp.SetChainID(AppChainID))

			fmt.Printf(
				"running non-determinism simulation; seed %d: %d/%d, attempt: %d/%d\n",
				config.Seed, i+1, numSeeds, j+1, numTimesToRunPerSeed,
			)

			_, _, err := simulateFromSeedFunc(t, app, config)
			require.NoError(t, err)

			if config.Commit {
				sim.PrintStats(db)
			}

			appHash := app.LastCommitID().Hash
			appHashList[j] = appHash

			if j != 0 {
				require.Equal(
					t, appHashList[0], appHashList[j],
					"non-determinism in seed %d: %d/%d, attempt: %d/%d\n",
					config.Seed, i+1, numSeeds, j+1, numTimesToRunPerSeed,
				)
			}
		}
	}
}
