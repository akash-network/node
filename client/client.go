package client

import (
	"context"

	"github.com/pkg/errors"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"

	tmrpc "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/ovrclk/akash/client/broadcaster"
	amodule "github.com/ovrclk/akash/x/audit"
	atypes "github.com/ovrclk/akash/x/audit/types/v1beta2"
	cmodule "github.com/ovrclk/akash/x/cert"
	ctypes "github.com/ovrclk/akash/x/cert/types/v1beta2"
	dmodule "github.com/ovrclk/akash/x/deployment"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mmodule "github.com/ovrclk/akash/x/market"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

var (
	// ErrClientNotFound is a new error with message "Client not found"
	ErrClientNotFound = errors.New("Client not found")
	ErrNodeNotSynced  = errors.New("rpc node is not catching up")
)

// QueryClient interface includes query clients of deployment, market and provider modules
type QueryClient interface {
	dtypes.QueryClient
	mtypes.QueryClient
	ptypes.QueryClient
	atypes.QueryClient
	ctypes.QueryClient
}

// Client interface pre-defined with query and tx interfaces
type Client interface {
	Query() QueryClient
	Tx() broadcaster.Client
	NodeSyncInfo(context.Context) (*tmrpc.SyncInfo, error)
}

// NewClient creates new client instance to interface with tendermint.
func NewClient(
	log log.Logger,
	cctx sdkclient.Context,
	txf tx.Factory,
	info keyring.Info,
	qclient QueryClient,
) Client {
	return NewClientWithBroadcaster(
		log,
		cctx,
		txf,
		info,
		qclient,
		broadcaster.NewClient(cctx, txf, info),
	)
}

func NewClientWithBroadcaster(
	log log.Logger,
	cctx sdkclient.Context,
	txf tx.Factory,
	info keyring.Info,
	qclient QueryClient,
	bclient broadcaster.Client,
) Client {
	return &client{
		cctx:    cctx,
		txf:     txf,
		info:    info,
		qclient: qclient,
		bclient: bclient,
		log:     log.With("cmp", "client/client"),
	}
}

type client struct {
	cctx    sdkclient.Context
	txf     tx.Factory
	info    keyring.Info
	qclient QueryClient
	bclient broadcaster.Client
	log     log.Logger
}

func (c *client) Tx() broadcaster.Client {
	return c.bclient
}

func (c *client) Query() QueryClient {
	return c.qclient
}

func (c *client) NodeSyncInfo(ctx context.Context) (*tmrpc.SyncInfo, error) {
	node, err := c.cctx.GetNode()
	if err != nil {
		return nil, err
	}

	status, err := node.Status(ctx)
	if err != nil {
		return nil, err
	}

	info := status.SyncInfo

	return &info, nil
}

type qclient struct {
	dclient dtypes.QueryClient
	mclient mtypes.QueryClient
	pclient ptypes.QueryClient
	aclient atypes.QueryClient
	cclient ctypes.QueryClient
}

// NewQueryClient creates new query client instance
func NewQueryClient(
	dclient dtypes.QueryClient,
	mclient mtypes.QueryClient,
	pclient ptypes.QueryClient,
	aclient atypes.QueryClient,
	cclient ctypes.QueryClient,
) QueryClient {
	return &qclient{
		dclient: dclient,
		mclient: mclient,
		pclient: pclient,
		aclient: aclient,
		cclient: cclient,
	}
}

func NewQueryClientFromCtx(cctx sdkclient.Context) QueryClient {
	return NewQueryClient(
		dmodule.AppModuleBasic{}.GetQueryClient(cctx),
		mmodule.AppModuleBasic{}.GetQueryClient(cctx),
		pmodule.AppModuleBasic{}.GetQueryClient(cctx),
		amodule.AppModuleBasic{}.GetQueryClient(cctx),
		cmodule.AppModuleBasic{}.GetQueryClient(cctx),
	)
}

func (c *qclient) Deployments(ctx context.Context, in *dtypes.QueryDeploymentsRequest, opts ...grpc.CallOption) (*dtypes.QueryDeploymentsResponse, error) {
	if c.dclient == nil {
		return &dtypes.QueryDeploymentsResponse{}, ErrClientNotFound
	}
	return c.dclient.Deployments(ctx, in, opts...)
}

func (c *qclient) Deployment(ctx context.Context, in *dtypes.QueryDeploymentRequest, opts ...grpc.CallOption) (*dtypes.QueryDeploymentResponse, error) {
	if c.dclient == nil {
		return &dtypes.QueryDeploymentResponse{}, ErrClientNotFound
	}
	return c.dclient.Deployment(ctx, in, opts...)
}

func (c *qclient) Group(ctx context.Context, in *dtypes.QueryGroupRequest, opts ...grpc.CallOption) (*dtypes.QueryGroupResponse, error) {
	if c.dclient == nil {
		return &dtypes.QueryGroupResponse{}, ErrClientNotFound
	}
	return c.dclient.Group(ctx, in, opts...)
}

func (c *qclient) Orders(ctx context.Context, in *mtypes.QueryOrdersRequest, opts ...grpc.CallOption) (*mtypes.QueryOrdersResponse, error) {
	if c.mclient == nil {
		return &mtypes.QueryOrdersResponse{}, ErrClientNotFound
	}
	return c.mclient.Orders(ctx, in, opts...)
}

func (c *qclient) Order(ctx context.Context, in *mtypes.QueryOrderRequest, opts ...grpc.CallOption) (*mtypes.QueryOrderResponse, error) {
	if c.mclient == nil {
		return &mtypes.QueryOrderResponse{}, ErrClientNotFound
	}
	return c.mclient.Order(ctx, in, opts...)
}

func (c *qclient) Bids(ctx context.Context, in *mtypes.QueryBidsRequest, opts ...grpc.CallOption) (*mtypes.QueryBidsResponse, error) {
	if c.mclient == nil {
		return &mtypes.QueryBidsResponse{}, ErrClientNotFound
	}
	return c.mclient.Bids(ctx, in, opts...)
}

func (c *qclient) Bid(ctx context.Context, in *mtypes.QueryBidRequest, opts ...grpc.CallOption) (*mtypes.QueryBidResponse, error) {
	if c.mclient == nil {
		return &mtypes.QueryBidResponse{}, ErrClientNotFound
	}
	return c.mclient.Bid(ctx, in, opts...)
}

func (c *qclient) Leases(ctx context.Context, in *mtypes.QueryLeasesRequest, opts ...grpc.CallOption) (*mtypes.QueryLeasesResponse, error) {
	if c.mclient == nil {
		return &mtypes.QueryLeasesResponse{}, ErrClientNotFound
	}
	return c.mclient.Leases(ctx, in, opts...)
}

func (c *qclient) Lease(ctx context.Context, in *mtypes.QueryLeaseRequest, opts ...grpc.CallOption) (*mtypes.QueryLeaseResponse, error) {
	if c.mclient == nil {
		return &mtypes.QueryLeaseResponse{}, ErrClientNotFound
	}
	return c.mclient.Lease(ctx, in, opts...)
}

func (c *qclient) Providers(ctx context.Context, in *ptypes.QueryProvidersRequest, opts ...grpc.CallOption) (*ptypes.QueryProvidersResponse, error) {
	if c.pclient == nil {
		return &ptypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.pclient.Providers(ctx, in, opts...)
}

func (c *qclient) Provider(ctx context.Context, in *ptypes.QueryProviderRequest, opts ...grpc.CallOption) (*ptypes.QueryProviderResponse, error) {
	if c.pclient == nil {
		return &ptypes.QueryProviderResponse{}, ErrClientNotFound
	}
	return c.pclient.Provider(ctx, in, opts...)
}

// AllProvidersAttributes queries all providers
func (c *qclient) AllProvidersAttributes(ctx context.Context, in *atypes.QueryAllProvidersAttributesRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.AllProvidersAttributes(ctx, in, opts...)
}

// ProviderAttributes queries all provider signed attributes
func (c *qclient) ProviderAttributes(ctx context.Context, in *atypes.QueryProviderAttributesRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.ProviderAttributes(ctx, in, opts...)
}

// ProviderAuditorAttributes queries provider signed attributes by specific validator
func (c *qclient) ProviderAuditorAttributes(ctx context.Context, in *atypes.QueryProviderAuditorRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.ProviderAuditorAttributes(ctx, in, opts...)
}

// AuditorAttributes queries all providers signed by this validator
func (c *qclient) AuditorAttributes(ctx context.Context, in *atypes.QueryAuditorAttributesRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.AuditorAttributes(ctx, in, opts...)
}

func (c *qclient) Certificates(ctx context.Context, in *ctypes.QueryCertificatesRequest, opts ...grpc.CallOption) (*ctypes.QueryCertificatesResponse, error) {
	if c.cclient == nil {
		return &ctypes.QueryCertificatesResponse{}, ErrClientNotFound
	}
	return c.cclient.Certificates(ctx, in, opts...)
}
