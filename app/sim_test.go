package app_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"

	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	atypes "pkg.akt.dev/go/node/audit/v1"
	ctypes "pkg.akt.dev/go/node/cert/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	taketypes "pkg.akt.dev/go/node/take/v1"

	dbm "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/store"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdksim "github.com/cosmos/cosmos-sdk/types/simulation"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v7/modules/core/exported"

	akash "pkg.akt.dev/node/app"
	"pkg.akt.dev/node/app/sim"
)

// AppChainID hardcoded chainID for simulation
const (
	AppChainID = "akash-sim"
)

type StoreKeysPrefixes struct {
	A        storetypes.StoreKey
	B        storetypes.StoreKey
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

func simulateFromSeedFunc(t *testing.T, newApp *akash.AkashApp, config simtypes.Config) (bool, simulation.Params, error) {
	return simulation.SimulateFromSeed(
		t,
		os.Stdout,
		newApp.BaseApp,
		simtestutil.AppStateFn(newApp.AppCodec(), newApp.SimulationManager(), akash.NewDefaultGenesisState(newApp.AppCodec())),
		simtypes.RandomAccounts, // Replace with own random account function if using keys other than secp256k1
		simtestutil.SimulationOperations(newApp, newApp.AppCodec(), config),
		newApp.ModuleAccountAddrs(), config,
		newApp.AppCodec(),
	)
}

func TestFullAppSimulation(t *testing.T) {
	config, db, dir, logger, skip, err := sim.SetupSimulation("leveldb-app-sim", "Simulation")
	if skip {
		t.Skip("skipping application simulation")
	}

	require.NoError(t, err, "simulation setup failed")

	encodingConfig := akash.MakeEncodingConfig()

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

	fmt.Printf("config-------- %v", config)
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

	encodingConfig := akash.MakeEncodingConfig()

	appOpts := viper.New()
	appOpts.Set("home", akash.DefaultHome)

	r := rand.New(rand.NewSource(config.Seed)) // nolint: gosec
	genTime := sdksim.RandTimestamp(r)

	appOpts.Set("GenesisTime", genTime)

	app := akash.NewApp(logger, db, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))
	require.Equal(t, akash.AppName, app.Name())

	// Run randomized simulation
	_, simParams, simErr := simulateFromSeedFunc(t, app, config)

	// export state and simParams before the simulation error is checked
	err = simtestutil.CheckExportSimulation(app, config, simParams)
	require.NoError(t, err)
	require.NoError(t, simErr)

	if config.Commit {
		sim.PrintStats(db)
	}

	fmt.Printf("exporting genesis...\n")

	exported, err := app.ExportAppStateAndValidators(false, []string{}, []string{})
	require.NoError(t, err)

	fmt.Printf("importing genesis...\n")

	_, newDB, newDir, _, _, err := sim.SetupSimulation("leveldb-app-sim-2", "Simulation-2")
	require.NoError(t, err, "simulation setup failed")

	defer func() {
		_ = newDB.Close()
		require.NoError(t, os.RemoveAll(newDir))
	}()

	newApp := akash.NewApp(log.NewNopLogger(), newDB, nil, true, sim.FlagPeriodValue, map[int64]bool{}, encodingConfig, appOpts, fauxMerkleModeOpt, baseapp.SetChainID(AppChainID))
	require.Equal(t, akash.AppName, newApp.Name())

	var genesisState akash.GenesisState
	err = json.Unmarshal(exported.AppState, &genesisState)
	require.NoError(t, err)

	ctxA := app.NewContext(true, tmproto.Header{Height: app.LastBlockHeight()})
	ctxB := newApp.NewContext(true, tmproto.Header{Height: app.LastBlockHeight()})

	newApp.MM.InitGenesis(ctxB, app.AppCodec(), genesisState)
	newApp.StoreConsensusParams(ctxB, exported.ConsensusParams)

	fmt.Printf("comparing stores...\n")

	storeKeysPrefixes := []StoreKeysPrefixes{
		{
			app.GetKey(consensustypes.StoreKey),
			newApp.GetKey(consensustypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(authtypes.StoreKey),
			newApp.GetKey(authtypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(feegrant.StoreKey),
			newApp.GetKey(feegrant.StoreKey),
			[][]byte{
				feegrant.FeeAllowanceKeyPrefix,
				feegrant.FeeAllowanceQueueKeyPrefix,
			},
		},
		{
			app.GetKey(authzkeeper.StoreKey),
			newApp.GetKey(authzkeeper.StoreKey),
			[][]byte{
				authzkeeper.GrantKey,
				authzkeeper.GrantQueuePrefix,
			},
		},
		{
			app.GetKey(banktypes.StoreKey),
			newApp.GetKey(banktypes.StoreKey),
			[][]byte{
				banktypes.DenomMetadataPrefix,
				banktypes.DenomAddressPrefix,
				banktypes.BalancesPrefix,
				banktypes.SendEnabledPrefix,
				banktypes.ParamsKey,
			},
		},
		{
			app.GetKey(stakingtypes.StoreKey),
			newApp.GetKey(stakingtypes.StoreKey),
			[][]byte{
				stakingtypes.LastValidatorPowerKey,
				stakingtypes.LastTotalPowerKey,
				stakingtypes.ValidatorsKey,
				stakingtypes.ValidatorsByConsAddrKey,
				stakingtypes.ValidatorsByPowerIndexKey,
				stakingtypes.DelegationKey,
				stakingtypes.UnbondingDelegationKey,
				stakingtypes.UnbondingDelegationByValIndexKey,
				stakingtypes.RedelegationKey,
				stakingtypes.RedelegationByValSrcIndexKey,
				stakingtypes.RedelegationByValDstIndexKey,
				stakingtypes.UnbondingIDKey,
				stakingtypes.UnbondingIndexKey,
				stakingtypes.UnbondingTypeKey,
				stakingtypes.UnbondingQueueKey,
				stakingtypes.RedelegationQueueKey,
				stakingtypes.ValidatorQueueKey,
				stakingtypes.HistoricalInfoKey,
				stakingtypes.ValidatorUpdatesKey,
				stakingtypes.ParamsKey,
				stakingtypes.TokenizeShareRecordPrefix,
				stakingtypes.TokenizeShareRecordIDByOwnerPrefix,
				stakingtypes.TokenizeShareRecordIDByDenomPrefix,
				stakingtypes.LastTokenizeShareRecordIDKey,
				stakingtypes.TotalLiquidStakedTokensKey,
				stakingtypes.TokenizeSharesLockPrefix,
				stakingtypes.TokenizeSharesUnlockQueuePrefix,
			},
		}, // ordering may change but it doesn't matter
		{
			app.GetKey(minttypes.StoreKey),
			newApp.GetKey(minttypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(distrtypes.StoreKey),
			newApp.GetKey(distrtypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(slashingtypes.StoreKey),
			newApp.GetKey(slashingtypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(govtypes.StoreKey),
			newApp.GetKey(govtypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(paramtypes.StoreKey),
			newApp.GetKey(paramtypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(ibchost.StoreKey),
			newApp.GetKey(ibchost.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(ibctransfertypes.StoreKey),
			newApp.GetKey(ibctransfertypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(upgradetypes.StoreKey),
			newApp.GetKey(upgradetypes.StoreKey),
			[][]byte{
				upgradetypes.PlanKey(),
				{upgradetypes.DoneByte},
				{upgradetypes.VersionMapByte},
				{upgradetypes.ProtocolVersionByte},
			},
		},
		{
			app.GetKey(evidencetypes.StoreKey),
			newApp.GetKey(evidencetypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(capabilitytypes.StoreKey),
			newApp.GetKey(capabilitytypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(crisistypes.StoreKey),
			newApp.GetKey(crisistypes.StoreKey),
			[][]byte{},
		},
		{
			app.GetKey(atypes.StoreKey),
			newApp.GetKey(atypes.StoreKey),
			[][]byte{
				atypes.PrefixProviderID(),
			},
		},
		{
			app.GetKey(ctypes.StoreKey),
			newApp.GetKey(ctypes.StoreKey),
			[][]byte{
				ctypes.PrefixCertificateID(),
			},
		},
		{
			app.GetKey(dtypes.StoreKey),
			newApp.GetKey(dtypes.StoreKey),
			[][]byte{
				dtypes.DeploymentPrefix(),
				dtypes.GroupPrefix(),
				dtypes.ParamsPrefix(),
			},
		},
		{
			app.GetKey(mtypes.StoreKey),
			newApp.GetKey(mtypes.StoreKey),
			[][]byte{
				mtypes.OrderPrefix(),
				mtypes.BidPrefix(),
				mtypes.LeasePrefix(),
				mtypes.SecondaryLeasePrefix(),
				mtypes.ParamsPrefix(),
			},
		},
		{
			app.GetKey(ptypes.StoreKey),
			newApp.GetKey(ptypes.StoreKey),
			[][]byte{
				ptypes.ProviderPrefix(),
			},
		},
		{
			app.GetKey(taketypes.StoreKey),
			newApp.GetKey(taketypes.StoreKey),
			[][]byte{
				taketypes.ParamsPrefix(),
			},
		},
	}

	for _, skp := range storeKeysPrefixes {
		storeA := ctxA.KVStore(skp.A)
		storeB := ctxB.KVStore(skp.B)

		failedKVAs, failedKVBs := sdk.DiffKVStores(storeA, storeB, skp.Prefixes)
		require.Equal(t, len(failedKVAs), len(failedKVBs), "unequal sets of key-values to compare")

		fmt.Printf("compared %d key/value pairs between %s and %s\n", len(failedKVAs), skp.A, skp.B)
		require.Equal(t, len(failedKVAs), 0, simtestutil.GetSimulationLog(skp.A.Name(),
			app.SimulationManager().StoreDecoders, failedKVAs, failedKVBs))
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

	encodingConfig := akash.MakeEncodingConfig()

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

	newApp.InitChain(abci.RequestInitChain{
		AppStateBytes: exported.AppState,
		ChainId:       AppChainID,
	})

	_, _, err = simulateFromSeedFunc(t, newApp, config)
	require.NoError(t, err)
}

func TestAppStateDeterminism(t *testing.T) {
	if !sim.FlagEnabledValue {
		t.Skip("skipping application simulation")
	}

	encodingConfig := akash.MakeEncodingConfig()

	config := sim.NewConfigFromFlags()
	config.InitialBlockHeight = 1
	config.ExportParamsPath = ""
	config.OnOperation = false
	config.AllInvariants = false
	config.ChainID = AppChainID

	numSeeds := 2
	numTimesToRunPerSeed := 2
	appHashList := make([]json.RawMessage, numTimesToRunPerSeed)

	for i := 0; i < numSeeds; i++ {
		config.Seed = rand.Int63() // nolint:gosec

		for j := 0; j < numTimesToRunPerSeed; j++ {
			var logger log.Logger
			if sim.FlagVerboseValue {
				logger = log.TestingLogger()
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
