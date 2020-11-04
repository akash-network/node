package app_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/cosmos-sdk/simapp"
	"github.com/ovrclk/akash/app"
	abci "github.com/tendermint/tendermint/abci/types"
)

func TestAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	app1 := app.NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)),
		db, nil, 0, map[int64]bool{}, app.DefaultHome, simapp.EmptyAppOptions{})

	genesisState := app.NewDefaultGenesisState()
	stateBytes, err := json.MarshalIndent(genesisState, "", "  ")
	require.NoError(t, err)

	// Initialize the chain
	app1.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app1.Commit()

	// Making a new app object with the db, so that initchain hasn't been called
	app2 := app.NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, 0, map[int64]bool{}, app.DefaultHome, simapp.EmptyAppOptions{})
	_, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

// TODO: re-enable test, can't use unexported fields (keeper) in *_test.go files
// func TestBlockedAddrs(t *testing.T) {
// 	db := dbm.NewMemDB()
// 	app1 := app.NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, 0, map[int64]bool{}, app.DefaultHome)

// 	for acc := range app.MacPerms() {
// 		require.True(t, app1.keeper.bank.BlockedAddr(app1.keeper.acct.GetModuleAddress(acc)))
// 	}
// }
