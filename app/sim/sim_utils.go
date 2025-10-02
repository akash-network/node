package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/cosmos/cosmos-sdk/runtime"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"

	akash "pkg.akt.dev/node/app"
)

// SetupSimulation creates the config, db (levelDB), temporary directory and logger for
// the simulation tests. If `FlagEnabledValue` is false, it skips the current test.
// Returns error on an invalid db instance or temp dir creation.
func SetupSimulation(dirPrefix, dbName string) (simtypes.Config, dbm.DB, string, log.Logger, bool, error) {
	if !FlagEnabledValue {
		return simtypes.Config{}, nil, "", nil, true, nil
	}

	config := NewConfigFromFlags()
	config.ChainID = "akash-sim"

	var logger log.Logger
	if FlagVerboseValue {
		logger = log.NewTestLogger(&testing.T{})
	} else {
		logger = log.NewNopLogger()
	}

	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		return simtypes.Config{}, nil, "", nil, false, err
	}

	db, err := dbm.NewDB(dbName, dbm.BackendType(config.DBBackend), dir)
	if err != nil {
		return simtypes.Config{}, nil, "", nil, false, err
	}

	return config, db, dir, logger, false, nil
}

// SimulationOperations retrieves the simulation params from the provided file path
// and returns all the modules weighted operations
func SimulationOperations(app *akash.AkashApp, cdc codec.JSONCodec, config simtypes.Config) []simtypes.WeightedOperation {
	simState := module.SimulationState{
		AppParams: make(simtypes.AppParams),
		Cdc:       cdc,
	}

	if config.ParamsFile != "" {
		bz, err := os.ReadFile(config.ParamsFile)
		if err != nil {
			panic(err)
		}

		err = json.Unmarshal(bz, &simState.AppParams)
		if err != nil {
			panic(err)
		}
	}

	simState.LegacyProposalContents = app.SimulationManager().GetProposalContents(simState) //nolint:staticcheck
	simState.ProposalMsgs = app.SimulationManager().GetProposalMsgs(simState)
	return app.SimulationManager().WeightedOperations(simState)
}

// CheckExportSimulation exports the app state and simulation parameters to JSON
// if the export paths are defined.
func CheckExportSimulation(app runtime.AppI, config simtypes.Config, params simtypes.Params) error {
	if config.ExportStatePath != "" {
		fmt.Println("exporting app state...")
		exported, err := app.ExportAppStateAndValidators(false, nil, nil)
		if err != nil {
			return err
		}

		if err := os.WriteFile(config.ExportStatePath, []byte(exported.AppState), 0o600); err != nil {
			return err
		}
	}

	if config.ExportParamsPath != "" {
		fmt.Println("exporting simulation params...")
		paramsBz, err := json.MarshalIndent(params, "", " ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(config.ExportParamsPath, paramsBz, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// PrintStats prints the corresponding statistics from the app DB.
func PrintStats(db dbm.DB) {
	fmt.Println("\nLevelDB Stats")
	fmt.Println(db.Stats()["leveldb.stats"])
	fmt.Println("LevelDB cached block size", db.Stats()["leveldb.cachedblock"])
}
