package cmd

import (
	"crypto/tls"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	akashclient "github.com/ovrclk/akash/client"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cosmos/cosmos-sdk/client/flags"
)

func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "provider",
		Short:        "Akash provider commands",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String(flags.FlagNode, "http://localhost:26657", "The node address")
	if err := viper.BindPFlag(flags.FlagNode, cmd.PersistentFlags().Lookup(flags.FlagNode)); err != nil {
		return nil
	}

	cmd.AddCommand(SendManifestCmd())
	cmd.AddCommand(statusCmd())
	cmd.AddCommand(leaseStatusCmd())
	cmd.AddCommand(leaseEventsCmd())
	cmd.AddCommand(leaseLogsCmd())
	cmd.AddCommand(serviceStatusCmd())
	cmd.AddCommand(RunCmd())
	cmd.AddCommand(migrateHostnamesCmd())

	return cmd
}

func migrateHostnamesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "migrate-hostnames",
		Short:        "",
		SilenceUsage: true,
		RunE: migrateHostnames,
	}

	addCmdFlags(cmd)
	return cmd
}

func migrateHostnames(cmd *cobra.Command, args []string) error {
	hostnames := args
	if len(hostnames) == 0 {
		panic("empty hostnames")
	}
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		panic(err)
		return err
	}

	prov, err := providerFromFlags(cmd.Flags())
	if err != nil {
		panic(err)
		return err
	}

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		panic(err)
		return err
	}

	gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
	if err != nil {
		panic(err)
		return err
	}

	dseq, err := cmd.Flags().GetUint64("dseq")
	if err != nil {
		panic(err)
		return err
	}

	err = gclient.MigrateHostnames(cmd.Context(), hostnames, dseq)
	if err != nil {
		return showErrorToUser(err)
	}

	return nil
}

