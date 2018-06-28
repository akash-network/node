package grpc

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"
)

type server struct {
	cluster.Client
	*grpc.Server
	handler manifest.Handler
	network string
	port    string
	log     log.Logger
}

// NewServer network can be "tcp", "tcp4", "tcp6", "unix" or "unixpacket". phandler is the provider cluster handler
func newServer(log log.Logger, network, port string, handler manifest.Handler, client kube.Client) *server {
	s := &server{
		handler: handler,
		network: network,
		port:    port,
		Server:  grpc.NewServer(grpc.MaxConcurrentStreams(2), grpc.MaxRecvMsgSize(500000)),
		log:     log,
		Client:  client,
	}
	types.RegisterClusterServer(s.Server, s)
	return s
}

func (s server) Ping(context context.Context, req *types.Empty) (*types.ServerStatus, error) {
	return &types.ServerStatus{
		Code:    http.StatusOK,
		Message: "OK",
	}, nil
}

func (s *server) listenAndServe() error {
	l, err := net.Listen(s.network, s.port)
	if err != nil {
		return err
	}
	l = netutil.LimitListener(l, 10)
	s.log.Info("Running manifest server", "port", s.port, "network", s.network)
	return s.Server.Serve(l)
}

func RunServer(ctx context.Context, log log.Logger, network, port string, handler manifest.Handler, client kube.Client) error {

	address := fmt.Sprintf(":%v", port)

	server := newServer(log, network, address, handler, client)

	ctx, cancel := context.WithCancel(ctx)

	donech := make(chan struct{})

	go func() {
		defer close(donech)
		<-ctx.Done()
		log.Info("Shutting down server")
		server.GracefulStop()
	}()

	log.Info("Starting GRPC server", "address", address)
	err := server.listenAndServe()
	cancel()

	<-donech

	log.Info("GRPC server shutdown")

	return err
}

func (s server) Deploy(context context.Context, req *types.ManifestRequest) (*types.DeployRespone, error) {
	if err := s.handler.HandleManifest(req); err != nil {
		return nil, err
	}
	return &types.DeployRespone{
		Message: "manifest deployed",
	}, nil
}

func (s server) LeaseStatus(context context.Context, req *types.LeaseStatusRequest) (*types.LeaseStatusResponse, error) {
	return &types.LeaseStatusResponse{
		Services: []*types.LeaseStatus{&types.LeaseStatus{"OK"}},
	}, nil
}
