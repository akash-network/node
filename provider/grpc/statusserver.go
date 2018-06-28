package status

import (
	"net"
	"net/http"

	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type statusServer struct {
	kube.Client
	*grpc.Server
	network string
	port    string
	log     log.Logger
}

func (s *statusServer) ListenAndServe() error {
	l, err := net.Listen(s.network, s.port)
	if err != nil {
		return err
	}
	s.log.Info("Running status server", "port", s.port, "network", s.network)
	return s.Server.Serve(l)
}

func NewStatusServer(log log.Logger, network, port string) *statusServer {
	s := &statusServer{
		network: network,
		port:    port,
		Server:  grpc.NewServer(),
		log:     log,
	}
	types.RegisterStatusServer(s.Server, s)
	return s
}

func (s statusServer) Ping(context context.Context, req *types.StatusRequest) (*types.StatusResponse, error) {
	return &types.StatusResponse{
		Code:    http.StatusOK,
		Message: "systems normal",
	}, nil
}
