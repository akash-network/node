package provider

import (
	"context"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/bidengine"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/provider/session"
)

type Service interface {
	ManifestHandler() manifest.Handler
	Close() error
	Done() <-chan struct{}
}

func NewService(ctx context.Context, session session.Session, bus event.Bus) (Service, error) {

	ctx, cancel := context.WithCancel(ctx)

	session = session.ForModule("provider-service")

	cluster, err := cluster.NewService(session.Log(), ctx, bus)
	if err != nil {
		cancel()
		return nil, err
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

func (s *service) run() {
	defer s.lc.ShutdownCompleted()

	select {
	case <-s.lc.ShutdownRequest():
	case <-s.cluster.Done():
	case <-s.bidengine.Done():
	case <-s.manifest.Done():
	}

	s.lc.ShutdownInitiated(nil)
	s.cancel()

	<-s.cluster.Done()
	<-s.bidengine.Done()
	<-s.manifest.Done()
}
