package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	akashclient "github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/cmd/common"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func AuthenticateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "authenticate",
		Short:        "Authenticate with a provider using mTLS and get a JWT in return",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForever(func(ctx context.Context) error {
				return doAuthenticateCmd(ctx, cmd, args)
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

func doAuthenticateCmd(ctx context.Context, cmd *cobra.Command, _ []string) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
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

	kpm, err := cutils.NewKeyPairManager(cctx, cctx.FromAddress)
	if err != nil {
		return err
	}

	_, tlsCert, err := kpm.ReadX509KeyPair(certFromFlag)
	if err != nil {
		return err
	}

	gclient, err := gwrest.NewJwtClient(ctx, akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{tlsCert})
	if err != nil {
		return err
	}

	jwt, err := gclient.GetJWT(ctx)
	if err != nil {
		return err
	}

	return cctx.PrintString(jwt.Raw)
}
