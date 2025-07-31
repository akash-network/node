package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	tmcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	tmjson "github.com/cometbft/cometbft/libs/json"
	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/spf13/cast"
	cflags "pkg.akt.dev/go/cli/flags"

	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/spf13/cobra"

	sdkserver "github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/version"
)

const (
	// Tendermint full-node start flags
	flagTraceStore = "trace-store"
)

// Commands server commands
func Commands(defaultNodeHome string, appCreator servertypes.AppCreator, appExport servertypes.AppExporter, addStartFlags servertypes.ModuleInitFlags) []*cobra.Command {
	tendermintCmd := &cobra.Command{
		Use:   "tendermint",
		Short: "Tendermint subcommands",
	}

	tendermintCmd.AddCommand(
		sdkserver.ShowNodeIDCmd(),
		sdkserver.ShowValidatorCmd(),
		sdkserver.ShowAddressCmd(),
		sdkserver.VersionCmd(),
		tmcmd.ResetAllCmd,
		tmcmd.ResetStateCmd,
	)

	startCmd := sdkserver.StartCmd(appCreator, defaultNodeHome)
	addStartFlags(startCmd)

	cmds := []*cobra.Command{
		startCmd,
		tendermintCmd,
		ExportCmd(appExport, defaultNodeHome),
		version.NewVersionCommand(),
		sdkserver.NewRollbackCmd(appCreator, defaultNodeHome),
	}

	return cmds
}

// ExportCmd dumps app state to JSON.
func ExportCmd(appExporter servertypes.AppExporter, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export state to JSON",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sctx := sdkserver.GetServerContextFromCmd(cmd)
			config := sctx.Config

			homeDir, _ := cmd.Flags().GetString(cflags.FlagHome)
			config.SetRoot(homeDir)

			if _, err := os.Stat(config.GenesisFile()); os.IsNotExist(err) {
				return err
			}

			db, err := openDB(config.RootDir, sdkserver.GetAppDBBackend(sctx.Viper))
			if err != nil {
				return err
			}

			outFile := os.Stdout
			var outputDocument string

			if outputDocument, _ = cmd.Flags().GetString(cflags.FlagOutputDocument); outputDocument != "-" {
				outFile, err = os.Create(outputDocument) //nolint: gosec
				if err != nil {
					return err
				}
			}
			//
			defer func() {
				if outFile != os.Stdout {
					_ = outFile.Close()
				}
			}()

			if appExporter == nil {
				if _, err := fmt.Fprintln(os.Stderr, "WARNING: App exporter not defined. Returning genesis file."); err != nil {
					return err
				}

				genesis, err := os.ReadFile(config.GenesisFile())
				if err != nil {
					return err
				}

				_, err = fmt.Fprintln(outFile, string(genesis))
				if err != nil {
					return err
				}

				return nil
			}

			traceWriterFile, _ := cmd.Flags().GetString(flagTraceStore)
			traceWriter, err := openTraceWriter(traceWriterFile)
			if err != nil {
				return err
			}

			height, _ := cmd.Flags().GetInt64(cflags.FlagHeight)
			forZeroHeight, _ := cmd.Flags().GetBool(cflags.FlagForZeroHeight)
			jailAllowedAddrs, _ := cmd.Flags().GetStringSlice(cflags.FlagJailAllowedAddrs)
			modulesToExport, _ := cmd.Flags().GetStringSlice(cflags.FlagModulesToExport)

			exported, err := appExporter(
				sctx.Logger,
				db,
				traceWriter,
				height,
				forZeroHeight,
				jailAllowedAddrs,
				sctx.Viper,
				modulesToExport,
			)

			if err != nil {
				return fmt.Errorf("error exporting state: %v", err)
			}

			doc, err := tmtypes.GenesisDocFromFile(sctx.Config.GenesisFile())
			if err != nil {
				return err
			}

			doc.AppState = exported.AppState
			doc.Validators = exported.Validators
			doc.InitialHeight = exported.Height
			doc.ConsensusParams = &tmtypes.ConsensusParams{
				Block: tmtypes.BlockParams{
					MaxBytes: exported.ConsensusParams.Block.MaxBytes,
					MaxGas:   exported.ConsensusParams.Block.MaxGas,
				},
				Evidence: tmtypes.EvidenceParams{
					MaxAgeNumBlocks: exported.ConsensusParams.Evidence.MaxAgeNumBlocks,
					MaxAgeDuration:  exported.ConsensusParams.Evidence.MaxAgeDuration,
					MaxBytes:        exported.ConsensusParams.Evidence.MaxBytes,
				},
				Validator: tmtypes.ValidatorParams{
					PubKeyTypes: exported.ConsensusParams.Validator.PubKeyTypes,
				},
			}

			// NOTE: Tendermint uses a custom JSON decoder for GenesisDoc
			// (except for stuff inside AppState). Inside AppState, we're free
			// to encode as protobuf or amino.
			encoded, err := tmjson.Marshal(doc)
			if err != nil {
				return err
			}

			out := sdk.MustSortJSON(encoded)
			if outputDocument == "-" {
				cmd.Println(string(out))
				_, err = fmt.Fprintln(outFile, string(out))
				if err != nil {
					return err
				}
				return nil
			}

			var exportedGenDoc tmtypes.GenesisDoc
			if err = tmjson.Unmarshal(out, &exportedGenDoc); err != nil {
				return err
			}
			if err = exportedGenDoc.SaveAs(outputDocument); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().String(cflags.FlagHome, defaultNodeHome, "The application home directory")
	cmd.Flags().Int64(cflags.FlagHeight, -1, "Export state from a particular height (-1 means latest height)")
	cmd.Flags().Bool(cflags.FlagForZeroHeight, false, "Export state to start at height zero (perform preprocessing)")
	cmd.Flags().StringSlice(cflags.FlagJailAllowedAddrs, []string{}, "Comma-separated list of operator addresses of jailed validators to unjail")
	cmd.Flags().StringSlice(cflags.FlagModulesToExport, []string{}, "Comma-separated list of modules to export. If empty, will export all modules")
	cmd.Flags().String(cflags.FlagOutputDocument, "-", "Exported state is written to the given file instead of STDOUT")

	return cmd
}

func openDB(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("application", backendType, dataDir)
}

func openTraceWriter(traceWriterFile string) (w io.Writer, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile( //nolint: gosec
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o600,
	)
}

// GetAppDBBackend gets the backend type to use for the application DBs.
func GetAppDBBackend(opts servertypes.AppOptions) dbm.BackendType {
	rv := cast.ToString(opts.Get("app-db-backend"))
	if len(rv) == 0 {
		rv = cast.ToString(opts.Get("db_backend"))
	}
	if len(rv) != 0 {
		return dbm.BackendType(rv)
	}

	return dbm.GoLevelDBBackend
}
