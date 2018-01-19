package accounts

import "github.com/tendermint/go-wire/data"

// Data is the struct we use to store in the merkle tree
type Data struct {
	// SetAt is the block height this was set at
	SetAt int64 `json:"set_at"`
	// Value is the data that was stored.
	// data.Bytes is like []byte but json encodes as hex not base64
	Type data.Bytes `json:"type"`
}

// NewData creates a new Data item
func NewData(accountType []byte, setAt int64) Data {
	return Data{
		SetAt: setAt,
		Type:  accountType,
	}
}
