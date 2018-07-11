package json

import (
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type proxy struct {
	endpoint string
	addr     string
	log      log.Logger
	mux      *runtime.ServeMux
}

func new(ctx context.Context, log log.Logger, addr, endpoint string) (*proxy, error) {
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := types.RegisterClusterHandlerFromEndpoint(ctx, mux, endpoint, opts)
	if err != nil {
		return nil, err
	}
	return &proxy{
		endpoint: endpoint,
		addr:     addr,
		log:      log,
		mux:      mux,
	}, nil
}

func (p *proxy) listenAndServe() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := types.RegisterClusterHandlerFromEndpoint(ctx, mux, p.endpoint, opts)
	if err != nil {
		return err
	}
	return http.ListenAndServe(p.addr, mux)
}

func Run(ctx context.Context, log log.Logger, address, endpoint string) error {
	ctx, cancel := context.WithCancel(ctx)
	proxy, err := new(ctx, log, address, endpoint)
	if err != nil {
		return err
	}

	log.Info("Starting GRPC JSON proxy server", "address", proxy.addr, "endpoint", proxy.endpoint)
	err = proxy.listenAndServe()
	cancel()

	log.Info("GRPC server shutdown")

	return err
}
