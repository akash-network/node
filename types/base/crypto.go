package base

import (
	"bytes"

	crypto "github.com/tendermint/go-crypto"
	wire "github.com/tendermint/go-wire"
)

type PubKey crypto.PubKey

func (t PubKey) Marshal() ([]byte, error) {
	return wire.BinaryBytes(t), nil
}

func (t *PubKey) MarshalTo(data []byte) (n int, err error) {
	b := bytes.NewBuffer(data)
	wire.WriteBinary(t, b, &n, &err)
	return
}

func (t *PubKey) Unmarshal(data []byte) error {
	return wire.ReadBinaryBytes(data, t)
}

func (t PubKey) MarshalJSON() ([]byte, error) {
	return t.MarshalJSON()
}

func (t *PubKey) UnmarshalJSON(data []byte) error {
	return t.UnmarshalJSON(data)
}

type Signature crypto.Signature

func (t Signature) Marshal() ([]byte, error) {
	return wire.BinaryBytes(t), nil
}

func (t *Signature) MarshalTo(data []byte) (n int, err error) {
	b := bytes.NewBuffer(data)
	wire.WriteBinary(t, b, &n, &err)
	return
}

func (t *Signature) Unmarshal(data []byte) error {
	return wire.ReadBinaryBytes(data, t)
}

func (t Signature) MarshalJSON() ([]byte, error) {
	return t.MarshalJSON()
}

func (t *Signature) UnmarshalJSON(data []byte) error {
	return t.UnmarshalJSON(data)
}
