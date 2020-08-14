package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/cli"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

func main() {

	common.InitSDKConfig()

	encodingConfig := app.MakeEncodingConfig()

	initClientCtx := client.Context{}.
		WithJSONMarshaler(encodingConfig.Marshaler).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastBlock).
		WithHomeDir(common.DefaultNodeHome())

	root := &cobra.Command{
		Use:  "akashd",
		Long: "Akash Daemon CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
				return err
			}

			return server.InterceptConfigsPreRunHandler(cmd)
		},
	}

	authclient.Codec = encodingConfig.Marshaler

	root.AddCommand(
		genutilcli.InitCmd(app.ModuleBasics(), common.DefaultNodeHome()),

		genutilcli.CollectGenTxsCmd(banktypes.GenesisBalancesIterator{}, common.DefaultNodeHome()),

		genutilcli.MigrateGenesisCmd(),

		genutilcli.GenTxCmd(
			app.ModuleBasics(),
			encodingConfig.TxConfig,
			banktypes.GenesisBalancesIterator{},
			common.DefaultNodeHome()),

		genutilcli.ValidateGenesisCmd(app.ModuleBasics(), encodingConfig.TxConfig),
		AddGenesisAccountCmd(common.DefaultNodeHome(), common.DefaultCLIHome()),

		cli.NewCompletionCmd(root, true),
		debug.Cmd(),
	)

	server.AddCommands(root, common.DefaultNodeHome(), newApp, exportAppStateAndTMValidators)

	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, &client.Context{})
	ctx = context.WithValue(ctx, server.ServerContextKey, server.NewDefaultContext())

	executor := cli.PrepareBaseCmd(root, "AKASHD", common.DefaultNodeHome())
	err := executor.ExecuteContext(ctx)
	if err != nil {
		panic(err)
	}

}

func newApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	var cache sdk.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(server.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := server.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	return app.NewApp(
		logger, db, traceStore, cast.ToUint(appOpts.Get(server.FlagInvCheckPeriod)), skipUpgradeHeights,
		cast.ToString(appOpts.Get(flags.FlagHome)),
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(server.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(server.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(server.FlagHaltTime))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(server.FlagTrace))),
	)
}

func exportAppStateAndTMValidators(
	logger log.Logger, db dbm.DB, tio io.Writer, height int64, forZeroHeight bool, jailWhiteList []string,
) (json.RawMessage, []tmtypes.GenesisValidator, *abci.ConsensusParams, error) {

	app := app.NewApp(logger, db, ioutil.Discard, uint(1), map[int64]bool{}, "")

	if height != -1 {
		err := app.LoadHeight(height)
		if err != nil {
			return nil, nil, nil, err
		}
		return app.ExportAppStateAndValidators(forZeroHeight, jailWhiteList)
	}

	return app.ExportAppStateAndValidators(forZeroHeight, jailWhiteList)
}
