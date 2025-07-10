package query

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	types "pkg.akt.dev/go/node/deployment/v1"
)

// RawClient interface
type RawClient interface {
	Deployments(DeploymentFilters) ([]byte, error)
	Deployment(types.DeploymentID) ([]byte, error)
	Group(types.GroupID) ([]byte, error)
}

// NewRawClient creates a raw client instance with provided context and key
func NewRawClient(ctx sdkclient.Context, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

type rawclient struct {
	ctx sdkclient.Context
	key string
}

func (c *rawclient) Deployments(dfilters DeploymentFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getDeploymentsPath(dfilters)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Deployment(id types.DeploymentID) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, DeploymentPath(id)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Group(id types.GroupID) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getGroupPath(id)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}
