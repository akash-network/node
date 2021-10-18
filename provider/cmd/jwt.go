package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	akashclient "github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/cmd/common"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
)

func JwtServerAuthenticateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "jwt-server-authenticate",
		Short:        "get JWT",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForever(func(ctx context.Context) error {
				return doJwtServerAuthenticateCmd(ctx, cmd, args)
			})
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	cmd.Flags().String(FlagAuthPem, "", "")
	cmd.Flags().String(FlagProvider, "", "provider")
	if err := viper.BindPFlag(FlagProvider, cmd.Flags().Lookup(FlagProvider)); err != nil {
		return nil
	}
	if err := cmd.MarkFlagRequired(FlagProvider); err != nil {
		panic(err.Error())
	}

	return cmd
}

func doJwtServerAuthenticateCmd(ctx context.Context, cmd *cobra.Command, _ []string) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	txFactory := tx.NewFactoryCLI(cctx, cmd.Flags()).WithTxConfig(cctx.TxConfig).WithAccountRetriever(cctx.AccountRetriever)
	if err != nil {
		return err
	}

	prov, err := sdk.AccAddressFromBech32(viper.GetString(FlagProvider))
	if err != nil {
		return err
	}

	var certFromFlag io.Reader
	if val := cmd.Flag(FlagAuthPem).Value.String(); val != "" {
		certFromFlag = bytes.NewBufferString(val)
	}

	cpem, err := cutils.LoadPEMForAccount(cctx, txFactory.Keybase(), cutils.PEMFromReader(certFromFlag))
	if err != nil {
		return err
	}

	cert, err := tls.X509KeyPair(cpem.Cert, cpem.Priv)
	if err != nil {
		return err
	}

	gclient, err := gwrest.NewJwtClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
	if err != nil {
		return err
	}

	jwt, err := gclient.GetJWT(ctx)
	if err != nil {
		return err
	}

	return cctx.PrintString(jwt.Raw)
}
