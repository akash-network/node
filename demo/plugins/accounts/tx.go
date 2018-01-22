package accounts

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/tendermint/go-wire/data"
)

// nolint
const (
	TypeCreate = Name + "/create"
	ByteCreate = 0xF6
	TypeUpdate = Name + "/update"
	ByteUpdate = 0xF7
)

func init() {
	sdk.TxMapper.
		RegisterImplementation(CreateTx{}, TypeCreate, ByteCreate).
		RegisterImplementation(UpdateTx{}, TypeUpdate, ByteUpdate)
}

// sets an account's type
type CreateTx struct {
	Type  data.Bytes `json:"type"`
	Actor sdk.Actor  `json:"actor"`
}

func NewCreateTx(accountType []byte, actor sdk.Actor) sdk.Tx {
	return CreateTx{Type: accountType, Actor: actor}.Wrap()
}

// Wrap - fulfills TxInner interface
func (t CreateTx) Wrap() sdk.Tx {
	return sdk.Tx{t}
}

// ValidateBasic makes sure it is valid
func (t CreateTx) ValidateBasic() error {
	// todo: ensure type is one of user or datacenter
	if len(t.Type) == 0 {
		return ErrMissingData()
	}
	return nil
}

// sets an account's type
type UpdateTx struct {
	Resources data.Bytes `json:"resources"`
	Actor     sdk.Actor  `json:"actor"`
}

func NewUpdateTx(resources []byte, actor sdk.Actor) sdk.Tx {
	return UpdateTx{Resources: resources, Actor: actor}.Wrap()
}

// Wrap - fulfills TxInner interface
func (t UpdateTx) Wrap() sdk.Tx {
	return sdk.Tx{t}
}

// ValidateBasic makes sure it is valid
func (t UpdateTx) ValidateBasic() error {
	// todo: ensure account type is datacenter
	if len(t.Resources) == 0 {
		return ErrMissingData()
	}
	return nil
}
