package testnetify

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"

	cmtjson "github.com/tendermint/tendermint/libs/json"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	ibccltypes "github.com/cosmos/ibc-go/v4/modules/core/02-client/types"
	ibcchtypes "github.com/cosmos/ibc-go/v4/modules/core/04-channel/types"
)

const (
	flagConfig         = "config"
	flagSpinner        = "spinner"
	denomDecimalPlaces = 1e6
)

// yeah, I know
var cdc codec.Codec

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "testnetify INFILE OUTFILE",
		Short: "Utility to alter exported genesis state for testing purposes",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			cctx := client.GetClientContextFromCmd(cmd)
			cdc = cctx.Codec

			sID, err := cmd.Flags().GetInt(flagSpinner)
			if err != nil {
				return err
			}

			if sID < 0 || sID > 90 {
				return fmt.Errorf("invalid value %d for --spinner. expecting 0..90", sID) // nolint: goerr113
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cctx := client.GetClientContextFromCmd(cmd)
			cdc := cctx.Codec

			oFile, err := os.Create(args[1])
			if err != nil {
				return err
			}

			modifyCompleted := false

			var spinner *yacspin.Spinner

			defer func() {
				_ = oFile.Sync()
				_ = oFile.Close()

				if !modifyCompleted {
					_ = os.Remove(args[1])
				}

				if spinner != nil && spinner.Status() == yacspin.SpinnerRunning {
					if err != nil {
						_ = spinner.StopFail()
					} else {
						_ = spinner.Stop()
					}
				}
			}()

			cfg := config{}

			if cfgPath, _ := cmd.Flags().GetString(flagConfig); cfgPath != "" {
				cfgFile, err := os.Open(cfgPath)
				if err != nil {
					return err
				}

				defer func() {
					_ = cfgFile.Close()
				}()

				cfgData, err := io.ReadAll(cfgFile)
				if err != nil {
					return err
				}
				if err = json.Unmarshal(cfgData, &cfg); err != nil {
					return err
				}
			}

			spinnerID, _ := cmd.Flags().GetInt(flagSpinner)

			ycfg := yacspin.Config{
				Frequency:         100 * time.Millisecond,
				CharSet:           yacspin.CharSets[spinnerID],
				ColorAll:          true,
				Prefix:            "",
				Suffix:            " ",
				SuffixAutoColon:   true,
				StopCharacter:     "✓",
				StopFailCharacter: "✗",
				StopColors:        []string{"fgGreen"},
				StopFailColors:    []string{"fgRed"},
			}

			spinner, err = yacspin.New(ycfg)
			if err != nil {
				return err
			}

			cmd.SilenceErrors = true

			inSource := "stdin"
			if args[0] != "-" {
				inSource = args[0]
			}

			var gState *GenesisState

			{
				spinner.Message(fmt.Sprintf("loading genesis from \"%s\"", inSource))
				spinner.StopMessage(fmt.Sprintf("loaded genesis from \"%s\"", inSource))
				_ = spinner.Start()
				appState, genDoc, err := loadGenesis(cmd, args[0])
				if err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed to load genesis state: %s", err.Error()))
					return err
				}
				_ = spinner.Stop()

				spinner.Message("preparing genesis state")
				spinner.StopMessage("prepared genesis state")
				_ = spinner.Start()
				gState, err = NewGenesisState(spinner, appState, genDoc)
				if err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed to prepare genesis state: %s", err.Error()))
					return err
				}
			}

			if c := cfg.ChainID; c != nil {
				spinner.Message("modifying chain_id")
				spinner.StopMessage("modified chain_id")

				gState.doc.ChainID = *c

				_ = spinner.Stop()
			}

			if c := cfg.Escrow; c != nil {
				spinner.Message("modifying escrow module")
				spinner.StopMessage("modified escrow module")
				_ = spinner.Start()
				if err = gState.modifyEscrowState(cdc, c); err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed modifying escrow module. %s", err.Error()))
					return err
				}
				_ = spinner.Stop()
			}

			if c := cfg.IBC; c != nil {
				spinner.Message("modifying IBC module")
				spinner.StopMessage("modified IBC module")
				_ = spinner.Start()
				if err = gState.modifyIBC(cdc, c); err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed modifying IBC. %s", err.Error()))
					return err
				}
				_ = spinner.Stop()
			}

			if c := cfg.Gov; c != nil {
				spinner.Message("modifying gov module")
				spinner.StopMessage("modified gov module")
				_ = spinner.Start()
				if err = gState.modifyGov(cdc, c); err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed modifying gov module. %s", err.Error()))
					return err
				}
				_ = spinner.Stop()
			}

			if c := cfg.Accounts; c != nil {
				spinner.Message("modifying accounts")
				spinner.StopMessage("modified accounts")
				_ = spinner.Start()
				if err = gState.modifyAccounts(spinner, cdc, c); err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed modifying accounts. %s", err.Error()))
					return err
				}
				_ = spinner.Stop()
			}

			if c := cfg.Validators; c != nil {
				spinner.Message("modifying validators")
				spinner.StopMessage("modified validators")
				_ = spinner.Start()
				if err = gState.modifyValidators(cdc, c); err != nil {
					spinner.StopFailMessage(fmt.Sprintf("failed modifying validators. %s", err.Error()))
					return err
				}
				_ = spinner.Stop()
			}

			spinner.Message("marshaling genesis state")
			spinner.StopMessage("marshaled genesis state")
			_ = spinner.Start()
			if err = gState.pack(cdc); err != nil {
				spinner.StopFailMessage(fmt.Sprintf("failed to pack genesis state. %s", err.Error()))
				return err
			}

			spinner.Message("validating genesis state")
			spinner.StopMessage("validated genesis state")
			_ = spinner.Start()
			if err := gState.doc.ValidateAndComplete(); err != nil {
				spinner.StopFailMessage(fmt.Sprintf("error validating genesis state. %s", err.Error()))
				return err
			}
			_ = spinner.Stop()

			spinner.Message(fmt.Sprintf("exporting genesis doc to \"%s\"", args[1]))
			spinner.StopMessage(fmt.Sprintf("exported genesis doc to \"%s\"", args[1]))
			_ = spinner.Start()
			genBytes, err := cmtjson.MarshalIndent(gState.doc, "", "  ")
			if err != nil {
				spinner.StopFailMessage(fmt.Sprintf("error marshaling genesis doc. %s", err.Error()))
				return err
			}

			_, err = oFile.Write(genBytes)
			if err != nil {
				spinner.StopFailMessage(fmt.Sprintf("error exporting genesis doc. %s", err.Error()))
				return err
			}

			modifyCompleted = true
			_ = spinner.Stop()
			return nil
		},
	}

	cmd.Flags().StringP(flagConfig, "c", "", "config file")
	cmd.Flags().Int(flagSpinner, 52, "spinner type. allowed values 0..90")

	return cmd
}

func loadGenesis(cmd *cobra.Command, file string) (genesisState map[string]json.RawMessage, genDoc *tmtypes.GenesisDoc, err error) {
	var stream []byte

	reader := cmd.InOrStdin()

	defer func() {
		if rd, closer := reader.(io.Closer); reader != nil && closer {
			_ = rd.Close()
		}
	}()

	if file != "-" {
		reader, err = os.Open(file)
		if err != nil {
			reader = nil
			return genesisState, genDoc, err
		}
	}

	if stream, err = io.ReadAll(reader); err != nil {
		return genesisState, genDoc, err
	}

	if genDoc, err = tmtypes.GenesisDocFromJSON(stream); err != nil {
		return genesisState, genDoc, err
	}

	genesisState, err = genutiltypes.GenesisStateFromGenDoc(*genDoc)
	return genesisState, genDoc, err
}

func (ga *GenesisState) modifyGov(cdc codec.Codec, cfg *GovConfig) error {
	if cfg == nil {
		return nil
	}

	if err := ga.app.GovState.unpack(cdc); err != nil {
		return err
	}

	if params := cfg.VotingParams; params != nil {
		if params.VotingPeriod.Duration > 0 {
			ga.app.GovState.state.VotingParams.VotingPeriod = params.VotingPeriod.Duration
		}
	}

	return nil
}

func (ga *GenesisState) modifyIBC(cdc codec.Codec, cfg *IBCConfig) error {
	if cfg == nil {
		return nil
	}

	if err := ga.app.IBCState.unpack(cdc); err != nil {
		return err
	}

	if cfg.Prune {
		ga.app.IBCState.state.ChannelGenesis.Channels = []ibcchtypes.IdentifiedChannel{}
		ga.app.IBCState.state.ChannelGenesis.Acknowledgements = []ibcchtypes.PacketState{}
		ga.app.IBCState.state.ChannelGenesis.Commitments = []ibcchtypes.PacketState{}
		ga.app.IBCState.state.ChannelGenesis.Receipts = []ibcchtypes.PacketState{}
		ga.app.IBCState.state.ChannelGenesis.SendSequences = []ibcchtypes.PacketSequence{}
		ga.app.IBCState.state.ChannelGenesis.RecvSequences = []ibcchtypes.PacketSequence{}
		ga.app.IBCState.state.ChannelGenesis.AckSequences = []ibcchtypes.PacketSequence{}

		ga.app.IBCState.state.ClientGenesis.Clients = ibccltypes.IdentifiedClientStates{}
		ga.app.IBCState.state.ClientGenesis.ClientsConsensus = ibccltypes.ClientsConsensusStates{}
		ga.app.IBCState.state.ClientGenesis.ClientsMetadata = []ibccltypes.IdentifiedGenesisMetadata{}
	}

	return nil
}
