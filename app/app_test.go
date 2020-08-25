package app

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	simapp "github.com/cosmos/cosmos-sdk/simapp"
	"github.com/ovrclk/akash/cmd/common"
	abci "github.com/tendermint/tendermint/abci/types"
)

func TestAkashAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)),
		db, nil, 0, map[int64]bool{}, common.DefaultNodeHome())

	genesisState := simapp.NewDefaultGenesisState()
	stateBytes, err := json.MarshalIndent(genesisState, "", "  ")
	require.NoError(t, err)

	// Initialize the chain
	app.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	app.Commit()

	// Making a new app object with the db, so that initchain hasn't been called
	app2 := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, 0, map[int64]bool{}, common.DefaultNodeHome())
	_, _, _, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

func TestBlockedAddrs(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, 0, map[int64]bool{}, common.DefaultNodeHome())

	for acc := range macPerms() {
		require.Equal(t, !allowedReceivingModAcc[acc], app.keeper.bank.BlockedAddr(app.keeper.acct.GetModuleAddress(acc)))
	}
}
