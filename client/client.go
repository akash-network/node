package client

import (
	"context"

	"github.com/pkg/errors"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"

	"github.com/ovrclk/akash/client/broadcaster"
	atypes "github.com/ovrclk/akash/x/audit/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

var (
	// ErrClientNotFound is a new error with message "Client not found"
	ErrClientNotFound = errors.New("Client not found")
)

// QueryClient interface includes query clients of deployment, market and provider modules
type QueryClient interface {
	dtypes.QueryClient
	mtypes.QueryClient
	ptypes.QueryClient
	atypes.QueryClient

	// TODO: implement with search parameters
	ActiveLeasesForProvider(id sdk.AccAddress) (mtypes.Leases, error)
}

// Client interface pre-defined with query and tx interfaces
type Client interface {
	Query() QueryClient
	Tx() broadcaster.Client
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

type qclient struct {
	dclient dtypes.QueryClient
	mclient mtypes.QueryClient
	pclient ptypes.QueryClient
	aclient atypes.QueryClient
}

// NewQueryClient creates new query client instance
func NewQueryClient(
	dclient dtypes.QueryClient,
	mclient mtypes.QueryClient,
	pclient ptypes.QueryClient,
	aclient atypes.QueryClient,
) QueryClient {
	return &qclient{
		dclient: dclient,
		mclient: mclient,
		pclient: pclient,
		aclient: aclient,
	}
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

// ProviderValidatorAttributes queries all providers
func (c *qclient) AllProvidersAttributes(ctx context.Context, in *atypes.QueryAllProvidersAttributesRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.AllProvidersAttributes(ctx, in, opts...)
}

// ProviderValidatorAttributes queries all provider signed attributes
func (c *qclient) ProviderAttributes(ctx context.Context, in *atypes.QueryProviderAttributesRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.ProviderAttributes(ctx, in, opts...)
}

// ProviderValidatorAttributes queries provider signed attributes by specific validator
func (c *qclient) ProviderValidatorAttributes(ctx context.Context, in *atypes.QueryProviderValidatorRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.ProviderValidatorAttributes(ctx, in, opts...)
}

// ValidatorAttributes queries all providers signed by this validator
func (c *qclient) ValidatorAttributes(ctx context.Context, in *atypes.QueryValidatorAttributesRequest, opts ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	if c.aclient == nil {
		return &atypes.QueryProvidersResponse{}, ErrClientNotFound
	}
	return c.aclient.ValidatorAttributes(ctx, in, opts...)
}
