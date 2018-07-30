package query

import (
	"bytes"
	"context"
	"errors"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

type Client interface {
	Account(ctx context.Context, id []byte) (*types.Account, error)

	Providers(ctx context.Context) (*types.Providers, error)
	Provider(ctx context.Context, id []byte) (*types.Provider, error)

	Deployments(ctx context.Context) (*types.Deployments, error)
	Deployment(ctx context.Context, id []byte) (*types.Deployment, error)
	DeploymentLeases(ctx context.Context, id []byte) (*types.Leases, error)

	DeploymentGroups(ctx context.Context) (*types.DeploymentGroups, error)
	DeploymentGroupsForDeployment(ctx context.Context, id []byte) (*types.DeploymentGroups, error)
	DeploymentGroup(ctx context.Context, id types.DeploymentGroupID) (*types.DeploymentGroup, error)

	Orders(ctx context.Context) (*types.Orders, error)
	Order(ctx context.Context, id types.OrderID) (*types.Order, error)

	Fulfillments(ctx context.Context) (*types.Fulfillments, error)
	Fulfillment(ctx context.Context, id types.FulfillmentID) (*types.Fulfillment, error)

	Leases(ctx context.Context) (*types.Leases, error)
	Lease(ctx context.Context, id types.LeaseID) (*types.Lease, error)

	TenantDeployments(ctx context.Context, tenant []byte) (*types.Deployments, error)
	TenantLeases(ctx context.Context, tenant []byte) (*types.Leases, error)
	ProviderLeases(ctx context.Context, provider []byte) (*types.Leases, error)

	Get(ctx context.Context, path string, obj proto.Message, data []byte) error
}

type client struct {
	tmc *tmclient.HTTP
}

func NewClient(tmc *tmclient.HTTP) Client {
	return &client{tmc: tmc}
}

func (c *client) Account(ctx context.Context, id []byte) (*types.Account, error) {
	obj := &types.Account{}
	path := AccountPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Providers(ctx context.Context) (*types.Providers, error) {
	obj := &types.Providers{}
	path := ProvidersPath()
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Provider(ctx context.Context, id []byte) (*types.Provider, error) {
	obj := &types.Provider{}
	path := ProviderPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Deployments(ctx context.Context) (*types.Deployments, error) {
	obj := &types.Deployments{}
	path := DeploymentsPath()
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Deployment(ctx context.Context, id []byte) (*types.Deployment, error) {
	obj := &types.Deployment{}
	path := DeploymentPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) DeploymentGroups(ctx context.Context) (*types.DeploymentGroups, error) {
	obj := &types.DeploymentGroups{}
	path := DeploymentGroupsPath()
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

// TODO: server-side
func (c *client) DeploymentGroupsForDeployment(ctx context.Context, id []byte) (*types.DeploymentGroups, error) {
	obj, err := c.DeploymentGroups(ctx)
	if err != nil {
		return nil, err
	}

	var items []*types.DeploymentGroup

	for _, item := range obj.Items {
		if bytes.Equal(item.Deployment, id) {
			items = append(items, item)
		}
	}

	return &types.DeploymentGroups{
		Items: items,
	}, nil
}

func (c *client) DeploymentGroup(ctx context.Context, id types.DeploymentGroupID) (*types.DeploymentGroup, error) {
	obj := &types.DeploymentGroup{}
	path := DeploymentGroupPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) DeploymentLeases(ctx context.Context, id []byte) (*types.Leases, error) {
	obj := &types.Leases{}
	path := DeploymentLeasesPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Orders(ctx context.Context) (*types.Orders, error) {
	obj := &types.Orders{}
	path := OrdersPath()
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Order(ctx context.Context, id types.OrderID) (*types.Order, error) {
	obj := &types.Order{}
	path := OrderPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Fulfillments(ctx context.Context) (*types.Fulfillments, error) {
	obj := &types.Fulfillments{}
	path := FulfillmentsPath()
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Fulfillment(ctx context.Context, id types.FulfillmentID) (*types.Fulfillment, error) {
	obj := &types.Fulfillment{}
	path := FulfillmentPath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Leases(ctx context.Context) (*types.Leases, error) {
	obj := &types.Leases{}
	path := LeasesPath()
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) Lease(ctx context.Context, id types.LeaseID) (*types.Lease, error) {
	obj := &types.Lease{}
	path := LeasePath(id)
	if err := c.Get(ctx, path, obj, nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *client) TenantDeployments(ctx context.Context, tenant []byte) (*types.Deployments, error) {
	obj := &types.Deployments{}
	path := DeploymentsPath()
	if err := c.Get(ctx, path, obj, tenant); err != nil {
		return nil, err
	}
	return obj, nil
}
func (c *client) TenantLeases(ctx context.Context, tenant []byte) (*types.Leases, error) {
	obj := &types.Leases{}
	path := LeasesPath()
	if err := c.Get(ctx, path, obj, tenant); err != nil {
		return nil, err
	}
	return obj, nil
}

// TODO: server-side
func (c *client) ProviderLeases(ctx context.Context, id []byte) (*types.Leases, error) {
	obj, err := c.Leases(ctx)
	if err != nil {
		return nil, err
	}

	var items []*types.Lease

	for _, item := range obj.Items {
		if bytes.Equal(item.Provider, id) {
			items = append(items, item)
		}
	}

	return &types.Leases{
		Items: items,
	}, nil
}

func (c *client) Get(ctx context.Context, path string, obj proto.Message, data []byte) error {
	result, err := c.tmc.ABCIQuery(path, data)
	if err != nil {
		return err
	}

	if result.Response.IsErr() {
		return errors.New(result.Response.GetLog())
	}

	return proto.Unmarshal(result.Response.Value, obj)
}
