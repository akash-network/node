package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/spf13/cobra"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/go-chi/chi"

	"github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	dmodule "github.com/ovrclk/akash/x/deployment"
	mmodule "github.com/ovrclk/akash/x/market"
	pmodule "github.com/ovrclk/akash/x/provider"
)

type serviceRestConfig struct {
	ctx           context.Context
	log           log.Logger
	serviceStatus provider.StatusClient
}

type serviceRest struct {
	serviceRestConfig
	server *http.Server
	mux    chi.Router
	wg     sync.WaitGroup
}

type restResponse struct {
	Status string      `json:"status"`
	Error  error       `json:"error,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// to make this work akashctl MUST be build -X github.com/cosmos/cosmos-sdk/version.Name=akash
func providerCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "run akash provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx := ccontext.NewCLIContext().WithCodec(cdc)
			ctx := context.Background()

			txbldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			keyname := cctx.GetFromName()
			info, err := txbldr.Keybase().Get(keyname)
			if err != nil {
				return err
			}

			// TODO: actually get the passphrase.
			// passphrase, err := keys.GetPassphrase(fromName)

			aclient := client.NewClient(
				cctx,
				txbldr,
				info,
				keys.DefaultKeyPass,
				client.NewQueryClient(
					dmodule.AppModuleBasic{}.GetQueryClient(cctx),
					mmodule.AppModuleBasic{}.GetQueryClient(cctx),
					pmodule.AppModuleBasic{}.GetQueryClient(cctx),
				),
			)

			pinfo, err := aclient.Query().Provider(info.GetAddress())
			if err != nil {
				return err
			}

			lg := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

			ses := session.New(lg, aclient, pinfo)

			bus := pubsub.NewBus()
			defer bus.Close()

			var cclient cluster.Client
			var service provider.Service

			// todo (troian) provider selection
			cclient = cluster.NullClient()

			if service, err = provider.NewService(ctx, ses, bus, cclient); err != nil {
				return err
			}

			var rest *serviceRest
			if rest, err = newServiceRest(serviceRestConfig{
				ctx:           ctx,
				log:           lg,
				serviceStatus: service,
			}); err != nil {
				_ = service.Close()
				return err
			}

			<-service.Done()

			rest.shutdown()

			return nil
		},
	}

	cmd.Flags().String(flags.FlagKeyringBackend, flags.DefaultKeyringBackend, "Select keyring's backend (os|file|test)")
	viper.BindPFlag(flags.FlagKeyringBackend, cmd.PersistentFlags().Lookup(flags.FlagKeyringBackend))

	cmd.Flags().String(flags.FlagFrom, "provider", "")
	viper.BindPFlag(flags.FlagFrom, cmd.PersistentFlags().Lookup(flags.FlagFrom))
	// if err := viper.BindPFlag(flags.FlagKeyringBackend, cmd.PersistentFlags().Lookup(flags.FlagKeyringBackend)); err != nil {
	// 	return err
	// }
	cmd.Flags().Bool("cluster-k8s", false, "Use Kubernetes cluster")
	cmd.Flags().String("manifest-ns", "lease", "Cluster manifest namespace")

	return cmd
}

func newServiceRest(cfg serviceRestConfig) (*serviceRest, error) {
	rs := &serviceRest{
		serviceRestConfig: cfg,
		mux:               chi.NewRouter(),
	}

	rs.server = &http.Server{
		Addr:    ":8080", // todo(troian) read port from config
		Handler: rs.mux,
	}

	rs.mux.Get("/status", rs.handleStatus)

	rs.wg.Add(1)
	go func(rs *serviceRest) {
		defer rs.wg.Done()

		if err := rs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			rs.log.Error("router error %s", err.Error())
		}
	}(rs)

	return rs, nil
}

func (rs *serviceRest) shutdown() {
	_ = rs.server.Shutdown(rs.ctx)
	rs.wg.Wait()
}

func (rs *serviceRest) handleStatus(wr http.ResponseWriter, _ *http.Request) {
	status, err := rs.serviceStatus.Status(rs.ctx)
	if err != nil {
		wr.WriteHeader(http.StatusInternalServerError)
	} else {
		wr.WriteHeader(http.StatusOK)
	}

	resp := restResponse{
		Status: "ok",
		Error:  err,
		Data:   status,
	}

	if err != nil {
		resp.Status = "error"
	}

	var data []byte

	if data, err = json.Marshal(&resp); err != nil {
		rs.log.Error("marshal status response: %s", err.Error())

		resp = restResponse{
			Status: "error",
			Error:  err,
		}

		data, _ = json.Marshal(&resp)

		wr.WriteHeader(http.StatusInternalServerError)
	}

	_, _ = wr.Write(data)
}
