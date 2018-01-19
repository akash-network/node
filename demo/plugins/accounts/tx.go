package accounts

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/tendermint/go-wire/data"
)

// nolint
const (
	TypeCreate = Name + "/create"
	ByteCreate = 0xF6
)

func init() {
	sdk.TxMapper.
		RegisterImplementation(CreateTx{}, TypeCreate, ByteCreate)
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
