package base

import (
	"encoding/hex"

	"github.com/tendermint/go-wire/data"
)

type Bytes data.Bytes

func (t Bytes) Marshal() ([]byte, error) {
	return t, nil
}

func (t *Bytes) Unmarshal(data []byte) error {
	*t = data
	return nil
}

func (t Bytes) MarshalJSON() ([]byte, error) {
	return data.Encoder.Marshal(t)
}

func (t *Bytes) UnmarshalJSON(buf []byte) error {
	ref := (*[]byte)(t)
	return data.Encoder.Unmarshal(ref, buf)
}

func (t *Bytes) DecodeString(buf string) error {
	val, err := hex.DecodeString(buf)
	*t = val
	return err
}

func (t Bytes) EncodeString() string {
	return hex.EncodeToString(t)
}
