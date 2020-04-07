package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/cosmos-sdk/codec"

	simapp "github.com/cosmos/cosmos-sdk/simapp"
	abci "github.com/tendermint/tendermint/abci/types"
)

func TestSimAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)),
		db, nil)

	genesisState := simapp.NewDefaultGenesisState()
	stateBytes, err := codec.MarshalJSONIndent(app.Codec(), genesisState)
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
	app2 := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil)
	_, _, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

// ensure that black listed addresses are properly set in bank keeper
// func TestBlackListedAddrs(t *testing.T) {
// 	db := dbm.NewMemDB()
// 	app := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, 0)

// 	for acc := range MacPerms() {
// 		require.Equal(t, !allowedReceivingModAcc[acc], app.BankKeeper.BlacklistedAddr(app.SupplyKeeper.GetModuleAddress(acc)))
// 	}
// }

// func TestGetMaccPerms(t *testing.T) {
// 	dup := MacPerms()
// 	require.Equal(t, MaccPerms, dup, "duplicated module account permissions differed from actual module account permissions")
// }
