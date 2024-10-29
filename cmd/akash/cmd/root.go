package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	"github.com/cosmos/cosmos-sdk/snapshots"
	"github.com/rs/zerolog"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"pkg.akt.dev/go/cli"

	dbm "github.com/cometbft/cometbft-db"
	cmtcfg "github.com/cometbft/cometbft/config"
	cmtcli "github.com/cometbft/cometbft/libs/cli"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	cmtrpc "github.com/cometbft/cometbft/rpc/core"
	cmtrpcsrv "github.com/cometbft/cometbft/rpc/jsonrpc/server"

	rosettaCmd "cosmossdk.io/tools/rosetta/cmd"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	snapshottypes "github.com/cosmos/cosmos-sdk/snapshots/types"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"

	cflags "pkg.akt.dev/go/cli/flags"

	"pkg.akt.dev/node/app"
	"pkg.akt.dev/node/app/params"
	"pkg.akt.dev/node/client"
	"pkg.akt.dev/node/cmd/akash/cmd/testnetify"
	ecmd "pkg.akt.dev/node/events/cmd"
	utilcli "pkg.akt.dev/node/util/cli"
	"pkg.akt.dev/node/util/server"
)

type appCreator struct {
	encCfg params.EncodingConfig
}

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

	// register akash api routes
	cmtrpc.Routes["akash"] = cmtrpcsrv.NewRPCFunc(client.RPCAkash, "")

	initRootCmd(rootCmd, encodingConfig)

	return rootCmd, encodingConfig
}

// GetPersistentPreRunE persistent prerun hook for root command
func GetPersistentPreRunE(encodingConfig params.EncodingConfig, envPrefixes []string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if err := utilcli.InterceptConfigsPreRunHandler(cmd, envPrefixes, false, "", nil); err != nil {
			return err
		}

		initClientCtx := sdkclient.Context{}.
			WithCodec(encodingConfig.Marshaler).
			WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
			WithTxConfig(encodingConfig.TxConfig).
			WithLegacyAmino(encodingConfig.Amino).
			WithInput(os.Stdin).
			WithAccountRetriever(authtypes.AccountRetriever{}).
			WithBroadcastMode(cflags.BroadcastBlock).
			WithHomeDir(app.DefaultHome)

		if err := sdkclient.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
			return err
		}

		return nil
	}
}

// Execute executes the root command.
func Execute(rootCmd *cobra.Command, envPrefix string) error {
	// Create and set a client.Context on the command's Context. During the pre-run
	// of the root command, a default initialized client.Context is provided to
	// seed child command execution with values such as AccountRetriever, Keyring,
	// and a Tendermint RPC. This requires the use of a pointer reference when
	// getting and setting the client.Context. Ideally, we utilize
	// https://github.com/spf13/cobra/pull/1118.

	return ExecuteWithCtx(context.Background(), rootCmd, envPrefix)
}

// ExecuteWithCtx executes the root command.
func ExecuteWithCtx(ctx context.Context, rootCmd *cobra.Command, envPrefix string) error {
	// Create and set a client.Context on the command's Context. During the pre-run
	// of the root command, a default initialized client.Context is provided to
	// seed child command execution with values such as AccountRetriver, Keyring,
	// and a Tendermint RPC. This requires the use of a pointer reference when
	// getting and setting the client.Context. Ideally, we utilize
	// https://github.com/spf13/cobra/pull/1118.
	srvCtx := sdkserver.NewDefaultContext()

	ctx = context.WithValue(ctx, sdkclient.ClientContextKey, &sdkclient.Context{})
	ctx = context.WithValue(ctx, sdkserver.ServerContextKey, srvCtx)

	rootCmd.PersistentFlags().String(cflags.FlagLogLevel, zerolog.InfoLevel.String(), "The logging level (trace|debug|info|warn|error|fatal|panic)")
	rootCmd.PersistentFlags().String(cflags.FlagLogFormat, cmtcfg.LogFormatPlain, "The logging format (json|plain)")
	rootCmd.PersistentFlags().Bool(cflags.FlagLogColor, false, "Pretty logging output. Applied only when log_format=plain")
	rootCmd.PersistentFlags().String(cflags.FlagLogTimestamp, "", "Add timestamp prefix to the logs (rfc3339|rfc3339nano|kitchen)")

	executor := cmtcli.PrepareBaseCmd(rootCmd, envPrefix, app.DefaultHome)
	return executor.ExecuteContext(ctx)
}

func initRootCmd(rootCmd *cobra.Command, encodingConfig params.EncodingConfig) {
	ac := appCreator{encodingConfig}

	home := app.DefaultHome

	debugCmd := debug.Cmd()
	debugCmd.AddCommand(ConvertBech32Cmd())
	debugCmd.AddCommand(testnetify.Cmd())

	rootCmd.AddCommand(
		rpc.StatusCommand(),
		ecmd.EventCmd(),
		cli.QueryCmd(),
		cli.TxCmd(),
		keys.Commands(home),
		genesisCommand(encodingConfig),
		cmtcli.NewCompletionCmd(rootCmd, true),
		debugCmd,
		rosettaCmd.RosettaCommand(encodingConfig.InterfaceRegistry, encodingConfig.Marshaler),
		snapshot.Cmd(ac.newApp),
		pruning.Cmd(ac.newApp, home),
	)

	rootCmd.AddCommand(server.Commands(home, ac.newApp, ac.appExport, addModuleInitFlags)...)

	rootCmd.SetOut(rootCmd.OutOrStdout())
	rootCmd.SetErr(rootCmd.ErrOrStderr())
}

func addModuleInitFlags(startCmd *cobra.Command) {
	crisis.AddModuleInitFlags(startCmd)
}

// genesisCommand builds genesis-related `simd genesis` command. Users may provide application specific commands as a parameter
func genesisCommand(encodingConfig params.EncodingConfig, cmds ...*cobra.Command) *cobra.Command {
	home := app.DefaultHome

	cmd := cli.GetGenesisCmd(app.ModuleBasics(), encodingConfig.TxConfig, app.DefaultHome)

	for _, subCmd := range cmds {
		cmd.AddCommand(subCmd)
	}

	cmd.AddCommand(AddGenesisAccountCmd(home))

	return cmd
}

func (a appCreator) newApp(
	logger cmtlog.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts servertypes.AppOptions,
) servertypes.Application {
	var cache sdk.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(cflags.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(cflags.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := sdkserver.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	homeDir := cast.ToString(appOpts.Get(cflags.FlagHome))
	chainID := cast.ToString(appOpts.Get(cflags.FlagChainID))
	if chainID == "" {
		// fallback to genesis chain-id
		genDocFile := filepath.Join(homeDir, cast.ToString(appOpts.Get("genesis_file")))
		appGenesis, err := tmtypes.GenesisDocFromFile(genDocFile)
		if err != nil {
			panic(err)
		}

		chainID = appGenesis.ChainID
	}

	snapshotDir := filepath.Join(homeDir, "data", "snapshots")
	snapshotDB, err := dbm.NewDB("metadata", server.GetAppDBBackend(appOpts), snapshotDir)
	if err != nil {
		panic(err)
	}
	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}

	// BaseApp Opts
	snapshotOptions := snapshottypes.NewSnapshotOptions(
		cast.ToUint64(appOpts.Get(cflags.FlagStateSyncSnapshotInterval)),
		cast.ToUint32(appOpts.Get(cflags.FlagStateSyncSnapshotKeepRecent)),
	)

	return app.NewApp(
		logger, db, traceStore, true, cast.ToUint(appOpts.Get(cflags.FlagInvCheckPeriod)), skipUpgradeHeights,
		a.encCfg,
		appOpts,
		baseapp.SetChainID(chainID),
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(cflags.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(cflags.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(cflags.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(cflags.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(cflags.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(cflags.FlagIndexEvents))),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(cast.ToInt(appOpts.Get(cflags.FlagIAVLCacheSize))),
	)
}

func (a appCreator) appExport(
	logger cmtlog.Logger,
	db dbm.DB,
	tio io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	var akashApp *app.AkashApp

	homePath, ok := appOpts.Get(cflags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home is not set")
	}
	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}
	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(cflags.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts

	if height != -1 {
		akashApp = app.NewApp(logger, db, tio, false, uint(1), map[int64]bool{}, a.encCfg, appOpts)

		if err := akashApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	} else {
		akashApp = app.NewApp(logger, db, tio, true, uint(1), map[int64]bool{}, a.encCfg, appOpts)
	}

	return akashApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}
