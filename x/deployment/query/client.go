package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/ovrclk/akash/x/deployment/types"
)

// Client interface
type Client interface {
	Deployments(types.DeploymentID) (Deployments, error)
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

func (c *client) Deployments(id types.DeploymentID) (Deployments, error) {
	var obj Deployments
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, DeploymentsPath(id)), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Deployment(id types.DeploymentID) (Deployment, error) {
	var obj Deployment
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, DeploymentPath(id)), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Group(id types.GroupID) (Group, error) {
	var obj Group
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, GroupPath(id)), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}
