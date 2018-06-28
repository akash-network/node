package rpc

import (
	"context"
	"fmt"
	"net"
	"net/http"

	grpcutil "github.com/ovrclk/akash/grpc"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"
)

type server struct {
	kube.Client
	*grpc.Server
	handler manifest.Handler
	network string
	port    string
	log     log.Logger
}

// NewServer network can be "tcp", "tcp4", "tcp6", "unix" or "unixpacket". phandler is the provider cluster handler
func NewServer(log log.Logger, network, port string, handler manifest.Handler) *server {
	s := &server{
		handler: handler,
		network: network,
		port:    port,
		Server:  grpc.NewServer(grpc.MaxConcurrentStreams(2), grpc.MaxRecvMsgSize(500000)),
		log:     log,
	}
	types.RegisterManifestHandlerServer(s.Server, s)
	return s
}

func (s server) Ping(context context.Context, req *types.GRPCRequest) (*types.ServerStatus, error) {
	return &types.ServerStatus{
		Code:    http.StatusOK,
		Message: "OK",
	}, nil
}

func (s *server) ListenAndServe() error {
	l, err := net.Listen(s.network, ":"+s.port)
	if err != nil {
		return err
	}
	l = netutil.LimitListener(l, 10)
	s.log.Info("Running manifest server", "port", s.port, "network", s.network)
	return s.Server.Serve(l)
}

func RunServer(ctx context.Context, log log.Logger, network, port string, handler manifest.Handler) error {

	address := fmt.Sprintf(":%v", port)

	server := NewServer(log, network, ":"+port, handler)

	ctx, cancel := context.WithCancel(ctx)

	donech := make(chan struct{})

	go func() {
		defer close(donech)
		<-ctx.Done()
		log.Info("Shutting down server")
		server.GracefulStop()
	}()

	log.Info("Starting rpc server", "address", address)
	err := server.ListenAndServe()
	cancel()

	<-donech

	log.Info("Server shutdown")

	return err
}

func (s server) DeployManifest(context context.Context, req *types.GRPCRequest) (*types.DeployManifestRespone, error) {
	// todo: verify request using client cert
	address, _ := grpcutil.VerifySignature(req)
	// payload, ok := req.Payload.(*types.GRPCRequest_ManifestRequest)
	// if !ok {
	// 	return nil, types.ErrInvalidPayload{"invalid request payload"}
	// }
	req.ManifestRequest.Address = address.Bytes()
	if err := s.handler.HandleManifest(req.ManifestRequest); err != nil {
		return nil, err
	}
	return &types.DeployManifestRespone{
		Message: "manifest deployed",
	}, nil
}
