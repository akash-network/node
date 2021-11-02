package app

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	abci "github.com/tendermint/tendermint/abci/types"
)

func TestAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	app1 := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)),
		db, nil, true, 0, map[int64]bool{}, DefaultHome, OptsWithGenesisTime(0))

	for acc := range MacPerms() {
		require.Equal(t, !allowedReceivingModAcc[acc], app1.keeper.bank.BlockedAddr(app1.keeper.acct.GetModuleAddress(acc)),
			"ensure that blocked addresses are properly set in bank keeper")
	}

	genesisState := NewDefaultGenesisState()
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
	app2 := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, 0, map[int64]bool{}, DefaultHome, OptsWithGenesisTime(0))
	_, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}
