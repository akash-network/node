package query

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"

	types "pkg.akt.dev/go/node/deployment/v1"
)

// Client interface
type Client interface {
	Deployments(DeploymentFilters) (Deployments, error)
	Deployment(types.DeploymentID) (Deployment, error)
	Group(types.GroupID) (Group, error)
}

// NewClient creates a client instance with provided context and key
func NewClient(ctx sdkclient.Context, key string) Client {
	return &client{ctx: ctx, key: key}
}

type client struct {
	ctx sdkclient.Context
	key string
}

func (c *client) Deployments(dfilters DeploymentFilters) (Deployments, error) {
	var obj Deployments
	buf, err := NewRawClient(c.ctx, c.key).Deployments(dfilters)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Deployment(id types.DeploymentID) (Deployment, error) {
	var obj Deployment
	buf, err := NewRawClient(c.ctx, c.key).Deployment(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Group(id types.GroupID) (Group, error) {
	var obj Group
	buf, err := NewRawClient(c.ctx, c.key).Group(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}
