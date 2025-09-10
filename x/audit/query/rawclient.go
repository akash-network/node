package query

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/audit/v1"
)

// RawClient interface
type RawClient interface {
	AllProviders() ([]byte, error)
	Provider(sdk.AccAddress) ([]byte, error)
	ProviderID(types.ProviderID) ([]byte, error)
	Auditor(sdk.AccAddress) ([]byte, error)
}

// NewRawClient creates a client instance with provided context and key
func NewRawClient(ctx sdkclient.Context, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

type rawclient struct {
	ctx sdkclient.Context
	key string
}

func (c *rawclient) AllProviders() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/attributes/list", c.key), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}

func (c *rawclient) Provider(id sdk.AccAddress) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/attributes/owner/%s/list", c.key, id), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}

func (c *rawclient) ProviderID(id types.ProviderID) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/attributes/auditor/%s/%s", c.key, id.Auditor, id.Owner), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}

func (c *rawclient) Auditor(id sdk.AccAddress) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/attributes/auditor/%s/list", c.key, id), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}
