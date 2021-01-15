package query

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/cert/types"
)

// RawClient interface
type RawClient interface {
	Certificates() ([]byte, error)
	CertificatesState(string) ([]byte, error)
	Owner(sdk.Address) ([]byte, error)
	OwnerState(sdk.Address, string) ([]byte, error)
	Certificate(types.CertID) ([]byte, error)
}

type rawclient struct {
	ctx sdkclient.Context
	key string
}

// NewRawClient creates a client instance with provided context and key
func NewRawClient(ctx sdkclient.Context, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

func (r rawclient) Certificates() ([]byte, error) {
	buf, _, err := r.ctx.QueryWithData(fmt.Sprintf("custom/%s/certificates/list", r.key), nil)
	if err != nil {
		return []byte{}, err
	}

	return buf, err
}

func (r rawclient) CertificatesState(state string) ([]byte, error) {
	buf, _, err := r.ctx.QueryWithData(fmt.Sprintf("custom/%s/certificates/state/{%s}/list", r.key, state), nil)
	if err != nil {
		return []byte{}, err
	}

	return buf, err
}

func (r rawclient) Owner(address sdk.Address) ([]byte, error) {
	buf, _, err := r.ctx.QueryWithData(fmt.Sprintf("custom/%s/certificates/owner/%s/list", r.key, address.String()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}

func (r rawclient) OwnerState(address sdk.Address, state string) ([]byte, error) {
	buf, _, err := r.ctx.QueryWithData(fmt.Sprintf("custom/%s/certificates/owner/%s/state/%s/list", r.key, address.String(), state), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}

func (r rawclient) Certificate(id types.CertID) ([]byte, error) {
	buf, _, err := r.ctx.QueryWithData(fmt.Sprintf("custom/%s/certificates/owner/%s/%s", r.key, id.Owner.String(), id.Serial.String()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}
