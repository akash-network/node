package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	tmjson "github.com/tendermint/tendermint/libs/json"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	tmcmd "github.com/tendermint/tendermint/cmd/cometbft/commands"

	sdkserver "github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/version"
)

const (
	// Tendermint full-node start flags
	flagTraceStore = "trace-store"
	flagToFile     = "to-file"
)

// Commands server commands
func Commands(defaultNodeHome string, appCreator types.AppCreator, appExport types.AppExporter, addStartFlags types.ModuleInitFlags) []*cobra.Command {
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
func ExportCmd(appExporter types.AppExporter, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export state to JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverCtx := sdkserver.GetServerContextFromCmd(cmd)
			config := serverCtx.Config

			homeDir, _ := cmd.Flags().GetString(flags.FlagHome)
			config.SetRoot(homeDir)

			if _, err := os.Stat(config.GenesisFile()); os.IsNotExist(err) {
				return err
			}

			db, err := openDB(config.RootDir)
			if err != nil {
				return err
			}

			outFile := os.Stdout
			if toFile, _ := cmd.Flags().GetString(flagToFile); toFile != "" {
				outFile, err = os.Create(toFile)
				if err != nil {
					return err
				}
			}

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

			height, _ := cmd.Flags().GetInt64(sdkserver.FlagHeight)
			forZeroHeight, _ := cmd.Flags().GetBool(sdkserver.FlagForZeroHeight)
			jailAllowedAddrs, _ := cmd.Flags().GetStringSlice(sdkserver.FlagJailAllowedAddrs)

			exported, err := appExporter(serverCtx.Logger, db, traceWriter, height, forZeroHeight, jailAllowedAddrs, serverCtx.Viper)
			if err != nil {
				return fmt.Errorf("error exporting state: %v", err)
			}

			doc, err := tmtypes.GenesisDocFromFile(serverCtx.Config.GenesisFile())
			if err != nil {
				return err
			}

			doc.AppState = exported.AppState
			doc.Validators = exported.Validators
			doc.InitialHeight = exported.Height
			doc.ConsensusParams = &tmproto.ConsensusParams{
				Block: tmproto.BlockParams{
					MaxBytes:   exported.ConsensusParams.Block.MaxBytes,
					MaxGas:     exported.ConsensusParams.Block.MaxGas,
					TimeIotaMs: doc.ConsensusParams.Block.TimeIotaMs,
				},
				Evidence: tmproto.EvidenceParams{
					MaxAgeNumBlocks: exported.ConsensusParams.Evidence.MaxAgeNumBlocks,
					MaxAgeDuration:  exported.ConsensusParams.Evidence.MaxAgeDuration,
					MaxBytes:        exported.ConsensusParams.Evidence.MaxBytes,
				},
				Validator: tmproto.ValidatorParams{
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

			_, err = fmt.Fprintln(outFile, string(sdk.MustSortJSON(encoded)))
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().String(flags.FlagHome, defaultNodeHome, "The application home directory")
	cmd.Flags().Int64(sdkserver.FlagHeight, -1, "Export state from a particular height (-1 means latest height)")
	cmd.Flags().Bool(sdkserver.FlagForZeroHeight, false, "Export state to start at height zero (perform preprocessing)")
	cmd.Flags().StringSlice(sdkserver.FlagJailAllowedAddrs, []string{}, "Comma-separated list of operator addresses of jailed validators to unjail")
	cmd.Flags().String(flagToFile, "", "Export Genesis to specified file")

	return cmd
}

func openDB(rootDir string) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return sdk.NewLevelDB("application", dataDir)
}

func openTraceWriter(traceWriterFile string) (w io.Writer, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile(
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o666,
	)
}
