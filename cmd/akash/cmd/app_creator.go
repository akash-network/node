package cmd

import (
	"io"

	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	servertypes "github.com/cosmos/cosmos-sdk/server/types"

	akash "github.com/akash-network/node/app"
	"github.com/akash-network/node/cmd/akash/cmd/testnetify"
)

// for a testnet to be created from the provided app.
func newTestnetApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	// Create an app and type cast to an AkashApp
	app := newApp(logger, db, traceStore, appOpts)
	akashApp, ok := app.(*akash.AkashApp)
	if !ok {
		panic("app created from newApp is not of type AkashApp")
	}

	tcfg, valid := appOpts.Get(testnetify.KeyTestnetConfig).(*akash.TestnetConfig)
	if !valid {
		panic("cflags.KeyTestnetConfig is not of type akash.TestnetConfig")
	}

	// Make modifications to the normal AkashApp required to run the network locally
	return akash.InitAkashAppForTestnet(akashApp, tcfg)
}
