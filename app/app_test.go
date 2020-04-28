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
		db, nil, 0, map[int64]bool{})

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
	app2 := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, 0, map[int64]bool{})
	_, _, err = app2.ExportAppStateAndValidators(false, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

func TestBlackListedAddrs(t *testing.T) {
	db := dbm.NewMemDB()
	app := NewApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, 0, map[int64]bool{})

	for acc := range macPerms() {
		require.True(t, app.keeper.bank.BlacklistedAddr(app.keeper.supply.GetModuleAddress(acc)))
	}
}
