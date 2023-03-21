package cli

import (
	"encoding/json"
	"fmt"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

func cmdGenerate() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "generate",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(cmdGenerateClient(),
		cmdGenerateServer(),
	)

	return cmd
}

func addGenerateFlags(cmd *cobra.Command) error {
	cmd.Flags().String(flagStart, "", "certificate is not valid before this date. default current timestamp. RFC3339")
	if err := viper.BindPFlag(flagStart, cmd.Flags().Lookup(flagStart)); err != nil {
		return err
	}

	cmd.Flags().Duration(flagValidTime, time.Hour*24*365, "certificate is not valid after this date. RFC3339")
	if err := viper.BindPFlag(flagValidTime, cmd.Flags().Lookup(flagValidTime)); err != nil {
		return err
	}
	cmd.Flags().Bool(flagOverwrite, false, "overwrite existing certificate if present")
	if err := viper.BindPFlag(flagOverwrite, cmd.Flags().Lookup(flagOverwrite)); err != nil {
		return err
	}

	flags.AddTxFlagsToCmd(cmd) // TODO - add just the keyring flags? not all the TX ones
	return nil
}

func cmdGenerateClient() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "client",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE:                       doGenerateCmd,
		SilenceUsage:               true,
		Args:                       cobra.ExactArgs(0),
	}
	err := addGenerateFlags(cmd)
	if err != nil {
		panic(err)
	}

	return cmd
}

func cmdGenerateServer() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "server",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE:                       doGenerateCmd,
		SilenceUsage:               true,
		Args:                       cobra.MinimumNArgs(1),
	}
	err := addGenerateFlags(cmd)
	if err != nil {
		panic(err)
	}

	return cmd
}

func cmdPublish() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "publish",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(cmdPublishClient(),
		cmdPublishServer())

	return cmd
}

func cmdPublishClient() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "client",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doPublishCmd(cmd)
		},
		SilenceUsage: true,
		Args:         cobra.ExactArgs(0),
	}
	err := addPublishFlags(cmd)
	if err != nil {
		panic(err)
	}

	return cmd
}

func cmdPublishServer() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "server",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doPublishCmd(cmd)
		},
		SilenceUsage: true,
		Args:         cobra.ExactArgs(0),
	}
	err := addPublishFlags(cmd)
	if err != nil {
		panic(err)
	}

	return cmd
}

func addPublishFlags(cmd *cobra.Command) error {
	cmd.Flags().Bool(flagToGenesis, false, "add to genesis")
	if err := viper.BindPFlag(flagToGenesis, cmd.Flags().Lookup(flagToGenesis)); err != nil {
		return err
	}

	flags.AddTxFlagsToCmd(cmd)

	return nil
}

func addCertToGenesis(cmd *cobra.Command, cert types.GenesisCertificate) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	cdc := cctx.Codec

	serverCtx := server.GetServerContextFromCmd(cmd)
	config := serverCtx.Config

	config.SetRoot(cctx.HomeDir)

	if err := cert.Validate(); err != nil {
		return fmt.Errorf("%w: failed to validate new genesis certificate", err)
	}

	genFile := config.GenesisFile()
	appState, genDoc, err := genutiltypes.GenesisStateFromGenFile(genFile)
	if err != nil {
		return fmt.Errorf("%w: failed to unmarshal genesis state", err)
	}

	certsGenState := types.GetGenesisStateFromAppState(cdc, appState)

	if certsGenState.Certificates.Contains(cert) {
		return fmt.Errorf("%w: cannot add already existing certificate", err)
	}
	certsGenState.Certificates = append(certsGenState.Certificates, cert)

	certsGenStateBz, err := cdc.MarshalJSON(certsGenState)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal auth genesis state", err)
	}

	appState[types.ModuleName] = certsGenStateBz

	appStateJSON, err := json.Marshal(appState)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal application genesis state", err)
	}

	genDoc.AppState = appStateJSON
	return genutil.ExportGenesisFile(genDoc, genFile)
}

func cmdRevoke() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "revoke",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}
	cmd.AddCommand(cmdRevokeClient(),
		cmdRevokeServer())

	return cmd
}

func cmdRevokeClient() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "client",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doRevokeCmd(cmd)
		},
		SilenceUsage: true,
		Args:         cobra.ExactArgs(0),
	}
	err := addRevokeCmdFlags(cmd)
	if err != nil {
		panic(err)
	}

	return cmd
}

func cmdRevokeServer() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "server",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doRevokeCmd(cmd)
		},
		SilenceUsage: true,
		Args:         cobra.ExactArgs(0),
	}
	err := addRevokeCmdFlags(cmd)
	if err != nil {
		panic(err)
	}

	return cmd
}

func addRevokeCmdFlags(cmd *cobra.Command) error {
	cmd.Flags().String(flagSerial, "", "revoke certificate by serial number")
	if err := viper.BindPFlag(flagSerial, cmd.Flags().Lookup(flagSerial)); err != nil {
		return err
	}

	flags.AddTxFlagsToCmd(cmd)
	return nil
}
