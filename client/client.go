package client

import (
	"errors"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/crypto/keys"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pquery "github.com/ovrclk/akash/x/provider/query"
)

// ErrClientNotFound is a new error with message "Client not found"
var ErrClientNotFound = errors.New("Client not found")

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
	}
}

type client struct {
	cctx       ccontext.CLIContext
	txbldr     auth.TxBuilder
	info       keys.Info
	passphrase string
	qclient    QueryClient
}

func (c *client) Tx() TxClient {
	return c
}

func (c *client) Broadcast(msgs ...sdk.Msg) error {
	bytes, err := c.txbldr.BuildAndSign(c.info.GetName(), c.passphrase, msgs)
	if err != nil {
		return err
	}

	_, err = c.cctx.BroadcastTx(bytes)
	return err
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

func (c *qclient) Deployments(id dtypes.DeploymentID) (dquery.Deployments, error) {
	if c.dclient == nil {
		return dquery.Deployments{}, ErrClientNotFound
	}
	return c.dclient.Deployments(id)
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

func (c *qclient) Orders() (mquery.Orders, error) {
	if c.mclient == nil {
		return mquery.Orders{}, ErrClientNotFound
	}
	return c.mclient.Orders()
}

func (c *qclient) Bids() (mquery.Bids, error) {
	if c.mclient == nil {
		return mquery.Bids{}, ErrClientNotFound
	}
	return c.mclient.Bids()
}

func (c *qclient) Bid(id mtypes.BidID) (mquery.Bid, error) {
	if c.mclient == nil {
		return mquery.Bid{}, ErrClientNotFound
	}
	return c.mclient.Bid(id)
}

func (c *qclient) Leases() (mquery.Leases, error) {
	if c.mclient == nil {
		return mquery.Leases{}, ErrClientNotFound
	}
	return c.mclient.Leases()
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
