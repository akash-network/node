package cli

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/ovrclk/akash/sdkutil"
	types "github.com/ovrclk/akash/x/cert/types/v1beta2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"math/big"
	"time"
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



func addGenerateflags(cmd *cobra.Command){
	cmd.Flags().String(flagStart, "", "certificate is not valid before this date. default current timestamp. RFC3339")
	cmd.Flags().Duration(flagValidTime, time.Hour * 24 * 365, "certificate is not valid after this date. RFC3339")
	cmd.Flags().Bool(flagOverwrite, false, "overwrite existing certificate if present")

	flags.AddTxFlagsToCmd(cmd) // TODO - add just the keyring flags? not all the TX ones
}

func cmdGenerateClient() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "client",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doGenerateCmd(cmd, args)
		},
		SilenceUsage: true,
	}
	addGenerateflags(cmd)

	return cmd
}

func cmdGenerateServer() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "server",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doGenerateCmd(cmd, args)
		},
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
	}
	addGenerateflags(cmd)

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
	}
	addPublishFlags(cmd)

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
	}
	addPublishFlags(cmd)

	return cmd
}

func addPublishFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(flagToGenesis, false, "add to genesis")
	flags.AddTxFlagsToCmd(cmd)
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
		return fmt.Errorf("%w: cannot add already existing certificate")
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

func doRevokeCmd(cmd *cobra.Command) error {
	serial := viper.GetString(flagSerial)
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	if len(serial) == 0 {
		if _, valid := new(big.Int).SetString(serial, 10); !valid {
			return errInvalidSerialFlag
		}
	} else {

		fromAddress := cctx.GetFromAddress()

		kpm, err := newKeyPairManager(cctx, fromAddress)
		if err != nil {
			return err
		}

		cert, _, _, err := kpm.read()
		if err != nil {
			return err
		}


		parsedCert, err := x509.ParseCertificate(cert)
		if err != nil {
			return err
		}

		serial = parsedCert.SerialNumber.String()
	}


	msg := &types.MsgRevokeCertificate{
		ID: types.CertificateID{
		Owner:  cctx.FromAddress.String(),
		Serial: serial,
		},
	}

	return sdkutil.BroadcastTX(cmd.Context(), cctx, cmd.Flags(), msg)
}

func cmdRevokeClient() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "server",
		Short:                      "",
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doRevokeCmd(cmd)
		},
		SilenceUsage: true,
	}
	addRevokeCmdFlags(cmd)

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
	}
	addRevokeCmdFlags(cmd)

	return cmd
}

func addRevokeCmdFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagSerial, "", "revoke certificate by serial number")
	flags.AddTxFlagsToCmd(cmd)
}