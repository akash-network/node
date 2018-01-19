package accounts

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/tendermint/go-wire/data"
)

// nolint
const (
	TypeSet    = Name + "/set"
	TypeRemove = Name + "/remove"

	ByteSet    = 0xF6
	ByteRemove = 0xF7
)

func init() {
	sdk.TxMapper.
		RegisterImplementation(SetTx{}, TypeSet, ByteSet).
		RegisterImplementation(RemoveTx{}, TypeRemove, ByteRemove)
}

/****************
	  SET TX
*****************/

// SetTx sets a key-value pair
type SetTx struct {
	Key   data.Bytes `json:"key"`
	Value data.Bytes `json:"value"`
}

func NewSetTx(key, value []byte) sdk.Tx {
	return SetTx{Key: key, Value: value}.Wrap()
}

// Wrap - fulfills TxInner interface
func (t SetTx) Wrap() sdk.Tx {
	return sdk.Tx{t}
}

// ValidateBasic makes sure it is valid
func (t SetTx) ValidateBasic() error {
	if len(t.Key) == 0 || len(t.Value) == 0 {
		return ErrMissingData()
	}
	return nil
}

/****************
	  REMOVE TX
*****************/

// RemoveTx deletes the value at this key, returns old value
type RemoveTx struct {
	Key data.Bytes `json:"key"`
}

func NewRemoveTx(key []byte) sdk.Tx {
	return RemoveTx{Key: key}.Wrap()
}

// Wrap - fulfills TxInner interface
func (t RemoveTx) Wrap() sdk.Tx {
	return sdk.Tx{t}
}

// ValidateBasic makes sure it is valid
func (t RemoveTx) ValidateBasic() error {
	if len(t.Key) == 0 {
		return ErrMissingData()
	}
	return nil
}

/****************
	  CREATE TX
*****************/

// // sets an account's type
// type CreateTx struct {
// 	Type data.Bytes `json:"type"`
// }

// func NewCreateTx(accountType []byte) sdk.Tx {
// 	return CreateTx{Type: accountType}.Wrap()
// }

// // Wrap - fulfills TxInner interface
// func (t CreateTx) Wrap() sdk.Tx {
// 	return sdk.Tx{t}
// }

// // ValidateBasic makes sure it is valid
// func (t CreateTx) ValidateBasic() error {
// 	// todo: ensure type is one of user or datacenter
// 	if len(t.Type) == 0 {
// 		return ErrMissingData()
// 	}
// 	return nil
// }
