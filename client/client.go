package client

import (
	"context"

	"github.com/pkg/errors"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authutils "github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	"github.com/tendermint/tendermint/libs/log"
	grpc "google.golang.org/grpc"
)

var (
	// ErrClientNotFound is a new error with message "Client not found"
	ErrClientNotFound = errors.New("Client not found")

	// ErrBroadcastTx is used when a broadcast fails due to tendermint errors
	ErrBroadcastTx = errors.New("broadcast tx error")
)

// QueryClient interface includes query clients of deployment, market and provider modules
type QueryClient interface {
	dtypes.QueryClient
	mtypes.QueryClient
	ptypes.QueryClient

	// TODO: implement with search parameters
	ActiveLeasesForProvider(id sdk.AccAddress) (mtypes.Leases, error)
}

// TxClient interface
type TxClient interface {
	Broadcast(...sdk.Msg) error
}

// Client interface pre-defined with query and tx interfaces
type Client interface {
	Query() QueryClient
	Tx() TxClient
}

// NewClient creates new client instance
func NewClient(
	log log.Logger,
	cctx sdkclient.Context,
	txbldr auth.TxBuilder,
	info keyring.Info,
	passphrase string,
	qclient QueryClient,
) Client {
	return &client{
		cctx:       cctx,
		txbldr:     txbldr,
		info:       info,
		passphrase: passphrase,
		qclient:    qclient,
		log:        log.With("cmp", "client/client"),
	}
}

type client struct {
	cctx       sdkclient.Context
	txbldr     auth.TxBuilder
	info       keyring.Info
	passphrase string
	qclient    QueryClient
	log        log.Logger
}

func (c *client) Tx() TxClient {
	return c
}

func (c *client) Broadcast(msgs ...sdk.Msg) error {
	txbldr, err := authutils.PrepareTxBuilder(c.txbldr, c.cctx)
	if err != nil {
		return err
	}

	bytes, err := txbldr.BuildAndSign(c.info.GetName(), c.passphrase, msgs)
	if err != nil {
		return err
	}

	response, err := c.cctx.BroadcastTxSync(bytes)
	if err != nil {
		return err
	}

	if response.Code != 0 {
		c.log.Error("error broadcasting transaction", "response", response)
		return ErrBroadcastTx
	}

	return nil
}

func (c *client) Query() QueryClient {
	return c.qclient
}

type qclient struct {
	dclient dtypes.QueryClient
	mclient mtypes.QueryClient
	pclient ptypes.QueryClient
}

// NewQueryClient creates new query client instance
func NewQueryClient(
	dclient dtypes.QueryClient,
	mclient mtypes.QueryClient,
	pclient ptypes.QueryClient,
) QueryClient {
	return &qclient{
		dclient: dclient,
		mclient: mclient,
		pclient: pclient,
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
