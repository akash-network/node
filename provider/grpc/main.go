package grpc

import (
	"flag"
	"net/http"

	"github.com/golang/glog"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/ovrclk/akash/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	echoEndpoint = flag.String("echo_endpoint", "localhost:9090", "endpoint of YourService")
)

func run() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := types.RegisterYourServiceHandlerFromEndpoint(ctx, mux, *echoEndpoint, opts)
	if err != nil {
		return err
	}

	go func() {
		http.ListenAndServe(":8080", mux)
	}()

	

	return nil
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if err := run(); err != nil {
		glog.Fatal(err)
	}
}

package rpc

import (
	"bytes"
	"net"
	"net/http"

	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
	"golang.org/x/net/context"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"
)
type server struct {
	*grpc.Server
}

func (s *server) ListenAndServe() error {
	l, err := net.Listen("tcp", "9090")
	l = netutil.LimitListener(l, 10)
	if err != nil {
		return err
	}
	s.log.Info("Running status server")
	return s.Server.Serve(l)
}

// NewServer network can be "tcp", "tcp4", "tcp6", "unix" or "unixpacket". phandler is the provider cluster handler
func NewServer(log log.Logger, network, port string) *server {
	s := &server{
		network: network,
		port:    port,
		Server:  grpc.NewServer(grpc.MaxConcurrentStreams(2), grpc.MaxRecvMsgSize(500000)),
		log:     log,
	}
	types.RegisterClusterServer(s.Server, s)
	return s
}

func (s server) Ping(context context.Context, req *types.GRPCRequest) (*types.ServerStatus, error) {
	return &types.ServerStatus{
		Code:    http.StatusOK,
		Message: "OK",
	}, nil
}

func (s server) LeaseStatus(context context.Context, req *types.GRPCRequest) (*types.LeaseStatusResponse, error) {
	request, ok := req.Payload.(*types.GRPCRequest_StatusRequest)
	lease := request.StatusRequest.GetLease()
	if !ok {
		return nil, types.ErrInvalidPayload{Message: "invalid payload"}
	}
	deployments, err := s.Client.Deployments()
	if err != nil {
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	if deployments == nil {
		return nil, types.ErrResourceNotFound{Message: "no deployments found for lease"}
	}
	var ownedManifests []*types.ManifestGroup
	for _, deployment := range deployments {
		leaseID := deployment.LeaseID()
		if bytes.Equal(lease.Deployment, leaseID.Deployment) && lease.Group == leaseID.Group &&
			lease.Order == lease.Order && bytes.Equal(lease.Provider, leaseID.Provider) {
			ownedManifests = append(ownedManifests, deployment.ManifestGroup())
		}
	}
	return &types.LeaseStatusResponse{
		Manifests: ownedManifests,
	}, nil
}
