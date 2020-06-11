package client

import (
	"github.com/pkg/errors"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/crypto/keys"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authutils "github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pquery "github.com/ovrclk/akash/x/provider/query"
	"github.com/tendermint/tendermint/libs/log"
)

var (
	// ErrClientNotFound is a new error with message "Client not found"
	ErrClientNotFound = errors.New("Client not found")

	// ErrBroadcastTx is used when a broadcast fails due to tendermint errors
	ErrBroadcastTx = errors.New("broadcast tx error")
)

// QueryClient interface includes query clients of deployment, market and provider modules
type QueryClient interface {
	dquery.Client
	mquery.Client
	pquery.Client

	// TODO: implement with search parameters
	ActiveLeasesForProvider(id sdk.AccAddress) (mquery.Leases, error)
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
	cctx ccontext.CLIContext,
	txbldr auth.TxBuilder,
	info keys.Info,
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
	cctx       ccontext.CLIContext
	txbldr     auth.TxBuilder
	info       keys.Info
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
	dclient dquery.Client
	mclient mquery.Client
	pclient pquery.Client
}

// NewQueryClient creates new query client instance
func NewQueryClient(
	dclient dquery.Client,
	mclient mquery.Client,
	pclient pquery.Client,
) QueryClient {
	return &qclient{
		dclient: dclient,
		mclient: mclient,
		pclient: pclient,
	}
}

func (c *qclient) Deployments(filters dquery.DeploymentFilters) (dquery.Deployments, error) {
	if c.dclient == nil {
		return dquery.Deployments{}, ErrClientNotFound
	}
	return c.dclient.Deployments(filters)
}

func (c *qclient) Deployment(id dtypes.DeploymentID) (dquery.Deployment, error) {
	if c.dclient == nil {
		return dquery.Deployment{}, ErrClientNotFound
	}
	return c.dclient.Deployment(id)
}

func (c *qclient) Group(id dtypes.GroupID) (dquery.Group, error) {
	if c.dclient == nil {
		return dquery.Group{}, ErrClientNotFound
	}
	return c.dclient.Group(id)
}

func (c *qclient) Orders(filters mquery.OrderFilters) (mquery.Orders, error) {
	if c.mclient == nil {
		return mquery.Orders{}, ErrClientNotFound
	}
	return c.mclient.Orders(filters)
}

func (c *qclient) Order(id mtypes.OrderID) (mquery.Order, error) {
	if c.mclient == nil {
		return mquery.Order{}, ErrClientNotFound
	}
	return c.mclient.Order(id)
}

func (c *qclient) Bids(filters mquery.BidFilters) (mquery.Bids, error) {
	if c.mclient == nil {
		return mquery.Bids{}, ErrClientNotFound
	}
	return c.mclient.Bids(filters)
}

func (c *qclient) Bid(id mtypes.BidID) (mquery.Bid, error) {
	if c.mclient == nil {
		return mquery.Bid{}, ErrClientNotFound
	}
	return c.mclient.Bid(id)
}

func (c *qclient) Leases(filters mquery.LeaseFilters) (mquery.Leases, error) {
	if c.mclient == nil {
		return mquery.Leases{}, ErrClientNotFound
	}
	return c.mclient.Leases(filters)
}

func (c *qclient) Lease(id mtypes.LeaseID) (mquery.Lease, error) {
	if c.mclient == nil {
		return mquery.Lease{}, ErrClientNotFound
	}
	return c.mclient.Lease(id)
}

func (c *qclient) Providers() (pquery.Providers, error) {
	if c.pclient == nil {
		return pquery.Providers{}, ErrClientNotFound
	}
	return c.pclient.Providers()
}

func (c *qclient) Provider(id sdk.AccAddress) (*pquery.Provider, error) {
	if c.pclient == nil {
		return nil, ErrClientNotFound
	}
	return c.pclient.Provider(id)
}
