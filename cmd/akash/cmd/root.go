package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/snapshots"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingcli "github.com/cosmos/cosmos-sdk/x/auth/vesting/client/cli"
	bankcmd "github.com/cosmos/cosmos-sdk/x/bank/client/cli"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"

	tmcfg "github.com/tendermint/tendermint/config"
	tmcli "github.com/tendermint/tendermint/libs/cli"
	"github.com/tendermint/tendermint/libs/log"
	dbm "github.com/tendermint/tm-db"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/cmd/akash/cmd/testnetify"
	ecmd "github.com/akash-network/node/events/cmd"
	utilcli "github.com/akash-network/node/util/cli"
	"github.com/akash-network/node/util/server"
)

// NewRootCmd creates a new root command for akash. It is called once in the
// main function.
func NewRootCmd() (*cobra.Command, params.EncodingConfig) {
	encodingConfig := app.MakeEncodingConfig()

	rootCmd := &cobra.Command{
		Use:               "akash",
		Short:             "Akash Blockchain Application",
		Long:              "Akash CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
		SilenceUsage:      true,
		PersistentPreRunE: GetPersistentPreRunE(encodingConfig, []string{"AKASH"}),
	}

	initRootCmd(rootCmd, encodingConfig)

	return rootCmd, encodingConfig
}

// GetPersistentPreRunE persistent prerun hook for root command
func GetPersistentPreRunE(encodingConfig params.EncodingConfig, envPrefixes []string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if err := utilcli.InterceptConfigsPreRunHandler(cmd, envPrefixes, false, "", nil); err != nil {
			return err
		}

		initClientCtx := client.Context{}.
			WithCodec(encodingConfig.Marshaler).
			WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
			WithTxConfig(encodingConfig.TxConfig).
			WithLegacyAmino(encodingConfig.Amino).
			WithInput(os.Stdin).
			WithAccountRetriever(authtypes.AccountRetriever{}).
			WithBroadcastMode(flags.BroadcastBlock).
			WithHomeDir(app.DefaultHome)

		return client.SetCmdClientContextHandler(initClientCtx, cmd)
	}
}

// Execute executes the root command.
func Execute(rootCmd *cobra.Command, envPrefix string) error {
	// Create and set a client.Context on the command's Context. During the pre-run
	// of the root command, a default initialized client.Context is provided to
	// seed child command execution with values such as AccountRetriver, Keyring,
	// and a Tendermint RPC. This requires the use of a pointer reference when
	// getting and setting the client.Context. Ideally, we utilize
	// https://github.com/spf13/cobra/pull/1118.
	srvCtx := sdkserver.NewDefaultContext()
	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, &client.Context{})
	ctx = context.WithValue(ctx, sdkserver.ServerContextKey, srvCtx)

	rootCmd.PersistentFlags().String(flags.FlagLogLevel, zerolog.InfoLevel.String(), "The logging level (trace|debug|info|warn|error|fatal|panic)")
	rootCmd.PersistentFlags().String(flags.FlagLogFormat, tmcfg.LogFormatPlain, "The logging format (json|plain)")
	rootCmd.PersistentFlags().Bool(utilcli.FlagLogColor, false, "Pretty logging output. Applied only when log_format=plain")
	rootCmd.PersistentFlags().String(utilcli.FlagLogTimestamp, "", "Add timestamp prefix to the logs (rfc3339|rfc3339nano|kitchen)")

	executor := tmcli.PrepareBaseCmd(rootCmd, envPrefix, app.DefaultHome)
	return executor.ExecuteContext(ctx)
}

func initRootCmd(rootCmd *cobra.Command, encodingConfig params.EncodingConfig) {
	debugCmd := debug.Cmd()
	debugCmd.AddCommand(ConvertBech32Cmd())
	debugCmd.AddCommand(testnetify.Cmd())

	rootCmd.AddCommand(
		rpc.StatusCommand(),
		ecmd.EventCmd(),
		QueryCmd(),
		TxCmd(),
		keys.Commands(app.DefaultHome),
		genutilcli.InitCmd(app.ModuleBasics(), app.DefaultHome),
		genutilcli.CollectGenTxsCmd(banktypes.GenesisBalancesIterator{}, app.DefaultHome),
		genutilcli.MigrateGenesisCmd(),
		genutilcli.GenTxCmd(app.ModuleBasics(), encodingConfig.TxConfig, banktypes.GenesisBalancesIterator{}, app.DefaultHome),
		genutilcli.ValidateGenesisCmd(app.ModuleBasics()),
		AddGenesisAccountCmd(app.DefaultHome),
		tmcli.NewCompletionCmd(rootCmd, true),
		debugCmd,
		sdkserver.RosettaCommand(encodingConfig.InterfaceRegistry, encodingConfig.Marshaler),
	)

	rootCmd.AddCommand(server.Commands(app.DefaultHome, newApp, createAppAndExport, addModuleInitFlags)...)

	rootCmd.SetOut(rootCmd.OutOrStdout())
	rootCmd.SetErr(rootCmd.ErrOrStderr())
}

func addModuleInitFlags(startCmd *cobra.Command) {
	crisis.AddModuleInitFlags(startCmd)
}

func newApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	var cache sdk.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(sdkserver.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(sdkserver.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := sdkserver.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	snapshotDir := filepath.Join(cast.ToString(appOpts.Get(flags.FlagHome)), "data", "snapshots")
	snapshotDB, err := sdk.NewLevelDB("metadata", snapshotDir)
	if err != nil {
		panic(err)
	}
	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}

	return app.NewApp(
		logger, db, traceStore, true, cast.ToUint(appOpts.Get(sdkserver.FlagInvCheckPeriod)), skipUpgradeHeights,
		cast.ToString(appOpts.Get(flags.FlagHome)),
		appOpts,
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(sdkserver.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(sdkserver.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(sdkserver.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(sdkserver.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(sdkserver.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(sdkserver.FlagIndexEvents))),
		baseapp.SetSnapshotStore(snapshotStore),
		baseapp.SetSnapshotInterval(cast.ToUint64(appOpts.Get(sdkserver.FlagStateSyncSnapshotInterval))),
		baseapp.SetSnapshotKeepRecent(cast.ToUint32(appOpts.Get(sdkserver.FlagStateSyncSnapshotKeepRecent))),
	)
}

func createAppAndExport(
	logger log.Logger, db dbm.DB, tio io.Writer, height int64, forZeroHeight bool, jailAllowedAddrs []string, appOpts servertypes.AppOptions) (servertypes.ExportedApp, error) {

	var akashApp *app.AkashApp

	if height != -1 {
		akashApp = app.NewApp(logger, db, tio, false, uint(1), map[int64]bool{}, "", appOpts)

		if err := akashApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	} else {
		akashApp = app.NewApp(logger, db, tio, true, uint(1), map[int64]bool{}, "", appOpts)
	}

	return akashApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs)
}

func QueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"q"},
		Short:   "Querying subcommands",
	}

	cmd.AddCommand(
		authcmd.GetAccountCmd(),
		flags.LineBreak,
		rpc.ValidatorCommand(),
		rpc.BlockCommand(),
		authcmd.QueryTxsByEventsCmd(),
		authcmd.QueryTxCmd(),
		flags.LineBreak,
	)

	app.ModuleBasics().AddQueryCommands(cmd)
	cmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")
	return cmd
}

func TxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx",
		Short: "Transactions subcommands",
	}

	cmd.AddCommand(
		bankcmd.NewSendTxCmd(),
		flags.LineBreak,
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetValidateSignaturesCommand(),
		flags.LineBreak,
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		flags.LineBreak,
		vestingcli.GetTxCmd(),
	)

	// add modules' tx commands
	app.ModuleBasics().AddTxCommands(cmd)
	cmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")

	return cmd
}
