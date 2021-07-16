package provider

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/boz/go-lifecycle"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/provider/bidengine"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

var (
	ErrClusterReadTimedout = errors.New("timeout waiting for cluster ready")
)

// ValidateClient is the interface to check if provider will bid on given groupspec
type ValidateClient interface {
	Validate(context.Context, dtypes.GroupSpec) (ValidateGroupSpecResult, error)
}

// StatusClient is the interface which includes status of service
type StatusClient interface {
	Status(context.Context) (*Status, error)
}

type Client interface {
	StatusClient
	ValidateClient
	Manifest() manifest.Client
	Cluster() cluster.Client
}

// Service is the interface that includes StatusClient interface.
// It also wraps ManifestHandler, Close and Done methods.
type Service interface {
	Client

	Close() error
	Done() <-chan struct{}
}

// NewService creates and returns new Service instance
// Simple wrapper around various services needed for running a provider.

func NewService(ctx context.Context, cctx client.Context, accAddr sdk.AccAddress, session session.Session, bus pubsub.Bus, cclient cluster.Client, cfg Config) (Service, error) {
	ctx, cancel := context.WithCancel(ctx)

	session = session.ForModule("provider-service")

	clusterConfig := cluster.NewDefaultConfig()
	clusterConfig.InventoryResourcePollPeriod = cfg.InventoryResourcePollPeriod
	clusterConfig.InventoryResourceDebugFrequency = cfg.InventoryResourceDebugFrequency
	clusterConfig.InventoryExternalPortQuantity = cfg.ClusterExternalPortQuantity
	clusterConfig.CPUCommitLevel = cfg.CPUCommitLevel
	clusterConfig.MemoryCommitLevel = cfg.MemoryCommitLevel
	clusterConfig.StorageCommitLevel = cfg.StorageCommitLevel
	clusterConfig.BlockedHostnames = cfg.BlockedHostnames

	cluster, err := cluster.NewService(ctx, session, bus, cclient, clusterConfig)
	if err != nil {
		cancel()
		return nil, err
	}

	select {
	case <-cluster.Ready():
	case <-time.After(cfg.ClusterWaitReadyDuration):
		session.Log().Error(ErrClusterReadTimedout.Error())
		cancel()
		<-cluster.Done()
		return nil, ErrClusterReadTimedout
	}

	bidengine, err := bidengine.NewService(ctx, session, cluster, bus, bidengine.Config{
		PricingStrategy: cfg.BidPricingStrategy,
		Deposit:         cfg.BidDeposit,
		BidTimeout:      cfg.BidTimeout,
	})
	if err != nil {
		errmsg := "creating bidengine service"
		session.Log().Error(errmsg, "err", err)
		cancel()
		<-cluster.Done()
		return nil, errors.Wrap(err, errmsg)
	}

	manifestConfig := manifest.ServiceConfig{
		HTTPServicesRequireAtLeastOneHost: !cfg.DeploymentIngressStaticHosts,
		ManifestTimeout:                   cfg.ManifestTimeout,
	}

	manifest, err := manifest.NewService(ctx, session, bus, cluster.HostnameService(), manifestConfig)
	if err != nil {
		session.Log().Error("creating manifest handler", "err", err)
		cancel()
		<-cluster.Done()
		<-bidengine.Done()
		return nil, err
	}

	bankQueryClient := bankTypes.NewQueryClient(cctx)

	service := &service{
		session:   session,
		bus:       bus,
		cluster:   cluster,
		cclient:   cclient,
		bidengine: bidengine,
		manifest:  manifest,
		ctx:       ctx,
		cancel:    cancel,
		bc:        newBalanceChecker(ctx, bankQueryClient, accAddr, session, bus, cfg.BalanceCheckerCfg),
		lc:        lifecycle.New(),
		config:    cfg,
	}

	go service.lc.WatchContext(ctx)
	go service.run()

	return service, nil
}

type service struct {
	config  Config
	session session.Session
	bus     pubsub.Bus
	cclient cluster.Client

	cluster   cluster.Service
	bidengine bidengine.Service
	manifest  manifest.Service
	bc        *balanceChecker

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

func (s *service) Manifest() manifest.Client {
	return s.manifest
}

func (s *service) Cluster() cluster.Client {
	return s.cclient
}

func (s *service) Status(ctx context.Context) (*Status, error) {
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
	return &Status{
		Cluster:               cluster,
		Bidengine:             bidengine,
		Manifest:              manifest,
		ClusterPublicHostname: s.config.ClusterPublicHostname,
	}, nil
}

func (s *service) Validate(ctx context.Context, gspec dtypes.GroupSpec) (ValidateGroupSpecResult, error) {
	// FUTURE - pass owner here
	price, err := s.config.BidPricingStrategy.CalculatePrice(ctx, "", &gspec)
	if err != nil {
		return ValidateGroupSpecResult{}, err
	}

	return ValidateGroupSpecResult{
		MinBidPrice: price,
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
	<-s.bc.lc.Done()
}
