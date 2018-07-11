package grpc

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/version"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type server struct {
	session session.Session
	client  cluster.Client
	status  provider.StatusClient
	handler manifest.Handler
	log     log.Logger
}

func Run(
	ctx context.Context,
	address string,
	session session.Session,
	client cluster.Client,
	status provider.StatusClient,
	handler manifest.Handler) error {
	server := create(session, client, status, handler)
	return run(ctx, server, address)
}

func (s *server) Status(ctx context.Context, req *types.Empty) (*types.ServerStatus, error) {
	status, err := s.status.Status(ctx)
	if err != nil {
		return nil, err
	}

	vsn := version.Get()
	return &types.ServerStatus{
		Provider: s.session.Provider().Address,
		Version:  &vsn,
		Status:   status,
		Code:     http.StatusOK,
		Message:  "OK",
	}, nil
}

func (s server) Deploy(ctx context.Context, req *types.ManifestRequest) (*types.DeployRespone, error) {
	if err := s.handler.HandleManifest(ctx, req); err != nil {
		return nil, err
	}
	return &types.DeployRespone{
		Message: "manifest deployed",
	}, nil
}

// Lease status will retry for one minute
func (s *server) LeaseStatus(ctx context.Context, req *types.LeaseStatusRequest) (*types.LeaseStatusResponse, error) {
	attempts := 12

	lease, err := keys.ParseLeasePath(strings.Join([]string{req.Deployment, req.Group, req.Order, req.Provider}, "/"))
	if err != nil {
		s.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}

	response, err := s.client.LeaseStatus(lease.LeaseID)
	if err == nil {
		return response, err
	}

	for i := 0; i < attempts; i++ {
		time.Sleep(time.Second * 5)
		response, err = s.client.LeaseStatus(lease.LeaseID)
		if err != cluster.ErrNoDeployments {
			break
		}
	}

	return response, err
}

func (s *server) ServiceStatus(ctx context.Context,
	req *types.ServiceStatusRequest) (*types.ServiceStatusResponse, error) {
	lease, err := keys.ParseLeasePath(strings.Join([]string{req.Deployment, req.Group, req.Order, req.Provider}, "/"))
	if err != nil {
		s.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	return s.client.ServiceStatus(lease.LeaseID, req.Name)
}

func (s *server) ServiceLogs(req *types.LogRequest, server types.Cluster_ServiceLogsServer) error {
	lease, err := keys.ParseLeasePath(strings.Join([]string{req.Deployment, req.Group, req.Order, req.Provider}, "/"))
	if err != nil {
		s.log.Error(err.Error())
		return types.ErrInternalError{Message: "internal error"}
	}
	logs, err := s.client.ServiceLogs(server.Context(), lease.LeaseID, req.Options.TailLines, req.Options.Follow)
	if err != nil {
		s.log.Error(err.Error())
		return types.ErrInternalError{Message: "internal error"}
	}
	if len(logs) == 0 {
		return types.ErrResourceNotFound{Message: "no logs for lease"}
	}

	errch := make(chan error, len(logs))
	logch := make(chan *types.Log)

	for _, log := range logs {
		go func(log *cluster.ServiceLog) {
			defer log.Stream.Close()
			for log.Scanner.Scan() {
				logch <- &types.Log{Name: log.Name, Message: log.Scanner.Text()}
			}
			errch <- log.Scanner.Err()
		}(log)
	}

	for remaining := len(logs); remaining > 0; {
		select {
		case err := <-errch:
			if err != nil {
				s.log.Error(err.Error())
			}
			remaining--
		case entry := <-logch:
			if err := server.Send(entry); err != nil {
				s.log.Error(err.Error())
			}
		}
	}

	return nil
}

func create(
	session session.Session,
	client cluster.Client,
	status provider.StatusClient,
	handler manifest.Handler) *server {

	log := session.Log().With("cmp", "grpc-server")

	return &server{
		session: session,
		client:  client,
		status:  status,
		handler: handler,
		log:     log,
	}
}

func run(ctx context.Context, server *server, address string) error {

	fd, err := net.Listen("tcp4", address)
	if err != nil {
		return err
	}

	gserver := grpc.NewServer(grpc.MaxConcurrentStreams(2), grpc.MaxRecvMsgSize(500000))
	types.RegisterClusterServer(gserver, server)

	ctx, cancel := context.WithCancel(ctx)
	donech := make(chan struct{})

	go func() {
		defer close(donech)
		<-ctx.Done()
		server.log.Info("Shutting down server")
		gserver.GracefulStop()
	}()

	server.log.Info("Starting GRPC server", "address", address)
	err = gserver.Serve(fd)
	server.log.Info("GRPC server shutdown.")
	if ctx.Err() == context.Canceled {
		err = nil
	}
	cancel()

	<-donech

	server.log.Info("GRPC done.")

	return err
}
