package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"io"
	"net/http"

	"golang.org/x/sync/errgroup"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/ovrclk/akash/cmd/common"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	FlagResourceServerListenAddress = "resource-server-listen-address"
	FlagLokiGatewayListenAddress    = "loki-gateway-listen-address"
)

var (
	ErrEcdsaPubkeyExpected = errors.New("expected a ecdsa public key")
)

func RunResourceServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "run-resource-server",
		Short: "Run the resource server which authenticates tenants based on JWT before" +
			" providing access to resources",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForeverWithContext(cmd.Context(), func(ctx context.Context) error {
				return doRunResourceServer(ctx, cmd, args)
			})
		},
	}
	flags.AddTxFlagsToCmd(cmd)

	cmd.Flags().String(FlagResourceServerListenAddress, "0.0.0.0:8445",
		"`host:port` for the resource server to listen on")
	if err := viper.BindPFlag(FlagResourceServerListenAddress, cmd.Flags().Lookup(FlagResourceServerListenAddress)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagLokiGatewayListenAddress, "localhost:3100",
		"`host:port` where the loki instance is exposed on provider's network")
	if err := viper.BindPFlag(FlagLokiGatewayListenAddress, cmd.Flags().Lookup(FlagLokiGatewayListenAddress)); err != nil {
		return nil
	}

	cmd.Flags().String(FlagAuthPem, "", "")

	return cmd
}

func doRunResourceServer(ctx context.Context, cmd *cobra.Command, _ []string) error {
	gwAddr := viper.GetString(FlagResourceServerListenAddress)
	lokiGwAddr := viper.GetString(FlagLokiGatewayListenAddress)

	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	var certFromFlag io.Reader
	if val := cmd.Flag(FlagAuthPem).Value.String(); val != "" {
		certFromFlag = bytes.NewBufferString(val)
	}

	kpm, err := cutils.NewKeyPairManager(cctx, cctx.GetFromAddress())
	if err != nil {
		return err
	}

	x509cert, _, err := kpm.ReadX509KeyPair(certFromFlag)
	if err != nil {
		return err
	}

	pubkey, ok := x509cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return ErrEcdsaPubkeyExpected
	}

	group, ctx := errgroup.WithContext(ctx)
	log := openLogger()

	resourceServer, err := gwrest.NewResourceServer(ctx, log, gwAddr, cctx.FromAddress, pubkey, lokiGwAddr)
	if err != nil {
		return err
	}

	group.Go(func() error {
		return resourceServer.ListenAndServe()
	})

	group.Go(func() error {
		<-ctx.Done()
		return resourceServer.Close()
	})

	err = group.Wait()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
