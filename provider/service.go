package provider

import (
	"context"
	"fmt"
	"time"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/caarlos0/env"
	"github.com/ovrclk/akash/provider/bidengine"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
)

type Service interface {
	ManifestHandler() manifest.Handler
	Close() error
	Done() <-chan struct{}

	StatusClient
}

type StatusClient interface {
	Status(context.Context) (*types.ProviderStatus, error)
}

// Simple wrapper around various services needed for running a provider.
func NewService(ctx context.Context, session session.Session, bus event.Bus, cclient cluster.Client) (Service, error) {

	config := config{}
	if err := env.Parse(&config); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	session = session.ForModule("provider-service")

	cluster, err := cluster.NewService(ctx, session, bus, cclient)
	if err != nil {
		cancel()
		return nil, err
	}

	select {
	case <-cluster.Ready():
	case <-time.After(config.ClusterWaitReadyDuration):
		session.Log().Error("timeout waiting for cluster ready")
		cancel()
		<-cluster.Done()
		return nil, fmt.Errorf("timeout waiting for cluster ready")
	}

	bidengine, err := bidengine.NewService(ctx, session, cluster, bus)
	if err != nil {
		session.Log().Error("creating bidengine service", "err", err)
		cancel()
		<-cluster.Done()
		return nil, err
	}

	manifest, err := manifest.NewHandler(ctx, session, bus)
	if err != nil {
		session.Log().Error("creating manifest handler", "err", err)
		cancel()
		<-cluster.Done()
		<-bidengine.Done()
		return nil, err
	}

	service := &service{
		session:   session,
		bus:       bus,
		cluster:   cluster,
		bidengine: bidengine,
		manifest:  manifest,
		ctx:       ctx,
		cancel:    cancel,
		lc:        lifecycle.New(),
	}

	go service.lc.WatchContext(ctx)
	go service.run()

	return service, nil
}

type service struct {
	session session.Session
	bus     event.Bus

	cluster   cluster.Service
	bidengine bidengine.Service
	manifest  manifest.Service

	ctx    context.Context
	cancel context.CancelFunc
	lc     lifecycle.Lifecycle
}

func (s *service) Close() error {
	s.lc.Shutdown(nil)
	return s.lc.Error()
}

func (s *service) Done() <-chan struct{} {
	return s.lc.Done()
}

func (s *service) ManifestHandler() manifest.Handler {
	return s.manifest
}

func (s *service) Status(ctx context.Context) (*types.ProviderStatus, error) {
	cluster, err := s.cluster.Status(ctx)
	if err != nil {
		return nil, err
	}
	bidengine, err := s.bidengine.Status(ctx)
	if err != nil {
		return nil, err
	}
	manifest, err := s.manifest.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &types.ProviderStatus{
		Cluster:   cluster,
		Bidengine: bidengine,
		Manifest:  manifest,
	}, nil
}

func (s *service) run() {
	defer s.lc.ShutdownCompleted()

	// Wait for any service to finish
	select {
	case <-s.lc.ShutdownRequest():
	case <-s.cluster.Done():
	case <-s.bidengine.Done():
	case <-s.manifest.Done():
	}

	// Shut down all services
	s.lc.ShutdownInitiated(nil)
	s.cancel()

	// Wait for all services to finish
	<-s.cluster.Done()
	<-s.bidengine.Done()
	<-s.manifest.Done()
}
