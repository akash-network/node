package query

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/ovrclk/akash/x/deployment/types"
)

// Client interface
type Client interface {
	Deployments(types.DeploymentFilters) (Deployments, error)
	Deployment(types.DeploymentID) (Deployment, error)
	Group(types.GroupID) (Group, error)
}

// NewClient creates a client instance with provided context and key
func NewClient(ctx context.CLIContext, key string) Client {
	return &client{ctx: ctx, key: key}
}

type client struct {
	ctx context.CLIContext
	key string
}

func (c *client) Deployments(dfilters types.DeploymentFilters) (Deployments, error) {
	var obj Deployments
	buf, err := NewRawClient(c.ctx, c.key).Deployments(dfilters)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Deployment(id types.DeploymentID) (Deployment, error) {
	var obj Deployment
	buf, err := NewRawClient(c.ctx, c.key).Deployment(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Group(id types.GroupID) (Group, error) {
	var obj Group
	buf, err := NewRawClient(c.ctx, c.key).Group(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}
