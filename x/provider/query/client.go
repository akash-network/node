package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Client interface
type Client interface {
	Providers() (Providers, error)
	Provider(sdk.AccAddress) (*Provider, error)
}

// NewClient creates a client instance with provided context and key
func NewClient(ctx context.CLIContext, key string) Client {
	return &client{ctx: ctx, key: key}
}

type client struct {
	ctx context.CLIContext
	key string
}

func (c *client) Providers() (Providers, error) {
	var obj Providers
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, ProvidersPath()), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Provider(id sdk.AccAddress) (*Provider, error) {
	var obj Provider
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, ProviderPath(id)), nil)
	if err != nil {
		return nil, err
	}
	return &obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}
