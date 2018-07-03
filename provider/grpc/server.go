package grpc

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
	"golang.org/x/net/context"
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

func (s server) Status(ctx context.Context, req *types.Empty) (*types.ServerStatus, error) {
	return &types.ServerStatus{
		Code:    http.StatusOK,
		Message: "OK",
	}, nil
}

func (s server) Deploy(ctx context.Context, req *types.ManifestRequest) (*types.DeployRespone, error) {
	if err := s.handler.HandleManifest(req); err != nil {
		return nil, err
	}
	return &types.DeployRespone{
		Message: "manifest deployed",
	}, nil
}

func (s server) LeaseStatus(ctx context.Context, req *types.LeaseStatusRequest) (*types.LeaseStatusResponse, error) {
	lease, err := keys.ParseLeasePath(strings.Join([]string{req.Deployment, req.Group, req.Order, req.Provider}, "/"))
	deployments, err := s.Client.KubeDeployments(lease.LeaseID)
	if err != nil {
		s.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	if deployments == nil {
		s.log.Error(err.Error())
		return nil, types.ErrResourceNotFound{Message: "no deployments for lease"}
	}
	response := &types.LeaseStatusResponse{}
	for _, deployment := range deployments.Items {
		status := &types.LeaseStatus{Name: deployment.Name, Status: fmt.Sprintf("available replicas: %v/%v", deployment.Status.AvailableReplicas, deployment.Status.Replicas)}
		response.Services = append(response.Services, status)
	}
	return response, nil
}

func (s server) ServiceStatus(ctx context.Context, req *types.ServiceStatusRequest) (*types.ServiceStatusResponse, error) {
	lease, err := keys.ParseLeasePath(strings.Join([]string{req.Deployment, req.Group, req.Order, req.Provider}, "/"))
	deployment, err := s.Client.KubeDeployment(lease.LeaseID, req.Name)
	if err != nil {
		s.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	if deployment == nil {
		s.log.Error(err.Error())
		return nil, types.ErrResourceNotFound{Message: "no deployment for lease"}
	}
	return &types.ServiceStatusResponse{
		ObservedGeneration: deployment.Status.ObservedGeneration,
		Replicas:           deployment.Status.Replicas,
		UpdatedReplicas:    deployment.Status.UpdatedReplicas,
		ReadyReplicas:      deployment.Status.ReadyReplicas,
		AvailableReplicas:  deployment.Status.AvailableReplicas,
	}, nil
}

func (s server) ServiceLog(req *types.LogRequest, server types.Cluster_ServiceLogServer) error {
	lease, err := keys.ParseLeasePath(strings.Join([]string{req.Deployment, req.Group, req.Order, req.Provider}, "/"))
	streams, err := s.Client.KubeLogs(lease.LeaseID, req.TailLines)
	if err != nil {
		s.log.Error(err.Error())
		return types.ErrInternalError{Message: "internal error"}
	}
	if len(streams) == 0 {
		s.log.Error(err.Error())
		return types.ErrResourceNotFound{Message: "no logs for lease"}
	}
	scanners := make([]*bufio.Scanner, len(streams))
	for i, stream := range streams {
		scanners[i] = bufio.NewScanner(stream)
	}
LOOP:
	for {
		select {
		case <-server.Context().Done():
			break LOOP
		default:
			for _, scanner := range scanners {
				if scanner.Scan() {
					server.Send(&types.Log{Message: "pod:" + "service:" + scanner.Text()})
				}
			}
		}
	}

	return nil
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

func (s *server) listenAndServe() error {
	l, err := net.Listen(s.network, s.port)
	if err != nil {
		return err
	}
	l = netutil.LimitListener(l, 10)
	s.log.Info("Running manifest server", "port", s.port, "network", s.network)
	return s.Server.Serve(l)
}
