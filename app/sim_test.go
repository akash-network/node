package app

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	simapp "github.com/cosmos/cosmos-sdk/simapp"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Get flags every time the simulator is run
var (
	_ = func() string {
		simapp.GetSimulatorFlags()
		return ""
	}()
)

type StoreKeysPrefixes struct {
	A        sdk.StoreKey
	B        sdk.StoreKey
	Prefixes [][]byte
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

func simulateFromSeedFunc(t *testing.T, newApp *AkashApp, config simtypes.Config) (bool, simulation.Params, error) {
	return simulation.SimulateFromSeed(
		t, os.Stdout, newApp.BaseApp, simapp.AppStateFn(newApp.AppCodec(), newApp.SimulationManager()),
		simapp.SimulationOperations(newApp, newApp.AppCodec(), config),
		newApp.ModuleAccountAddrs(), config,
	)
}

func TestFullAppSimulation(t *testing.T) {
	config, db, dir, logger, skip, err := simapp.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application simulation")
	}

	require.NoError(t, err, "simulation setup failed")

	defer func() {
		db.Close()
		require.NoError(t, os.RemoveAll(dir))
	}()

	app1 := NewApp(logger, db, nil, simapp.FlagPeriodValue, map[int64]bool{}, DefaultHome, fauxMerkleModeOpt)
	require.Equal(t, "akash", app1.Name())

	fmt.Printf("config-------- %v", config)
	// run randomized simulation
	_, simParams, simErr := simulateFromSeedFunc(t, app1, config)

	// export state and simParams before the simulation error is checked
	err = simapp.CheckExportSimulation(app1, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		simapp.PrintStats(db)
	}
}

func TestAppImportExport(t *testing.T) {
	config, db, dir, logger, skip, err := simapp.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application import/export simulation")
	}
	require.NoError(t, err, "simulation setup failed")

	defer func() {
		db.Close()
		require.NoError(t, os.RemoveAll(dir))
	}()

	newApp := NewApp(logger, db, nil, simapp.FlagPeriodValue, map[int64]bool{}, DefaultHome, fauxMerkleModeOpt)
	require.Equal(t, AppName, newApp.Name())

	// Run randomized simulation
	_, simParams, simErr := simulateFromSeedFunc(t, newApp, config)

	// export state and simParams before the simulation error is checked
	err = simapp.CheckExportSimulation(newApp, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		simapp.PrintStats(db)
	}

	fmt.Printf("exporting genesis...\n")

	appState, _, consensusParams, err := newApp.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err)

	fmt.Printf("importing genesis...\n")

	_, newDB, newDir, _, _, err := simapp.SetupSimulation("leveldb-app-sim-2", "Simulation-2")
	require.NoError(t, err, "simulation setup failed")

	defer func() {
		newDB.Close()
		require.NoError(t, os.RemoveAll(newDir))
	}()

	newApp1 := NewApp(log.NewNopLogger(), newDB, nil, simapp.FlagPeriodValue, map[int64]bool{}, DefaultHome, fauxMerkleModeOpt)
	require.Equal(t, AppName, newApp1.Name())

	var genesisState simapp.GenesisState
	err = json.Unmarshal(appState, &genesisState)
	require.NoError(t, err)

	ctxA := newApp1.NewContext(true, tmproto.Header{Height: newApp1.LastBlockHeight()})
	ctxB := newApp1.NewContext(true, tmproto.Header{Height: newApp1.LastBlockHeight()})

	newApp1.mm.InitGenesis(ctxB, newApp1.AppCodec(), genesisState)
	newApp1.StoreConsensusParams(ctxB, consensusParams)

	fmt.Printf("comparing stores...\n")

	storeKeysPrefixes := []StoreKeysPrefixes{
		{newApp1.keys[authtypes.StoreKey], newApp1.keys[authtypes.StoreKey], [][]byte{}},
		{newApp1.keys[stakingtypes.StoreKey], newApp1.keys[stakingtypes.StoreKey],
			[][]byte{
				stakingtypes.UnbondingQueueKey, stakingtypes.RedelegationQueueKey, stakingtypes.ValidatorQueueKey,
				stakingtypes.HistoricalInfoKey,
			}}, // ordering may change but it doesn't matter
		{newApp1.keys[slashingtypes.StoreKey], newApp1.keys[slashingtypes.StoreKey], [][]byte{}},
		{newApp1.keys[minttypes.StoreKey], newApp1.keys[minttypes.StoreKey], [][]byte{}},
		{newApp1.keys[distrtypes.StoreKey], newApp1.keys[distrtypes.StoreKey], [][]byte{}},
		{newApp1.keys[banktypes.StoreKey], newApp1.keys[banktypes.StoreKey], [][]byte{banktypes.BalancesPrefix}},
		{newApp1.keys[paramtypes.StoreKey], newApp1.keys[paramtypes.StoreKey], [][]byte{}},
		{newApp1.keys[govtypes.StoreKey], newApp1.keys[govtypes.StoreKey], [][]byte{}},
		{newApp1.keys[evidencetypes.StoreKey], newApp1.keys[evidencetypes.StoreKey], [][]byte{}},
	}

	for _, skp := range storeKeysPrefixes {
		storeA := ctxA.KVStore(skp.A)
		storeB := ctxB.KVStore(skp.B)

		failedKVAs, failedKVBs := sdk.DiffKVStores(storeA, storeB, skp.Prefixes)
		require.Equal(t, len(failedKVAs), len(failedKVBs), "unequal sets of key-values to compare")

		fmt.Printf("compared %d key/value pairs between %s and %s\n", len(failedKVAs), skp.A, skp.B)
		require.Equal(t, len(failedKVAs), 0, simapp.GetSimulationLog(skp.A.Name(),
			newApp1.SimulationManager().StoreDecoders, failedKVAs, failedKVBs))
	}
}

func TestAppSimulationAfterImport(t *testing.T) {
	config, db, dir, logger, skip, err := simapp.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application simulation after import")
	}

	require.NoError(t, err, "simulation setup failed")

	defer func() {
		db.Close()
		require.NoError(t, os.RemoveAll(dir))
	}()

	app := NewApp(logger, db, nil, simapp.FlagPeriodValue, map[int64]bool{}, DefaultHome, fauxMerkleModeOpt)
	require.Equal(t, AppName, app.Name())

	// Run randomized simulation
	stopEarly, simParams, simErr := simulateFromSeedFunc(t, app, config)

	// export state and simParams before the simulation error is checked
	err = simapp.CheckExportSimulation(app, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		simapp.PrintStats(db)
	}

	if stopEarly {
		fmt.Println("can't export or import a zero-validator genesis, exiting test...")
		return
	}

	fmt.Printf("exporting genesis...\n")

	appState, _, _, err := app.ExportAppStateAndValidators(true, []string{})
	require.NoError(t, err)

	fmt.Printf("importing genesis...\n")

	_, newDB, newDir, _, val, err := simapp.SetupSimulation("leveldb-app-sim-2", "Simulation-2")
	require.NoError(t, err, "simulation setup failed", val)

	defer func() {
		newDB.Close()
		require.NoError(t, os.RemoveAll(newDir))
	}()

	newApp := NewApp(log.NewNopLogger(), newDB, nil, simapp.FlagPeriodValue, map[int64]bool{}, DefaultHome, fauxMerkleModeOpt)
	require.Equal(t, AppName, newApp.Name())

	newApp.InitChain(abci.RequestInitChain{
		AppStateBytes: appState,
	})

	_, _, err = simulateFromSeedFunc(t, newApp, config)
	require.NoError(t, err)
}

func TestAppStateDeterminism(t *testing.T) {
	if !simapp.FlagEnabledValue {
		t.Skip("skipping application simulation")
	}

	config := simapp.NewConfigFromFlags()
	config.InitialBlockHeight = 1
	config.ExportParamsPath = ""
	config.OnOperation = false
	config.AllInvariants = false
	config.ChainID = helpers.SimAppChainID

	numSeeds := 2
	numTimesToRunPerSeed := 2
	appHashList := make([]json.RawMessage, numTimesToRunPerSeed)

	for i := 0; i < numSeeds; i++ {
		config.Seed = rand.Int63()

		for j := 0; j < numTimesToRunPerSeed; j++ {
			var logger log.Logger
			if simapp.FlagVerboseValue {
				logger = log.TestingLogger()
			} else {
				logger = log.NewNopLogger()
			}

			db := dbm.NewMemDB()

			app := NewApp(logger, db, nil, simapp.FlagPeriodValue, map[int64]bool{}, DefaultHome, interBlockCacheOpt())

			fmt.Printf(
				"running non-determinism simulation; seed %d: %d/%d, attempt: %d/%d\n",
				config.Seed, i+1, numSeeds, j+1, numTimesToRunPerSeed,
			)

			_, _, err := simulateFromSeedFunc(t, app, config)
			require.NoError(t, err)

			if config.Commit {
				simapp.PrintStats(db)
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
