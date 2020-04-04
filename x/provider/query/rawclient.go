package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RawClient interface
type RawClient interface {
	Providers() ([]byte, error)
	Provider(sdk.AccAddress) ([]byte, error)
}

// NewRawClient creates a client instance with provided context and key
func NewRawClient(ctx context.CLIContext, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

type rawclient struct {
	ctx context.CLIContext
	key string
}

func (c *rawclient) Providers() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getProvidersPath()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}

func (c *rawclient) Provider(id sdk.AccAddress) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getProviderPath(id)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}
