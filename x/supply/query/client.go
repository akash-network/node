package query

import (
	"github.com/cosmos/cosmos-sdk/client/context"
)

// Client interface
type Client interface {
	CirculatingSupply() (Supply, error)
}

// NewClient creates a client instance with provided context and key
func NewClient(ctx context.CLIContext, key string) Client {
	return &client{ctx: ctx, key: key}
}

type client struct {
	ctx context.CLIContext
	key string
}

func (c *client) CirculatingSupply() (Supply, error) {
	var obj Supply
	buf, err := NewRawClient(c.ctx, c.key).CirculatingSupply()
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}
